package muxer

import "time"

func durationGoToMp4(v time.Duration, timeScale uint32) uint64 {
	timeScale64 := uint64(timeScale)
	secs := v / time.Second
	dec := v % time.Second
	return uint64(secs)*timeScale64 + uint64(dec)*timeScale64/uint64(time.Second)
}

func durationMp4ToGo(v uint64, timeScale uint32) time.Duration {
	timeScale64 := uint64(timeScale)
	secs := v / timeScale64
	dec := v % timeScale64
	return time.Duration(secs)*time.Second + time.Duration(dec)*time.Second/time.Duration(timeScale64)
}

type MuxerPart struct {
	VideoSamples   []*PartSample
	AudioSamples   []*PartSample
	audioTimeScale uint32

	videoStartDTSFilled bool
	videoStartDTS       time.Duration
	audioStartDTSFilled bool
	audioStartDTS       time.Duration

	buffer            *Buffer
	partId            uint64
	partDuration      time.Duration
	videoPartDuration time.Duration
	audioPartDuration time.Duration
}

func NewMuxerPart(partId uint64, audioTimeScale uint32) *MuxerPart {
	return &MuxerPart{
		buffer:         &Buffer{},
		partId:         partId,
		audioTimeScale: audioTimeScale,
	}
}

func (p *MuxerPart) Bytes() []byte {
	return p.buffer.Bytes()
}

func (p *MuxerPart) Duration() time.Duration {
	return p.partDuration
}

func (p *MuxerPart) AudioTimeScale() uint32 {
	return p.audioTimeScale
}

func (p *MuxerPart) Encode(lastSampleDuration time.Duration, end bool) error {
	part := Part{
		SequenceNumber: uint32(p.partId),
	}

	if p.VideoSamples != nil {
		part.Tracks = append(part.Tracks, &PartTrack{
			ID:       1,
			BaseTime: durationGoToMp4(p.videoStartDTS, 90000),
			Samples:  p.VideoSamples,
		})
	}

	if p.AudioSamples != nil {
		part.Tracks = append(part.Tracks, &PartTrack{
			ID:       1 + len(part.Tracks),
			BaseTime: durationGoToMp4(p.audioStartDTS, p.audioTimeScale),
			Samples:  p.AudioSamples,
		})
	}

	err := part.Marshal(p.buffer)
	if err != nil {
		return err
	}

	if !end {
		if p.VideoSamples != nil {
			p.partDuration = lastSampleDuration - p.videoStartDTS
		} else {
			p.partDuration = lastSampleDuration - p.audioStartDTS
		}
	} else {
		if p.VideoSamples != nil {
			p.partDuration = p.videoPartDuration
		} else {
			p.partDuration = p.audioPartDuration
		}
	}

	p.VideoSamples = nil
	p.AudioSamples = nil

	return nil
}

func (p *MuxerPart) WriteVideo(sample *PartSample) {
	if !p.videoStartDTSFilled {
		p.videoStartDTSFilled = true
		p.videoStartDTS = sample.Dts
	}

	p.videoPartDuration = sample.Dts - p.videoStartDTS
	p.VideoSamples = append(p.VideoSamples, sample)
}

func (p *MuxerPart) WriteAudio(sample *PartSample) {
	if !p.audioStartDTSFilled {
		p.audioStartDTSFilled = true
		p.audioStartDTS = sample.Dts
	}

	p.audioPartDuration = sample.Dts - p.audioStartDTS
	p.AudioSamples = append(p.AudioSamples, sample)
}

func (p *MuxerPart) StartVideoDts() time.Duration {
	return p.videoStartDTS
}

func (p *MuxerPart) StartAudioDts() time.Duration {
	return p.audioStartDTS
}

func (p *MuxerPart) ResetStartVideoDts() {
	p.videoStartDTS = 0
}

func (p *MuxerPart) ResetStartAudioDts() {
	p.audioStartDTS = 0
}

func (p *MuxerPart) Clone() *MuxerPart {
	clone := *p
	clone.buffer = &Buffer{}
	return &clone
}

func (p *MuxerPart) SetPartId(partId uint64) {
	p.partId = partId
}

func (p *MuxerPart) CalcDuration(newPartStartDts time.Duration, end bool) (partDuration time.Duration) {
	if !end {
		if p.VideoSamples != nil {
			partDuration = newPartStartDts - p.videoStartDTS
		} else {
			partDuration = newPartStartDts - p.audioStartDTS
		}
	} else {
		if p.VideoSamples != nil {
			partDuration = p.videoPartDuration
		} else {
			partDuration = p.audioPartDuration
		}
	}

	return partDuration
}

func (p *MuxerPart) SetVideoStartDts(videoStartDTS time.Duration) {
	p.videoStartDTS = videoStartDTS
}

func (p *MuxerPart) SetAudioStartDts(audioStartDTS time.Duration) {
	p.audioStartDTS = audioStartDTS
}
