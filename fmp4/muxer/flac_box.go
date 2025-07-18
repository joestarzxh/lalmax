package muxer

import (
	"github.com/abema/go-mp4"
)

func BoxTypeFlac() mp4.BoxType { return mp4.StrToBoxType("fLaC") }

func init() {
	mp4.AddAnyTypeBoxDef(&mp4.AudioSampleEntry{}, BoxTypeFlac())
}

/*
func BoxTypeFlac() mp4.BoxType {
	return mp4.StrToBoxType("fLaC")
}

func init() {
	mp4.AddBoxDef(&FlacBox{})
}

type FlacBox struct {
	mp4.FullBox `mp4:"0,extend"`
}

func (f *FlacBox) GetType() mp4.BoxType {
	return BoxTypeFlac()
}
*/

func BoxTypeDfla() mp4.BoxType {
	return mp4.StrToBoxType("dfLa")
}

func init() {
	mp4.AddBoxDef(&DflaBox{})
}

type DflaBox struct {
	mp4.BaseCustomFieldObject
	Data []byte `mp4:"0,size=8,dynamic"`
}

func (d *DflaBox) GetType() mp4.BoxType {
	return BoxTypeDfla()
}

/*
func (d *DflaBox) GetFieldLength(name string, ctx mp4.Context) uint {
	switch name {
	case "NALUnit":
		return uint(d.Length)
	}
	return 0
}
*/

// AddFlag adds the flag
func (d *DflaBox) AddFlag(uint32) {}

func (d *DflaBox) CheckFlag(uint32) bool {
	return false
}

func (d *DflaBox) GetFlags() uint32 {
	return 0
}

func (d *DflaBox) GetVersion() uint8 {
	return 0
}

func (d *DflaBox) RemoveFlag(uint32) {
}

func (d *DflaBox) SetFlags(uint32) {
}

func (d *DflaBox) SetVersion(uint8) {
}
