package rtc

import (
	"fmt"

	"github.com/pion/rtp"
	"github.com/q191201771/lal/pkg/avc"
	"github.com/q191201771/lal/pkg/base"
	"github.com/q191201771/lal/pkg/hevc"
	"github.com/q191201771/lal/pkg/rtprtcp"
	"github.com/q191201771/naza/pkg/nazalog"
)

const (
	PacketH264 = "H264"
	PacketHEVC = "HEVC"
	PacketPCMA = "PCMA"
	PacketPCMU = "PCMU"
	PacketOPUS = "OPUS"
)

type Packer struct {
	enc IRtpEncoder
}

func NewPacker(mimeType string, codec []byte) *Packer {
	p := &Packer{}

	switch mimeType {
	case PacketH264:
		p.enc = NewH264RtpEncoder(codec)
	case PacketPCMA:
		p.enc = NewG711RtpEncoder(8)
	case PacketPCMU:
		p.enc = NewG711RtpEncoder(0)
	case PacketHEVC:
		p.enc = NewHevcRtpEncoder(codec)
	case PacketOPUS:
		p.enc = NewOpusRtpEncoder(111)
	}
	return p
}

func (p *Packer) Encode(msg base.RtmpMsg) ([]*rtp.Packet, error) {
	return p.enc.Encode(msg)
}

type IRtpEncoder interface {
	Encode(msg base.RtmpMsg) ([]*rtp.Packet, error)
}

type H264RtpEncoder struct {
	IRtpEncoder
	sps       []byte
	pps       []byte
	rtpPacker *rtprtcp.RtpPacker
}

func NewH264RtpEncoder(codec []byte) *H264RtpEncoder {
	sps, pps, err := avc.ParseSpsPpsFromSeqHeader(codec)
	if err != nil {
		nazalog.Error(err)
		return nil
	}

	pp := rtprtcp.NewRtpPackerPayloadAvc(func(option *rtprtcp.RtpPackerPayloadAvcHevcOption) {
		option.Typ = rtprtcp.RtpPackerPayloadAvcHevcTypeAnnexb
	})

	return &H264RtpEncoder{
		sps:       sps,
		pps:       pps,
		rtpPacker: rtprtcp.NewRtpPacker(pp, 90000, 0),
	}
}

func (enc *H264RtpEncoder) Encode(msg base.RtmpMsg) ([]*rtp.Packet, error) {
	var out []byte
	err := avc.IterateNaluAvcc(msg.Payload[5:], func(nal []byte) {
		t := avc.ParseNaluType(nal[0])
		if t == avc.NaluTypeSei {
			return
		}

		if t == avc.NaluTypeIdrSlice {
			out = append(out, avc.NaluStartCode3...)
			out = append(out, enc.sps...)
			out = append(out, avc.NaluStartCode3...)
			out = append(out, enc.pps...)
		}

		out = append(out, avc.NaluStartCode3...)
		out = append(out, nal...)
	})

	if err != nil {
		return nil, fmt.Errorf("Packetize failed")
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("Packetize failed")
	}

	avpacket := base.AvPacket{
		Timestamp: int64(msg.Dts()),
		Payload:   out,
	}

	var pkts []*rtp.Packet
	rtpPkts := enc.rtpPacker.Pack(avpacket)
	for _, pkt := range rtpPkts {
		var newRtpPkt rtp.Packet
		err := newRtpPkt.Unmarshal(pkt.Raw)
		if err != nil {
			nazalog.Error(err)
			continue
		}

		pkts = append(pkts, &newRtpPkt)
	}

	if len(pkts) == 0 {
		return nil, fmt.Errorf("Packetize failed")
	}

	return pkts, nil
}

type G711RtpEncoder struct {
	IRtpEncoder
	rtpPacker *rtprtcp.RtpPacker
}

func NewG711RtpEncoder(pt uint8) *G711RtpEncoder {
	// TODO 暂时采样率设置为8000
	pp := rtprtcp.NewRtpPackerPayloadPcm()

	return &G711RtpEncoder{
		rtpPacker: rtprtcp.NewRtpPacker(pp, 8000, 0),
	}
}

