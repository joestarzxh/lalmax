package rtc

import (
	"bytes"
	"context"
	"sync"
	"time"

	maxlogic "github.com/q191201771/lalmax/logic"
	"github.com/smallnest/chanx"

	"github.com/gofrs/uuid"
	"github.com/pion/webrtc/v3"
	"github.com/q191201771/lal/pkg/avc"
	"github.com/q191201771/lal/pkg/base"
	"github.com/q191201771/lal/pkg/hevc"
	"github.com/q191201771/lal/pkg/logic"
	"github.com/q191201771/naza/pkg/nazalog"
)

const whepMaxReplayPaceDelay = 5 * time.Millisecond

type whepSession struct {
	group          *maxlogic.Group
	pc             *peerConnection
	subscriberId   string
	lalServer      logic.ILalServer
	videoTrack     *webrtc.TrackLocalStaticRTP
	audioTrack     *webrtc.TrackLocalStaticRTP
	videopacker    *Packer
	audiopacker    *Packer
	msgChan        *chanx.UnboundedChan[base.RtmpMsg]
	closeChan      chan bool
	connectedChan  chan struct{}
	connectedOnce  sync.Once
	paceBaseDts    uint32
	paceBaseAt     time.Time
	paceStarted    bool
	replayingCache bool
}

func NewWhepSession(appName, streamid string, writeChanSize int, pc *peerConnection, lalServer logic.ILalServer) *whepSession {
	ok, group := maxlogic.GetGroupManagerInstance().GetGroup(maxlogic.NewStreamKey(appName, streamid))
	if !ok {
		nazalog.Errorf("not found stream, appName:%s, streamid:%s", appName, streamid)
		return nil
	}

	u, _ := uuid.NewV4()
	return &whepSession{
		group:         group,
		pc:            pc,
		lalServer:     lalServer,
		subscriberId:  u.String(),
		msgChan:       chanx.NewUnboundedChan[base.RtmpMsg](context.Background(), writeChanSize),
		closeChan:     make(chan bool, 2),
		connectedChan: make(chan struct{}, 1),
	}
}

func (conn *whepSession) GetAnswerSDP(offer string) (sdp string) {
	var err error

	videoHeader := conn.group.GetVideoSeqHeaderMsg()
	if videoHeader != nil {
		if videoHeader.IsAvcKeySeqHeader() {
			conn.videoTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "lalmax")
			if err != nil {
				nazalog.Error(err)
				return
			}

			_, err = conn.pc.AddTrack(conn.videoTrack)
			if err != nil {
				nazalog.Error(err)
				return
			}

			conn.videopacker = NewPacker(PacketH264, videoHeader.Payload)
		} else if videoHeader.IsHevcKeySeqHeader() {
			conn.videoTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH265}, "video", "lalmax")
			if err != nil {
				nazalog.Error(err)
				return
			}

			_, err = conn.pc.AddTrack(conn.videoTrack)
			if err != nil {
				nazalog.Error(err)
				return
			}

			conn.videopacker = NewPacker(PacketHEVC, videoHeader.Payload)
		}
	}

	audioHeader := conn.group.GetAudioSeqHeaderMsg()
	if audioHeader != nil {
		var mimeType string
		audioId := audioHeader.AudioCodecId()
		switch audioId {
		case base.RtmpSoundFormatG711A:
			conn.audioTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMA}, "audio", "lalmax")
			if err != nil {
				nazalog.Error(err)
				return
			}

			mimeType = PacketPCMA
		case base.RtmpSoundFormatG711U:
			conn.audioTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU}, "audio", "lalmax")
			if err != nil {
				nazalog.Error(err)
				return
			}

			mimeType = PacketPCMU
		case base.RtmpSoundFormatOpus:
			conn.audioTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "lalmax")
			if err != nil {
				nazalog.Error(err)
				return
			}

			mimeType = PacketOPUS
		default:
			nazalog.Error("unsupport audio codeid:", audioId)
		}

		if conn.audioTrack != nil {
			_, err = conn.pc.AddTrack(conn.audioTrack)
			if err != nil {
				nazalog.Error(err)
				return
			}

			conn.audiopacker = NewPacker(mimeType, nil)
		}
	}

	gatherComplete := webrtc.GatheringCompletePromise(conn.pc.PeerConnection)

	conn.pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  string(offer),
	})

	answer, err := conn.pc.CreateAnswer(nil)
	if err != nil {
		nazalog.Error(err)
		return
	}

	err = conn.pc.SetLocalDescription(answer)
	if err != nil {
		nazalog.Error(err)
		return
	}

	<-gatherComplete

	sdp = conn.pc.LocalDescription().SDP
	return
}

func (conn *whepSession) Run() {
	conn.pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		nazalog.Info("peer connection state: ", state.String())

		switch state {
		case webrtc.PeerConnectionStateConnected:
			conn.signalConnected()
		case webrtc.PeerConnectionStateDisconnected:
			fallthrough
		case webrtc.PeerConnectionStateFailed:
			fallthrough
		case webrtc.PeerConnectionStateClosed:
			conn.closeChan <- true
		}
	})

	if conn.pc.ConnectionState() == webrtc.PeerConnectionStateConnected {
		conn.signalConnected()
	}

	for {
		select {
		case <-conn.connectedChan:
			conn.group.AddSubscriber(maxlogic.SubscriberInfo{
				SubscriberID: conn.subscriberId,
				Protocol:     maxlogic.SubscriberProtocolWHEP,
			}, conn)
			goto connected
		case <-conn.closeChan:
			nazalog.Info("RemoveConsumer, connid:", conn.subscriberId)
			conn.group.RemoveSubscriber(conn.subscriberId)
			return
		}
	}

