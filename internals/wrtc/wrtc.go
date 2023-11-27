package wrtc

import (
	"errors"
	"log"

	"github.com/pion/mediadevices"
	"github.com/pion/webrtc/v3"

	"github.com/pion/mediadevices/pkg/codec/vpx"
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
)

var config = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	},
}

type WRTC struct {
	device      mediadevices.MediaStream
	mediaEngine webrtc.MediaEngine
	ListPeer    map[string]*webrtc.PeerConnection
}

func ConvertToSD(i interface{}) webrtc.SessionDescription {
	offerSDP := webrtc.SessionDescription{}
	// type assertion sdp
	offerSDP.SDP = i.(map[string]interface{})["sdp"].(string)
	if i.(map[string]interface{})["type"].(string) == "offer" {
		offerSDP.Type = webrtc.SDPTypeOffer
	} else {
		offerSDP.Type = webrtc.SDPTypeAnswer
	}
	return offerSDP
}

// Packaging a message for ready to send via websocket
func PayloadPackaging(uuid string, clientUUID string, sd *webrtc.SessionDescription) (interface{}, error) {

	type sessionDescriptionJson struct {
		Type string `json:"type"`
		SDP  string `json:"sdp"`
	}

	type Payload struct {
		From               string                 `json:"from"`
		Target             string                 `json:"target"`
		SessionDescription sessionDescriptionJson `json:"sessionDescription"`
	}

	type Message struct {
		Event   string  `json:"event"`
		Payload Payload `json:"payload"`
	}

	sdpjson := sessionDescriptionJson{
		Type: sd.Type.String(),
		SDP:  sd.SDP,
	}

	payload := Payload{
		From:               uuid,
		Target:             clientUUID,
		SessionDescription: sdpjson,
	}

	message := Message{
		Event:   "answer",
		Payload: payload,
	}
	return message, nil
}

func InitRTC() *WRTC {
	wrtc := WRTC{
		device:      nil,
		mediaEngine: webrtc.MediaEngine{},
		ListPeer:    make(map[string]*webrtc.PeerConnection),
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

		codecSelector.Populate(&wrtc.mediaEngine)

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
		log.Printf("[%s] added", uuid)
	}
	return nil
}

// create a answer session description from a offer Session Description
func (wrtc *WRTC) Answer(uuid string, offerSD webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	if wrtc.device == nil {
		wrtc.enableMediaStream()
	}
	peer, ok := wrtc.ListPeer[uuid]
	if !ok {
		return nil, errors.New("Client not exists")
	}
	if peer != nil {
		return nil, errors.New("Peer exists")
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&wrtc.mediaEngine))
	peer, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	// add track into peer
	for _, track := range wrtc.device.GetTracks() {
		track.OnEnded(func(err error) {
			log.Printf("Track (ID: %s) ended with error: %v\n", track.ID(), err)
		})

		_, err = peer.AddTransceiverFromTrack(track,
			webrtc.RtpTransceiverInit{
				Direction: webrtc.RTPTransceiverDirectionSendonly,
			},
		)
		if err != nil {
			return nil, err
		}
	}

	// Set the remote SessionDescription
	err = peer.SetRemoteDescription(offerSD)
	if err != nil {
		return nil, err
	}

	// Create an answer
	answerSD, err := peer.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}
	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peer)

	// Sets the LocalDescription, and starts our UDP listeners
	err = peer.SetLocalDescription(answerSD)
	if err != nil {
		return nil, err
	}
	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete
	return peer.LocalDescription(), nil
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