func (enc *G711RtpEncoder) Encode(msg base.RtmpMsg) ([]*rtp.Packet, error) {
	avpacket := base.AvPacket{
		Timestamp: int64(msg.Dts()),
		Payload:   msg.Payload[1:],
	}

	var pkts []*rtp.Packet
	rtpPkts := enc.rtpPacker.Pack(avpacket)
	for _, pkt := range rtpPkts {
		var newRtpPkt rtp.Packet
		err := newRtpPkt.Unmarshal(pkt.Raw)
		if err != nil {
			nazalog.Error(err)
			continue
		}

		pkts = append(pkts, &newRtpPkt)
	}

	if len(pkts) == 0 {
		return nil, fmt.Errorf("Packetize failed")
	}

	return pkts, nil
}

type HevcRtpEncoder struct {
	IRtpEncoder
	vps       []byte
	sps       []byte
	pps       []byte
	rtpPacker *rtprtcp.RtpPacker
}

func NewHevcRtpEncoder(codec []byte) *HevcRtpEncoder {
	vps, sps, pps, err := hevc.ParseVpsSpsPpsFromSeqHeader(codec)
	if err != nil {
		nazalog.Error(err)
		return nil
	}

	pp := rtprtcp.NewRtpPackerPayloadHevc(func(option *rtprtcp.RtpPackerPayloadAvcHevcOption) {
		option.Typ = rtprtcp.RtpPackerPayloadAvcHevcTypeAnnexb
	})

	return &HevcRtpEncoder{
		vps:       vps,
		sps:       sps,
		pps:       pps,
		rtpPacker: rtprtcp.NewRtpPacker(pp, 90000, 0),
	}
}

func (enc *HevcRtpEncoder) Encode(msg base.RtmpMsg) ([]*rtp.Packet, error) {
	var out []byte
	err := avc.IterateNaluAvcc(msg.Payload[5:], func(nal []byte) {
		t := hevc.ParseNaluType(nal[0])
		if t == hevc.NaluTypeSei || t == hevc.NaluTypeSeiSuffix {
			return
		}

		if t == hevc.NaluTypeSliceIdr || t == hevc.NaluTypeSliceIdrNlp || t == hevc.NaluTypeSliceCranut {
			out = append(out, avc.NaluStartCode3...)
			out = append(out, enc.vps...)
			out = append(out, avc.NaluStartCode3...)
			out = append(out, enc.sps...)
			out = append(out, avc.NaluStartCode3...)
			out = append(out, enc.pps...)
		}

		out = append(out, avc.NaluStartCode3...)
		out = append(out, nal...)
	})

	if err != nil {
		return nil, fmt.Errorf("Packetize failed")
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("Packetize failed")
	}

	avpacket := base.AvPacket{
		Timestamp: int64(msg.Dts()),
		Payload:   out,
	}

	var pkts []*rtp.Packet
	rtpPkts := enc.rtpPacker.Pack(avpacket)
	for _, pkt := range rtpPkts {
		var newRtpPkt rtp.Packet
		err := newRtpPkt.Unmarshal(pkt.Raw)
		if err != nil {
			nazalog.Error(err)
			continue
		}

		pkts = append(pkts, &newRtpPkt)
	}

	if len(pkts) == 0 {
		return nil, fmt.Errorf("Packetize failed")
	}

	return pkts, nil
}

type OpusRtpEncoder struct {
	IRtpEncoder
	rtpPacker *rtprtcp.RtpPacker
}

func NewOpusRtpEncoder(pt uint8) *OpusRtpEncoder {
	pp := rtprtcp.NewRtpPackerPayloadOpus()

	return &OpusRtpEncoder{
		rtpPacker: rtprtcp.NewRtpPacker(pp, 48000, 0),
	}
}

func (enc *OpusRtpEncoder) Encode(msg base.RtmpMsg) ([]*rtp.Packet, error) {
	avpacket := base.AvPacket{
		Timestamp: int64(msg.Dts()),
		Payload:   msg.Payload[1:],
	}

	var pkts []*rtp.Packet
	rtpPkts := enc.rtpPacker.Pack(avpacket)
	for _, pkt := range rtpPkts {
		var newRtpPkt rtp.Packet
		err := newRtpPkt.Unmarshal(pkt.Raw)
		if err != nil {
			nazalog.Error(err)
			continue
		}

		pkts = append(pkts, &newRtpPkt)
	}

	if len(pkts) == 0 {
		return nil, fmt.Errorf("Packetize failed")
	}

	return pkts, nil
}
