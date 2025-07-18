package muxer

import (
	"fmt"
	"time"

	"github.com/q191201771/lal/pkg/avc"
	"github.com/q191201771/lal/pkg/base"
	"github.com/q191201771/lalmax/utils"
	"github.com/q191201771/naza/pkg/nazalog"
)

func AudioTimeScale(c Codec) uint32 {
	switch codec := c.(type) {
	case *CodecAAC:
		samplerate, _ := codec.Ctx.GetSamplingFrequency()
		return uint32(samplerate)
	case *CodecOpus:
		return 48000
	}

	return 0
}

func TsToTime(ts uint32) time.Duration {
	return time.Millisecond * time.Duration(ts)
}

type Muxer struct {
	VideoTrack      *Track
	AudioTrack      *Track
	nextTrackId     uint32
	initFmp4        []byte
	hasinitVideo    bool
	hasinitAudio    bool
	vps, sps, pps   []byte
	auidoTimeScale  uint32
	lastVideoDts    time.Duration
	lastAudioDts    time.Duration
	log             nazalog.Logger
	VideoDtsDecoder *utils.DtsDecoder
	AudioDtsDecoder *utils.DtsDecoder
}

func NewMuxer() *Muxer {
	return &Muxer{
		nextTrackId: 1,
		log:         Log,
	}
}

func (m *Muxer) WithLog(log nazalog.Logger) {
	m.log = log
}

func (m *Muxer) AddVideoTrack(c Codec) {
	switch codec := c.(type) {
	case *CodecH264:
		m.sps = codec.SPS
		m.pps = codec.PPS
	case *CodecH265:
		m.vps = codec.VPS
		m.sps = codec.SPS
		m.pps = codec.PPS

	default:
		m.log.Errorf("invalid video codec")
		return
	}

	m.VideoTrack = NewTrack(c, m.nextTrackId, 90000)
	m.nextTrackId++
}

func (m *Muxer) AddAudioTrack(c Codec) {
	m.auidoTimeScale = AudioTimeScale(c)
	m.AudioTrack = NewTrack(c, m.nextTrackId, m.auidoTimeScale)
	m.nextTrackId++
}

func (m *Muxer) AudioTimeScale() uint32 {
	return m.auidoTimeScale
}

func (m *Muxer) GetInitMp4() []byte {
	if m.initFmp4 == nil {
		init := &Init{}
		if m.VideoTrack != nil {
			init.Tracks = append(init.Tracks, &InitTrack{
				ID:        int(m.VideoTrack.TrackId),
				TimeScale: 90000,
				Codec:     m.VideoTrack.Codec,
			})
		}

		if m.AudioTrack != nil {
			init.Tracks = append(init.Tracks, &InitTrack{
				ID:        int(m.AudioTrack.TrackId),
				TimeScale: m.auidoTimeScale,
				Codec:     m.AudioTrack.Codec,
			})
		}

		var w Buffer
		err := init.Marshal(&w)
		if err != nil {
			m.log.Errorf("marshal init fmp4 failed: %v", err)
			return nil
		}

		m.initFmp4 = w.Bytes()
	}

	return m.initFmp4
}

func (m *Muxer) Pack(msg base.RtmpMsg) (*PartSample, error) {
	if msg.Header.MsgTypeId == base.RtmpTypeIdVideo && !msg.IsVideoKeySeqHeader() {
		return m.FeedVideo(msg)
	} else if msg.Header.MsgTypeId == base.RtmpTypeIdAudio && !msg.IsAacSeqHeader() {
		return m.FeedAudio(msg)
	}

	return nil, fmt.Errorf("invalid msg type")
}

func (m *Muxer) FeedVideo(msg base.RtmpMsg) (*PartSample, error) {
	if m.VideoTrack == nil {
		return nil, fmt.Errorf("no video track")
	}
	randomAccess := false
	var nalus [][]byte

	if msg.IsVideoKeySeqHeader() {
		return nil, fmt.Errorf("msg is video key seq header")
	}

	if !m.hasinitVideo {
		if !msg.IsVideoKeyNalu() {
			return nil, fmt.Errorf("first video require key frame")
		}
	}

	var sample *PartSample
	if m.VideoDtsDecoder == nil {
		m.VideoDtsDecoder = utils.NewDtsDecoder(0, 90000, msg.Dts())
	}

	dts := m.VideoDtsDecoder.Decode(msg.Dts())
	if !m.hasinitVideo {
		m.lastVideoDts = dts
		m.hasinitVideo = true
	}

	sample_duration := uint32(durationGoToMp4(dts-m.lastVideoDts, 90000))

	switch msg.VideoCodecId() {
	case base.RtmpCodecIdAvc:
		nals, _ := avc.SplitNaluAvcc(msg.Payload[5:])
		if msg.IsAvcKeyNalu() {
			randomAccess = true
			nalus = append(nalus, m.sps)
			nalus = append(nalus, m.pps)
			nalus = append(nalus, nals...)
		} else {
			nalus = append(nalus, nals...)
		}

		sample = NewPartSampleH26x(int32(durationGoToMp4(TsToTime(msg.Cts()), 90000)), randomAccess, nalus, sample_duration, dts)

	case base.RtmpCodecIdHevc:
		var nals [][]byte
		if msg.IsEnchanedHevcNalu() {
			index := msg.GetEnchanedHevcNaluIndex()
			nals, _ = avc.SplitNaluAvcc(msg.Payload[index:])
		} else {
			nals, _ = avc.SplitNaluAvcc(msg.Payload[5:])
		}

		if msg.IsHevcKeyNalu() {
			randomAccess = true
			nalus = append(nalus, m.vps)
			nalus = append(nalus, m.sps)
			nalus = append(nalus, m.pps)
			nalus = append(nalus, nals...)
		} else {
			nalus = append(nalus, nals...)
		}

		sample = NewPartSampleH26x(int32(durationGoToMp4(TsToTime(msg.Cts()), 90000)), randomAccess, nalus, sample_duration, dts)

	default:
		return nil, fmt.Errorf("invalid video codec id: %d", msg.VideoCodecId())
	}

	m.lastVideoDts = dts

	return sample, nil
}

func (m *Muxer) FeedAudio(msg base.RtmpMsg) (*PartSample, error) {
	if m.AudioTrack == nil {
		return nil, fmt.Errorf("no audio track")
	}

	if m.AudioDtsDecoder == nil {
		m.AudioDtsDecoder = utils.NewDtsDecoder(0, time.Duration(m.auidoTimeScale), msg.Dts())
	}

	dts := m.AudioDtsDecoder.Decode(msg.Dts())
	if !m.hasinitAudio {
		m.lastAudioDts = dts
		m.hasinitAudio = true
	}
	sample_duration := uint32(durationGoToMp4(dts-m.lastAudioDts, m.auidoTimeScale))
	var payload []byte
	switch msg.AudioCodecId() {
	case base.RtmpSoundFormatAac:
		payload = msg.Payload[2:]
	case base.RtmpSoundFormatOpus:
		payload = msg.Payload[5:]

	default:
		return nil, fmt.Errorf("invalid audio codec id: %d", msg.AudioCodecId())
	}

	sample := &PartSample{
		Dts:             dts,
		Duration:        sample_duration,
		IsNonSyncSample: true,
		PTSOffset:       0,
		Payload:         payload,
	}

	m.lastAudioDts = dts

	return sample, nil
}
