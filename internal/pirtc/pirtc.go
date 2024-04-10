package pirtc

import (
	"errors"
	"log"
	"sync"

	"github.com/pion/mediadevices"
	"github.com/pion/webrtc/v3"

	"github.com/pion/mediadevices/pkg/codec/vpx"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
)

var defaultConfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	},
}

type PiRTC struct {
	isCameraUsed bool
	device       mediadevices.MediaStream
	mediaEngine  webrtc.MediaEngine
	Connections  map[string]*webrtc.PeerConnection
	mu           sync.Mutex
}

func Init() *PiRTC {
	pirtc := PiRTC{
		isCameraUsed: false,
		device:       nil,
		mediaEngine:  webrtc.MediaEngine{},
		Connections:  make(map[string]*webrtc.PeerConnection),
	}
	return &pirtc
}

func (pirtc *PiRTC) NewConnection(uuid string) error {
	if _, ok := pirtc.Connections[uuid]; ok {
		return errors.New("CLIENT EXIST")
	} else {
		pirtc.Connections[uuid] = nil
	}
	return nil
}

func (pirtc *PiRTC) Answer(uuid string, offerSD webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	if pirtc.device == nil {
		err := pirtc.enableDevice()
		if err != nil {
			return nil, err
		}
	}
	pirtc.mu.Lock()
	peer, ok := pirtc.Connections[uuid]
	if !ok {
		return nil, errors.New("CLIENT NOT EXISTS")
	}
	if peer != nil {
		return nil, errors.New("PEER CONNECTION")
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&pirtc.mediaEngine))
	peer, err := api.NewPeerConnection(defaultConfig)
	if err != nil {
		return nil, err
	}

	for _, track := range pirtc.device.GetTracks() {
		track.OnEnded(func(err error) {
			if err != nil {
				log.Printf("Track error: %v\n", err)
			}
			log.Printf("Track (ID: %s) ended \n", track.ID())
		})
		_, err = peer.AddTransceiverFromTrack(track, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionSendonly,
		})
		if err != nil {
			return nil, err
		}
	}

	peer.OnICEConnectionStateChange(func(is webrtc.ICEConnectionState) {
		if is == webrtc.ICEConnectionStateClosed {
			log.Printf("[Peer - %s]: peer closed\n", uuid)
			// TODO: need to do something to remove the closed peer from the list
		}
	})

	err = peer.SetRemoteDescription(offerSD)
	if err != nil {
		return nil, err
	}

	answerSD, err := peer.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}
	gatherComplete := webrtc.GatheringCompletePromise(peer)

	err = peer.SetLocalDescription(answerSD)
	if err != nil {
		return nil, err
	}
	<-gatherComplete

	pirtc.Connections[uuid] = peer
	pirtc.mu.Unlock()
	return peer.LocalDescription(), nil
}

func (pirtc *PiRTC) record(second int) {

}

func (pirtc *PiRTC) enableDevice() error {
	if pirtc.device == nil {
		VP8Params, err := vpx.NewVP8Params()
		if err != nil {
			return err
		}
		VP8Params.BitRate = 500_000 // 5Kbps
		codecSelector := mediadevices.NewCodecSelector(mediadevices.WithVideoEncoders(&VP8Params))
		codecSelector.Populate(&pirtc.mediaEngine)

		pirtc.device, err = mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
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
		pirtc.isCameraUsed = true

	}
	return nil
}
