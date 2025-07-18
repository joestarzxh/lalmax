package muxer

import (
	"fmt"

	"github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/q191201771/lal/pkg/avc"
	"github.com/q191201771/lal/pkg/hevc"
)

func boolToUint8(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}

// InitTrack is a track of Init.
type InitTrack struct {
	// ID, starts from 1.
	ID int

	// time scale.
	TimeScale uint32

	// maximum bitrate.
	// it defaults to 1MB for video tracks, 128k for audio tracks.
	MaxBitrate uint32

	// average bitrate.
	// it defaults to 1MB for video tracks, 128k for audio tracks.
	AvgBitrate uint32

	// codec.
	Codec Codec
}

func (it *InitTrack) marshal(w *mp4Writer) error {
	/*
		|trak|
		|    |tkhd|
		|    |mdia|
		|    |    |mdhd|
		|    |    |hdlr|
		|    |    |minf|
		|    |    |    |vmhd| (video)
		|    |    |    |smhd| (audio)
		|    |    |    |dinf|
		|    |    |    |    |dref|
		|    |    |    |    |    |url|
		|    |    |    |stbl|
		|    |    |    |    |stsd|
		|    |    |    |    |    |av01| (AV1)
		|    |    |    |    |    |    |av1C|
		|    |    |    |    |    |    |btrt|
		|    |    |    |    |    |vp09| (VP9)
		|    |    |    |    |    |    |vpcC|
		|    |    |    |    |    |    |btrt|
		|    |    |    |    |    |hev1| (H265)
		|    |    |    |    |    |    |hvcC|
		|    |    |    |    |    |    |btrt|
		|    |    |    |    |    |avc1| (H264)
		|    |    |    |    |    |    |avcC|
		|    |    |    |    |    |    |btrt|
		|    |    |    |    |    |mp4v| (MPEG-4/2/1 video, MJPEG)
		|    |    |    |    |    |    |esds|
		|    |    |    |    |    |    |btrt|
		|    |    |    |    |    |Opus| (Opus)
		|    |    |    |    |    |    |dOps|
		|    |    |    |    |    |    |btrt|
		|    |    |    |    |    |mp4a| (MPEG-4/1 audio)
		|    |    |    |    |    |    |esds|
		|    |    |    |    |    |    |btrt|
		|    |    |    |    |    |ac-3| (AC-3)
		|    |    |    |    |    |    |dac3|
		|    |    |    |    |    |    |btrt|
		|    |    |    |    |    |ipcm| (LPCM)
		|    |    |    |    |    |    |pcmC|
		|    |    |    |    |    |    |btrt|
		|	 |    |    |    |    |fLaC| (FLAC)
		|    |    |    |    |stts|
		|    |    |    |    |stsc|
		|    |    |    |    |stsz|
		|    |    |    |    |stco|
	*/

	var width int
	var height int

	_, err := w.writeBoxStart(&mp4.Trak{}) // <trak>
	if err != nil {
		return err
	}
	if it.Codec == nil {
		return fmt.Errorf("codec is not for track")
	}
	switch codec := it.Codec.(type) {
	case *CodecH264:
		if len(codec.SPS) == 0 || len(codec.PPS) == 0 {
			return fmt.Errorf("H264 parameters not provided")
		}

		var ctx avc.Context
		err = avc.ParseSps(codec.SPS, &ctx)
		if err != nil {
			return fmt.Errorf("h264 parse sps failed")
		}

		width = int(ctx.Width)
		height = int(ctx.Height)

	case *CodecH265:
		if len(codec.SPS) == 0 || len(codec.PPS) == 0 || len(codec.VPS) == 0 {
			return fmt.Errorf("H265 parameters not provided")
		}

		var ctx hevc.Context
		err = hevc.ParseSps(codec.SPS, &ctx)
		if err != nil {
			return fmt.Errorf("hevc parse sps failed")
		}

		width = int(ctx.PicWidthInLumaSamples)
		height = int(ctx.PicHeightInLumaSamples)

	}
	if it.Codec == nil {
		return nil
	}
	if it.Codec.IsVideo() {
		_, err = w.writeBox(&mp4.Tkhd{ // <tkhd/>
			FullBox: mp4.FullBox{
				Flags: [3]byte{0, 0, 3},
			},
			TrackID: uint32(it.ID),
			Width:   uint32(width * 65536),
			Height:  uint32(height * 65536),
			Matrix:  [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000},
		})
		if err != nil {
			return err
		}
	} else {
		_, err = w.writeBox(&mp4.Tkhd{ // <tkhd/>
			FullBox: mp4.FullBox{
				Flags: [3]byte{0, 0, 3},
			},
			TrackID:        uint32(it.ID),
			AlternateGroup: 1,
			Volume:         256,
			Matrix:         [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000},
		})
		if err != nil {
			return err
		}
	}

	_, err = w.writeBoxStart(&mp4.Mdia{}) // <mdia>
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Mdhd{ // <mdhd/>
		Timescale: it.TimeScale,
		Language:  [3]byte{'u', 'n', 'd'},
	})
	if err != nil {
		return err
	}

	if it.Codec.IsVideo() {
		_, err = w.writeBox(&mp4.Hdlr{ // <hdlr/>
			HandlerType: [4]byte{'v', 'i', 'd', 'e'},
			Name:        "VideoHandler",
		})
		if err != nil {
			return err
		}
	} else {
		_, err = w.writeBox(&mp4.Hdlr{ // <hdlr/>
			HandlerType: [4]byte{'s', 'o', 'u', 'n'},
			Name:        "SoundHandler",
		})
		if err != nil {
			return err
		}
	}

	_, err = w.writeBoxStart(&mp4.Minf{}) // <minf>
	if err != nil {
		return err
	}

	if it.Codec.IsVideo() {
		_, err = w.writeBox(&mp4.Vmhd{ // <vmhd/>
			FullBox: mp4.FullBox{
				Flags: [3]byte{0, 0, 1},
			},
		})
		if err != nil {
			return err
		}
	} else {
		_, err = w.writeBox(&mp4.Smhd{}) // <smhd/>
		if err != nil {
			return err
		}
	}

	_, err = w.writeBoxStart(&mp4.Dinf{}) // <dinf>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Dref{ // <dref>
		EntryCount: 1,
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Url{ // <url/>
		FullBox: mp4.FullBox{
			Flags: [3]byte{0, 0, 1},
		},
	})
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </dref>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </dinf>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Stbl{}) // <stbl>
	if err != nil {
		return err
	}

	_, err = w.writeBoxStart(&mp4.Stsd{ // <stsd>
		EntryCount: 1,
	})
	if err != nil {
		return err
	}

	maxBitrate := it.MaxBitrate
	if maxBitrate == 0 {
		if it.Codec.IsVideo() {
			maxBitrate = 1000000
		} else {
			maxBitrate = 128825
		}
	}

	avgBitrate := it.AvgBitrate
	if avgBitrate == 0 {
		if it.Codec.IsVideo() {
			avgBitrate = 1000000
		} else {
			avgBitrate = 128825
		}
	}

	switch codec := it.Codec.(type) {
	case *CodecH264:
		_, err = w.writeBoxStart(&mp4.VisualSampleEntry{ // <avc1>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeAvc1(),
				},
				DataReferenceIndex: 1,
			},
			Width:           uint16(width),
			Height:          uint16(height),
			Horizresolution: 4718592,
			Vertresolution:  4718592,
			FrameCount:      1,
			Depth:           24,
			PreDefined3:     -1,
		})
		if err != nil {
			return err
		}

		var ctx avc.Context
		err = avc.ParseSps(codec.SPS, &ctx)
		if err != nil {
			return fmt.Errorf("h264 parse sps failed")
		}

		_, err = w.writeBox(&mp4.AVCDecoderConfiguration{ // <avcc/>
			AnyTypeBox: mp4.AnyTypeBox{
				Type: mp4.BoxTypeAvcC(),
			},
			ConfigurationVersion:       1,
			Profile:                    ctx.Profile,
			ProfileCompatibility:       codec.SPS[2],
			Level:                      ctx.Level,
			LengthSizeMinusOne:         3,
			NumOfSequenceParameterSets: 1,
			SequenceParameterSets: []mp4.AVCParameterSet{
				{
					Length:  uint16(len(codec.SPS)),
					NALUnit: codec.SPS,
				},
			},
			NumOfPictureParameterSets: 1,
			PictureParameterSets: []mp4.AVCParameterSet{
				{
					Length:  uint16(len(codec.PPS)),
					NALUnit: codec.PPS,
				},
			},
		})
		if err != nil {
			return err
		}

	case *CodecH265:
		_, err = w.writeBoxStart(&mp4.VisualSampleEntry{ // <hev1>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeHev1(),
				},
				DataReferenceIndex: 1,
			},
			Width:           uint16(width),
			Height:          uint16(height),
			Horizresolution: 4718592,
			Vertresolution:  4718592,
			FrameCount:      1,
			Depth:           24,
			PreDefined3:     -1,
		})
		if err != nil {
			return err
		}

		var ctx hevc.Context
		err = hevc.ParseSps(codec.SPS, &ctx)
		if err != nil {
			return fmt.Errorf("hevc parse sps failed")
		}

		_, err = w.writeBox(&mp4.HvcC{ // <hvcC/>
			ConfigurationVersion:        1,
			GeneralProfileIdc:           ctx.GeneralProfileIdc,
			GeneralProfileCompatibility: Uint32ToBoolSlice(ctx.GeneralProfileCompatibilityFlags),
			GeneralConstraintIndicator: [6]uint8{
				codec.SPS[7], codec.SPS[8], codec.SPS[9],
				codec.SPS[10], codec.SPS[11], codec.SPS[12],
			},
			GeneralLevelIdc: ctx.GeneralLevelIdc,
			// MinSpatialSegmentationIdc
			// ParallelismType
			ChromaFormatIdc:      uint8(ctx.ChromaFormat),
			BitDepthLumaMinus8:   uint8(ctx.BitDepthLumaMinus8),
			BitDepthChromaMinus8: uint8(ctx.BitDepthChromaMinus8),
			// AvgFrameRate
			// ConstantFrameRate
			NumTemporalLayers: 1,
			// TemporalIdNested
			LengthSizeMinusOne: 3,
			NumOfNaluArrays:    3,
			NaluArrays: []mp4.HEVCNaluArray{
				{
					NaluType: byte(h265.NALUType_VPS_NUT),
					NumNalus: 1,
					Nalus: []mp4.HEVCNalu{{
						Length:  uint16(len(codec.VPS)),
						NALUnit: codec.VPS,
					}},
				},
				{
					NaluType: byte(h265.NALUType_SPS_NUT),
					NumNalus: 1,
					Nalus: []mp4.HEVCNalu{{
						Length:  uint16(len(codec.SPS)),
						NALUnit: codec.SPS,
					}},
				},
				{
					NaluType: byte(h265.NALUType_PPS_NUT),
					NumNalus: 1,
					Nalus: []mp4.HEVCNalu{{
						Length:  uint16(len(codec.PPS)),
						NALUnit: codec.PPS,
					}},
				},
			},
		})
		if err != nil {
			return err
		}

	case *CodecAAC:
		sampleRate, _ := codec.Ctx.GetSamplingFrequency()
		_, err = w.writeBoxStart(&mp4.AudioSampleEntry{ // <mp4a>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeMp4a(),
				},
				DataReferenceIndex: 1,
			},
			ChannelCount: uint16(codec.Ctx.ChannelConfiguration),
			SampleSize:   16,
			SampleRate:   uint32(sampleRate * 65536),
		})
		if err != nil {
			return err
		}

		_, err = w.writeBox(&mp4.Esds{ // <esds/>
			Descriptors: []mp4.Descriptor{
				{
					Tag:  mp4.ESDescrTag,
					Size: 32 + uint32(len(codec.AscData)),
					ESDescriptor: &mp4.ESDescriptor{
						ESID: uint16(it.ID),
					},
				},
				{
					Tag:  mp4.DecoderConfigDescrTag,
					Size: 18 + uint32(len(codec.Ctx.Pack())),
					DecoderConfigDescriptor: &mp4.DecoderConfigDescriptor{
						ObjectTypeIndication: objectTypeIndicationAudioISO14496part3,
						StreamType:           streamTypeAudioStream,
						Reserved:             true,
						MaxBitrate:           maxBitrate,
						AvgBitrate:           avgBitrate,
					},
				},
				{
					Tag:  mp4.DecSpecificInfoTag,
					Size: uint32(len(codec.AscData)),
					Data: codec.AscData,
				},
				{
					Tag:  mp4.SLConfigDescrTag,
					Size: 1,
					Data: []byte{0x02},
				},
			},
		})
		if err != nil {
			return err
		}

	case *CodecOpus:
		_, err = w.writeBoxStart(&mp4.AudioSampleEntry{ // <Opus>
			SampleEntry: mp4.SampleEntry{
				AnyTypeBox: mp4.AnyTypeBox{
					Type: mp4.BoxTypeOpus(),
				},
				DataReferenceIndex: 1,
			},
			ChannelCount: uint16(codec.ChannelCount),
			SampleSize:   16,
			SampleRate:   48000 * 65536,
		})
		if err != nil {
			return err
		}

		_, err = w.writeBox(&mp4.DOps{ // <dOps/>
			OutputChannelCount: uint8(codec.ChannelCount),
			PreSkip:            312,
			InputSampleRate:    48000,
		})
		if err != nil {
			return err
		}
	}

	_, err = w.writeBox(&mp4.Btrt{ // <btrt/>
		MaxBitrate: maxBitrate,
		AvgBitrate: avgBitrate,
	})
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </*>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </stsd>
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Stts{ // <stts/>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Stsc{ // <stsc/>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Stsz{ // <stsz/>
	})
	if err != nil {
		return err
	}

	_, err = w.writeBox(&mp4.Stco{ // <stco/>
	})
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </stbl>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </minf>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </mdia>
	if err != nil {
		return err
	}

	err = w.writeBoxEnd() // </trak>
	if err != nil {
		return err
	}

	return nil
}

func Uint32ToBoolSlice(num uint32) [32]bool {
	var boolSlice [32]bool

	for i := 0; i < 32; i++ {
		boolSlice[i] = num&(1<<i) != 0
	}

	return boolSlice
}
