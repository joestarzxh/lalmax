package muxer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/q191201771/lal/pkg/base"
)

type Fmp4Record struct {
	curfw                            *FileWriter
	initMp4                          []byte
	recordInterval                   int
	enableRecordByInterval           bool
	streamName                       string
	recordPath                       string
	curDuration                      float64
	hasWriteInit                     bool
	currentPart                      *MuxerPart
	currentFileNextPartId            uint64
	currentFileLastPartVideoStartDts time.Duration
	currentFIleLastPartAudioStartDts time.Duration
}

func NewFmp4Record(recordInterval int, enableRecordByInterval bool, streamName, recordPath string) *Fmp4Record {
	r := &Fmp4Record{
		recordInterval:         recordInterval,
		enableRecordByInterval: enableRecordByInterval,
		streamName:             streamName,
		recordPath:             recordPath,
	}

	if !r.enableRecordByInterval {
		err := r.createFile()
		if err != nil {
			return nil
		}
	}

	return r
}

func (r *Fmp4Record) createFile() (err error) {
	r.curfw = &FileWriter{}
	filename := fmt.Sprintf("%s-%d.mp4", r.streamName, time.Now().Unix())
	filenameWithPath := filepath.Join(r.recordPath, filename)
	if err := r.curfw.Create(filenameWithPath); err != nil {
		Log.Errorf("[%s] record fmp4 open file failed. filename=%s, err=%+v", filenameWithPath, err)
		r.curfw = nil
		return err
	}

	return
}

func (r *Fmp4Record) WriteInitFmp4(init []byte) {
	r.initMp4 = init
}

func (r *Fmp4Record) WriteFmp4Segment(part *MuxerPart, lastSampleDuration time.Duration, end bool) (err error) {
	if r.enableRecordByInterval {
		r.WriteMultiFile(part, lastSampleDuration, end)
	} else {
		r.writeSingleFile(part, lastSampleDuration, end)
	}
	return
}

func (r *Fmp4Record) WriteMultiFile(part *MuxerPart, lastSampleDuration time.Duration, end bool) {
	if part == nil {
		return
	}

	if r.curfw == nil {
		err := r.createFile()
		if err != nil {
			Log.Error("create file failed, err:", err)
			return
		}

		r.curDuration = 0
		r.currentFileLastPartVideoStartDts = 0
		r.currentFIleLastPartAudioStartDts = 0
		r.currentFileNextPartId = 0
		r.curfw.Write(r.initMp4)
	}

	if r.currentFileLastPartVideoStartDts == 0 {
		r.currentFileLastPartVideoStartDts = part.StartVideoDts()
	}
	baseDecodeVideoTime := part.StartVideoDts() - r.currentFileLastPartVideoStartDts

	if r.currentFIleLastPartAudioStartDts == 0 {
		r.currentFIleLastPartAudioStartDts = part.StartAudioDts()
	}
	baseDecodeAudioTime := part.StartAudioDts() - r.currentFIleLastPartAudioStartDts

	if end {
		part.SetVideoStartDts(baseDecodeVideoTime)
		part.SetAudioStartDts(baseDecodeAudioTime)
		part.SetPartId(r.currentFileNextPartId)
		part.Encode(lastSampleDuration, end)
		r.curfw.Write(part.Bytes())

		// 结束写文件
		r.curfw.Dispose()
		r.curfw = nil
	} else {
		curpartduration := part.CalcDuration(lastSampleDuration, end)
		part.SetVideoStartDts(baseDecodeVideoTime)
		part.SetAudioStartDts(baseDecodeAudioTime)
		part.SetPartId(r.currentFileNextPartId)
		part.Encode(lastSampleDuration, end)
		r.curfw.Write(part.Bytes())

		r.currentFileNextPartId++

		r.curDuration += curpartduration.Seconds()
		if r.curDuration >= float64(r.recordInterval) {

			// 结束写文件
			r.curfw.Dispose()
			r.curfw = nil
		}
	}
}

func (r *Fmp4Record) writeSingleFile(part *MuxerPart, lastSampleDuration time.Duration, end bool) {
	if r.hasWriteInit {
		err := part.Encode(lastSampleDuration, false)
		if err != nil {
			Log.Errorf("encode muxer part failed: %v", err)
			return
		}
		r.curfw.Write(part.Bytes())
	} else {
		r.curfw.Write(r.initMp4)
		err := part.Encode(lastSampleDuration, false)
		if err != nil {
			Log.Errorf("encode muxer part failed: %v", err)
			return
		}
		r.curfw.Write(part.Bytes())
		r.hasWriteInit = true
	}
}

func (r *Fmp4Record) Dispose() error {
	if r.curfw != nil {
		return r.curfw.Dispose()
	}

	return nil
}

type FileWriter struct {
	fp *os.File
}

func (fw *FileWriter) Create(filename string) (err error) {
	fw.fp, err = os.Create(filename)
	return
}

func (fw *FileWriter) Write(b []byte) (err error) {
	if fw.fp == nil {
		return base.ErrFileNotExist
	}
	_, err = fw.fp.Write(b)
	return
}

func (fw *FileWriter) Dispose() error {
	if fw.fp == nil {
		return base.ErrFileNotExist
	}
	return fw.fp.Close()
}

func (fw *FileWriter) Name() string {
	if fw.fp == nil {
		return ""
	}
	return fw.fp.Name()
}
