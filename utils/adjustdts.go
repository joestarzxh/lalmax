package utils

import "time"

type DtsDecoder struct {
	startDts  time.Duration
	clockRate time.Duration
	overall   time.Duration
	prev      uint32
}

func NewDtsDecoder(startDts, clockRate time.Duration, prevDts uint32) *DtsDecoder {
	return &DtsDecoder{
		startDts:  startDts,
		clockRate: clockRate,
		prev:      prevDts,
	}
}

func multiplyAndDivide(v, m, d time.Duration) time.Duration {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

func (d *DtsDecoder) Decode(ts uint32) time.Duration {
	// 这样可以解决翻转问题
	diff := int32(ts - d.prev)
	if diff >= 1000 || diff <= -1000 {
		// 以视频为主，音频计算以后看
		diff = 40
	}
	d.prev = ts
	d.overall += time.Duration(diff * int32(d.clockRate/1000))

	return d.startDts + multiplyAndDivide(d.overall, time.Second, d.clockRate)
}
