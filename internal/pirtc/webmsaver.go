package pirtc

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/at-wat/ebml-go/webm"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
)

type webmSaver struct {
	videoWriter    webm.BlockWriteCloser
	videoBuilder   *samplebuilder.SampleBuilder
	videoTimestamp time.Duration
}

func newWebmSaver() *webmSaver {
	return &webmSaver{
		videoBuilder: samplebuilder.New(20000, &codecs.VP8Packet{}, 90000),
	}
}

func (s *webmSaver) Close() {
	if s.videoWriter != nil {
		if err := s.videoWriter.Close(); err != nil {
			panic(err)
		}
	}
}

func (s *webmSaver) PushVP8(path string, rtpPacket *rtp.Packet) {
	s.videoBuilder.Push(rtpPacket)

	for {
		sample := s.videoBuilder.Pop()
		if sample == nil {
			return
		}
		// Read VP8 header.
		videoKeyframe := (sample.Data[0]&0x1 == 0)
		if videoKeyframe {
			// Keyframe has frame information.
			raw := uint(sample.Data[6]) | uint(sample.Data[7])<<8 | uint(sample.Data[8])<<16 | uint(sample.Data[9])<<24
			width := int(raw & 0x3FFF)
			height := int((raw >> 16) & 0x3FFF)

			if s.videoWriter == nil {
				// Initialize WebM saver using received frame size.
				s.InitWriter(path, width, height)
			}
		}
		if s.videoWriter != nil {
			s.videoTimestamp += sample.Duration
			if _, err := s.videoWriter.Write(videoKeyframe, int64(s.videoTimestamp/time.Millisecond), sample.Data); err != nil {
				panic(err)
			}
		}
	}
}

func (s *webmSaver) InitWriter(path string, width, height int) {
	dir := filepath.Dir(path)

	// Create directory if not exist
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Printf("Error while creating directory: %v\n", err)

	}

	// create file
	w, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		panic(err)
	}

	ws, err := webm.NewSimpleBlockWriter(w,
		[]webm.TrackEntry{
			{
				Name:            "Video",
				TrackNumber:     1,
				TrackUID:        67890,
				CodecID:         "V_VP8",
				TrackType:       1,
				DefaultDuration: 33333333,
				Video: &webm.Video{
					PixelWidth:  uint64(width),
					PixelHeight: uint64(height),
				},
			},
		})
	if err != nil {
		panic(err)
	}
	s.videoWriter = ws[0]
}
