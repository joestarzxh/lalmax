package muxer

import (
	"bytes"
	"time"

	"github.com/q191201771/lal/pkg/aac"
	"github.com/q191201771/lal/pkg/avc"
	"github.com/q191201771/lal/pkg/base"
	"github.com/q191201771/lal/pkg/hevc"
	"github.com/q191201771/naza/pkg/nazalog"
)

type IRtmp2Fmp4muxerObserver interface {
	OnInitFmp4(init []byte)
	OnFmp4Packets(currentPart *MuxerPart, lastSampleDuration time.Duration, end bool, isVideo bool)
}

var waitHeaderQueueSize = 16

type Rtmp2Fmp4Remuxer struct {
	data        []base.RtmpMsg
	done        bool
	maxMsgSize  int
	vCodec      Codec
	aCodec      Codec
	mux         *Muxer
	observer    IRtmp2Fmp4muxerObserver
	log         nazalog.Logger
	nextPartId  uint64
	currentPart *MuxerPart
}

func NewRtmp2Fmp4Remuxer(observer IRtmp2Fmp4muxerObserver) *Rtmp2Fmp4Remuxer {
	m := &Rtmp2Fmp4Remuxer{
		maxMsgSize: waitHeaderQueueSize,
		data:       make([]base.RtmpMsg, waitHeaderQueueSize)[0:0],
		done:       false,
		observer:   observer,
		log:        Log,
	}

	m.mux = NewMuxer()
	m.mux.WithLog(m.log)

	return m
}

func (m *Rtmp2Fmp4Remuxer) WithLog(log nazalog.Logger) *Rtmp2Fmp4Remuxer {
	m.log = log
	m.mux.WithLog(m.log)
	return m
}

func (m *Rtmp2Fmp4Remuxer) FeedRtmpMessage(msg base.RtmpMsg) {
	m.Push(msg)
}

func (m *Rtmp2Fmp4Remuxer) Push(msg base.RtmpMsg) {
	if msg.Header.MsgTypeId == base.RtmpTypeIdMetadata {
		return
	}

	if m.done {
		m.pack(msg)
		return
	}

	if msg.IsVideoKeySeqHeader() {
		switch msg.VideoCodecId() {
		case base.RtmpCodecIdAvc:
			if sps, pps, err := avc.ParseSpsPpsFromSeqHeader(msg.Payload); err != nil {
				m.log.Errorf("parse sps pps from seq header failed: %v", err)
				return
			} else {
				m.vCodec = &CodecH264{
					SPS: sps,
					PPS: pps,
				}
			}

		case base.RtmpCodecIdHevc:
			var vps, sps, pps []byte
			var err error

			if msg.IsEnhanced() {
				vps, sps, pps, err = hevc.ParseVpsSpsPpsFromEnhancedSeqHeader(msg.Payload)
				if err != nil {
					nazalog.Error("ParseVpsSpsPpsFromEnhancedSeqHeader failed, err:", err)
					break
				}

			} else {
				vps, sps, pps, err = hevc.ParseVpsSpsPpsFromSeqHeader(msg.Payload)
				if err != nil {
					nazalog.Error("ParseVpsSpsPpsFromSeqHeader failed, err:", err)
					break
				}
			}

			m.vCodec = &CodecH265{
				VPS: vps,
				SPS: sps,
				PPS: pps,
			}

		default:
			m.log.Errorf("unknown video codec id: %d", msg.VideoCodecId())
			return
		}
	}

	if msg.Header.MsgTypeId == base.RtmpTypeIdAudio {
		switch msg.AudioCodecId() {
		case base.RtmpSoundFormatAac:
			if msg.IsAacSeqHeader() {
				if ascCtx, err := aac.NewAscContext(msg.Payload[2:]); err != nil {
					m.log.Errorf("new asc context failed: %v", err)
					return
				} else {
					m.aCodec = &CodecAAC{
						Ctx:     ascCtx,
						AscData: msg.Payload[2:],
					}
				}
			}

		default:
			return
		}
	}

	m.data = append(m.data, msg.Clone())

	if m.vCodec != nil && m.aCodec != nil {
		m.drain()
		return
	}

	if len(m.data) >= m.maxMsgSize {
		m.drain()
		return
	}
}

func (m *Rtmp2Fmp4Remuxer) drain() {
	if m.vCodec != nil {
		m.mux.AddVideoTrack(m.vCodec)
	}

	if m.aCodec != nil {
		m.mux.AddAudioTrack(m.aCodec)
	}

	init := m.mux.GetInitMp4()
	if m.observer != nil {
		m.observer.OnInitFmp4(init)
	}

	for i := range m.data {
		m.pack(m.data[i])
	}

	m.data = nil
	m.done = true
}

func (m *Rtmp2Fmp4Remuxer) FlushLastSegment() {
	if m.currentPart != nil {
		if err := m.currentPart.Encode(0, true); err == nil && m.observer != nil {
			m.observer.OnFmp4Packets(m.currentPart, 0, true, false)
		}
	}
}

func (m *Rtmp2Fmp4Remuxer) Dispose() {
}

