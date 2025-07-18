package muxer

import (
	"time"
)

// PartSample is a sample of a PartTrack.
type PartSample struct {
	Dts             time.Duration
	Duration        uint32
	PTSOffset       int32
	IsNonSyncSample bool
	Payload         []byte
}

func avccMarshalSize(au [][]byte) int {
	n := 0
	for _, nalu := range au {
		n += 4 + len(nalu)
	}
	return n
}

// AVCCMarshal encodes an access unit into the AVCC stream format.
// Specification: ISO 14496-15, section 5.3.4.2.1
func AVCCMarshal(au [][]byte) ([]byte, error) {
	buf := make([]byte, avccMarshalSize(au))
	pos := 0

	for _, nalu := range au {
		naluLen := len(nalu)
		buf[pos] = byte(naluLen >> 24)
		buf[pos+1] = byte(naluLen >> 16)
		buf[pos+2] = byte(naluLen >> 8)
		buf[pos+3] = byte(naluLen)
		pos += 4

		pos += copy(buf[pos:], nalu)
	}

	return buf, nil
}

// NewPartSampleH26x creates a sample with H26x data.
func NewPartSampleH26x(ptsOffset int32, randomAccessPresent bool, au [][]byte, duration uint32, dts time.Duration) *PartSample {
	avcc, err := AVCCMarshal(au)
	if err != nil {
		return nil
	}

	return &PartSample{
		Dts:             dts,
		PTSOffset:       ptsOffset,
		IsNonSyncSample: !randomAccessPresent,
		Payload:         avcc,
		Duration:        duration,
	}
}
