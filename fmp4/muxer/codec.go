package muxer

import (
	"bytes"

	"github.com/q191201771/lal/pkg/aac"
)

type Codec interface {
	IsVideo() bool
	Equal(other Codec) bool
	String() string
}

type CodecH264 struct {
	SPS []byte
	PPS []byte
}

func (c *CodecH264) IsVideo() bool {
	return true
}

func (c *CodecH264) Equal(other Codec) bool {
	if other2, ok := other.(*CodecH264); ok {
		return bytes.Equal(c.SPS, other2.SPS) && bytes.Equal(c.PPS, other2.PPS)
	}

	return false
}

func (c *CodecH264) String() string {
	return "H264"
}

type CodecH265 struct {
	SPS []byte
	PPS []byte
	VPS []byte
}

func (c *CodecH265) IsVideo() bool {
	return true
}

func (c *CodecH265) Equal(other Codec) bool {
	if other2, ok := other.(*CodecH265); ok {
		return bytes.Equal(c.SPS, other2.SPS) && bytes.Equal(c.PPS, other2.PPS) && bytes.Equal(c.VPS, other2.VPS)
	}

	return false
}

func (c *CodecH265) String() string {
	return "H265"
}

type CodecAAC struct {
	Ctx     *aac.AscContext
	AscData []byte
}

func (c *CodecAAC) IsVideo() bool {
	return false
}

func (c *CodecAAC) Equal(other Codec) bool {
	if other2, ok := other.(*CodecAAC); ok {
		return bytes.Equal(c.AscData, other2.AscData)
	}

	return false
}

func (c *CodecAAC) String() string {
	return "AAC"
}

type CodecOpus struct {
	ChannelCount int
}

func (c *CodecOpus) IsVideo() bool {
	return false
}

func (c *CodecOpus) Equal(other Codec) bool {
	return false
}

func (c *CodecOpus) String() string {
	return "OPUS"
}
