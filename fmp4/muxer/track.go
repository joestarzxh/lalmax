package muxer

type Track struct {
	Codec
	TrackId   uint32
	timeScale uint32
	firstDTS  int64
	lastDTS   int64
	samples   []PartSample
}

func NewTrack(codec Codec, trackId, timeSacle uint32) *Track {
	return &Track{
		Codec:     codec,
		TrackId:   trackId,
		timeScale: timeSacle,
		firstDTS:  -1,
	}
}