connected:
	for {
		select {
		case msg := <-conn.msgChan.Out:
			if msg.Header.MsgTypeId == 0 {
				conn.replayingCache = false
				conn.paceBaseAt = time.Time{}
				conn.paceStarted = false
				continue
			}
			if conn.replayingCache {
				conn.paceReplayMsg(msg)
			}
			if msg.Header.MsgTypeId == base.RtmpTypeIdAudio && conn.audioTrack != nil {
				conn.sendAudio(msg)
			} else if msg.Header.MsgTypeId == base.RtmpTypeIdVideo && conn.videoTrack != nil {
				conn.sendVideo(msg)
			}
		case <-conn.closeChan:
			nazalog.Info("RemoveConsumer, connid:", conn.subscriberId)
			conn.group.RemoveSubscriber(conn.subscriberId)
			return
		}
	}
}

func (conn *whepSession) signalConnected() {
	conn.connectedOnce.Do(func() {
		conn.connectedChan <- struct{}{}
	})
}

func (conn *whepSession) OnReplayStart() {
	conn.replayingCache = true
	conn.paceBaseAt = time.Time{}
	conn.paceBaseDts = 0
	conn.paceStarted = false
}

func (conn *whepSession) OnReplayStop() {
	conn.msgChan.In <- base.RtmpMsg{}
}

func (conn *whepSession) paceReplayMsg(msg base.RtmpMsg) {
	if msg.Header.MsgTypeId != base.RtmpTypeIdAudio && msg.Header.MsgTypeId != base.RtmpTypeIdVideo {
		return
	}
	if msg.IsVideoKeySeqHeader() || msg.IsAacSeqHeader() {
		return
	}

	if !conn.paceStarted {
		conn.paceBaseDts = msg.Dts()
		conn.paceBaseAt = time.Now()
		conn.paceStarted = true
		return
	}

	dtsDelta := int64(msg.Dts()) - int64(conn.paceBaseDts)
	if dtsDelta <= 0 {
		return
	}

	mediaElapsed := time.Duration(dtsDelta) * time.Millisecond
	delay := time.Until(conn.paceBaseAt.Add(mediaElapsed))
	if delay <= 0 {
		return
	}
	if delay > whepMaxReplayPaceDelay {
		delay = whepMaxReplayPaceDelay
	}

	time.Sleep(delay)
}

func (conn *whepSession) OnMsg(msg base.RtmpMsg) {
	switch msg.Header.MsgTypeId {
	case base.RtmpTypeIdMetadata:
		return
	case base.RtmpTypeIdAudio:
		if conn.audioTrack != nil {
			conn.msgChan.In <- msg
		}
	case base.RtmpTypeIdVideo:
		if msg.IsVideoKeySeqHeader() {
			conn.updateVideoCodec(msg)
			return
		}
		if conn.videoTrack != nil {
			conn.msgChan.In <- msg
		}
	}
}

func (conn *whepSession) OnStop() {
	conn.closeChan <- true
}

func (conn *whepSession) sendAudio(msg base.RtmpMsg) {
	if conn.audiopacker != nil {
		pkts, err := conn.audiopacker.Encode(msg)
		if err != nil {
			nazalog.Error(err)
			return
		}

		for _, pkt := range pkts {
			if err := conn.audioTrack.WriteRTP(pkt); err != nil {
				continue
			}
		}
	}
}

func (conn *whepSession) sendVideo(msg base.RtmpMsg) {
	if conn.videopacker != nil {

		pkts, err := conn.videopacker.Encode(msg)
		if err != nil {
			nazalog.Error(err)
			return
		}

		for _, pkt := range pkts {
			if err := conn.videoTrack.WriteRTP(pkt); err != nil {
				continue
			}
		}
	}
}

func (conn *whepSession) updateVideoCodec(msg base.RtmpMsg) {
	if conn.videopacker == nil {
		return
	}

	if msg.IsAvcKeySeqHeader() {
		sps, pps, err := avc.ParseSpsPpsFromSeqHeader(msg.Payload)
		if err != nil {
			nazalog.Error("ParseSpsPpsFromSeqHeader err:", err)
			return
		}

		if h264Encoder, ok := conn.videopacker.enc.(*H264RtpEncoder); ok {
			if bytes.Equal(h264Encoder.sps, sps) && bytes.Equal(h264Encoder.pps, pps) {
				return
			}
		}

		conn.videopacker.UpdateVideoCodec(nil, sps, pps)
		return
	}

	if msg.IsHevcKeySeqHeader() {
		vps, sps, pps, err := hevc.ParseVpsSpsPpsFromSeqHeader(msg.Payload)
		if err != nil {
			nazalog.Error("ParseVpsSpsPpsFromSeqHeader err:", err)
			return
		}

		if hevcEncoder, ok := conn.videopacker.enc.(*HevcRtpEncoder); ok {
			if bytes.Equal(hevcEncoder.vps, vps) && bytes.Equal(hevcEncoder.sps, sps) && bytes.Equal(hevcEncoder.pps, pps) {
				return
			}
		}

		conn.videopacker.UpdateVideoCodec(vps, sps, pps)
	}
}

func (conn *whepSession) Close() {
	if conn.pc != nil {
		conn.pc.Close()
	}
}