func (m *Rtmp2Fmp4Remuxer) pack(msg base.RtmpMsg) {
	paramsChanged := false
	if m.done {
		if msg.IsVideoKeySeqHeader() {
			switch msg.VideoCodecId() {
			case base.RtmpCodecIdAvc:
				if sps, pps, err := avc.ParseSpsPpsFromSeqHeader(msg.Payload); err != nil {
					m.log.Errorf("parse sps pps from seq header failed: %v", err)
					return
				} else {
					codec, ok := m.vCodec.(*CodecH264)
					if !ok || !bytes.Equal(codec.SPS, sps) || !bytes.Equal(codec.PPS, pps) {
						old := m.vCodec
						m.vCodec = &CodecH264{
							SPS: sps,
							PPS: pps,
						}

						paramsChanged = true
						if old != nil && m.vCodec != nil {
							m.log.Infof("video codec changed, old:%s, new:%s", old.String(), m.vCodec.String())
						}
					}
				}

			case base.RtmpCodecIdHevc:
				var vps, sps, pps []byte
				var err error

				if msg.IsEnhanced() {
					vps, sps, pps, err = hevc.ParseVpsSpsPpsFromEnhancedSeqHeader(msg.Payload)
					if err != nil {
						nazalog.Error("ParseVpsSpsPpsFromEnhancedSeqHeader failed, err:", err)
						break
					}

				} else {
					vps, sps, pps, err = hevc.ParseVpsSpsPpsFromSeqHeader(msg.Payload)
					if err != nil {
						nazalog.Error("ParseVpsSpsPpsFromSeqHeader failed, err:", err)
						break
					}
				}

				codec, ok := m.vCodec.(*CodecH265)
				if !ok || !bytes.Equal(codec.VPS, vps) || !bytes.Equal(codec.SPS, sps) || !bytes.Equal(codec.PPS, pps) {
					old := m.vCodec
					m.vCodec = &CodecH265{
						VPS: vps,
						SPS: sps,
						PPS: pps,
					}

					paramsChanged = true
					if old != nil && m.vCodec != nil {
						m.log.Infof("video codec changed, old:%s, new:%s", old.String(), m.vCodec.String())
					}
				}
			}
		} else if msg.Header.MsgTypeId == base.RtmpTypeIdAudio {
			if msg.IsAacSeqHeader() {
				if ascCtx, err := aac.NewAscContext(msg.Payload[2:]); err != nil {
					m.log.Errorf("new asc context failed: %v", err)
					return
				} else {
					codec, ok := m.aCodec.(*CodecAAC)
					if !ok || !bytes.Equal(codec.AscData, msg.Payload[2:]) {
						old := m.aCodec

						m.aCodec = &CodecAAC{
							Ctx:     ascCtx,
							AscData: msg.Payload[2:],
						}

						paramsChanged = true
						if old != nil && m.aCodec != nil {
							m.log.Infof("audio codec changed, old:%s, new:%s", old.String(), m.aCodec.String())
						}
					}
				}
			}
		}

		if paramsChanged {
			// 编码格式发生变化，需要更新init和强制生成当前这个文件
			if m.currentPart != nil {
				if err := m.currentPart.Encode(0, true); err == nil && m.observer != nil {
					m.observer.OnFmp4Packets(m.currentPart, 0, true, false)
				}
			}

			m.mux = NewMuxer()
			m.mux.WithLog(m.log)

			if m.vCodec != nil {
				m.mux.AddVideoTrack(m.vCodec)
			}

			if m.aCodec != nil {
				m.mux.AddAudioTrack(m.aCodec)
			}

			init := m.mux.GetInitMp4()
			if m.observer != nil {
				m.observer.OnInitFmp4(init)
			}

			m.currentPart = nil
		}
	}

	sample, err := m.mux.Pack(msg)
	if err == nil {
		if m.vCodec != nil {
			// 视频存在的话，I帧作为分割点
			if msg.IsVideoKeyNalu() {
				if m.currentPart == nil {
					m.currentPart = NewMuxerPart(m.partId(), m.mux.AudioTimeScale())
				} else {
					if len(m.currentPart.VideoSamples) >= 15 {
						if m.observer != nil {
							m.observer.OnFmp4Packets(m.currentPart, sample.Dts, false, true)
						} else {
							return
						}

						m.currentPart = NewMuxerPart(m.partId(), m.mux.AudioTimeScale())
					}
				}
			}

			if msg.Header.MsgTypeId == base.RtmpTypeIdVideo {
				m.currentPart.WriteVideo(sample)
			} else {
				// 防止起始是音频
				if m.currentPart == nil {
					m.currentPart = NewMuxerPart(m.partId(), m.mux.AudioTimeScale())
				}

				m.currentPart.WriteAudio(sample)
			}
		} else {
			if m.currentPart == nil {
				m.currentPart = NewMuxerPart(m.partId(), m.mux.AudioTimeScale())
			} else {
				// 只有音频的话，2s分割
				if m.currentPart.Duration() >= 2*time.Second {
					if m.observer != nil {
						m.observer.OnFmp4Packets(m.currentPart, sample.Dts, false, false)
					}

					m.currentPart = NewMuxerPart(m.partId(), m.mux.AudioTimeScale())
				}
			}

			m.currentPart.WriteAudio(sample)
		}
	}
}

func (m *Rtmp2Fmp4Remuxer) partId() uint64 {
	id := m.nextPartId
	m.nextPartId++
	return id
}
