package wrtc

import (
	"errors"

	"github.com/pion/mediadevices"
	"github.com/pion/webrtc/v3"

	"github.com/pion/mediadevices/pkg/codec/vpx"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
)

type WRTC struct {
	device   mediadevices.MediaStream
	ListPeer map[string]*webrtc.PeerConnection
}

func InitRTC() *WRTC {
	wrtc := WRTC{
		device:   nil,
		ListPeer: make(map[string]*webrtc.PeerConnection),
	}
	return &wrtc
}

func (wrtc *WRTC) enableMediaStream() error {
	if wrtc.device == nil {
		// select codec VP8
		VPXParams, err := vpx.NewVP8Params()
		if err != nil {
			return err
		}
		VPXParams.BitRate = 500_000 // 5kbps
		codecSelector := mediadevices.NewCodecSelector(
			mediadevices.WithVideoEncoders(&VPXParams),
		)

		mediaEngine := webrtc.MediaEngine{}
		codecSelector.Populate(&mediaEngine)

		// open media devices with constraint
		wrtc.device, err = mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
			Video: func(constraint *mediadevices.MediaTrackConstraints) {
				constraint.FrameFormat = prop.FrameFormat(frame.FormatI420)
				constraint.Width = prop.Int(1280)
				constraint.Height = prop.Int(720)
			},
			Codec: codecSelector,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (wrtc *WRTC) NewConnection(uuid string) error {
	// allocate a place with the key is uuid of client
	if _, ok := wrtc.ListPeer[uuid]; ok {
		return errors.New("Client exist!")
	} else {
		wrtc.ListPeer[uuid] = nil
	}
	return nil
}

func (wrtc *WRTC) RemoveConnection(uuid string) error {
	if _, ok := wrtc.ListPeer[uuid]; ok {
		// condition to verify if client have a peer connection, disconnect first and remove

		//
		// remove client from the list
		delete(wrtc.ListPeer, uuid)
	} else {
		return errors.New("UUID does not exist")
	}
	return nil
}
