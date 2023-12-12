package pigortc

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

type PiGoRTC struct {
	isUseCamera bool
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

func InitRTC() *PiGoRTC {
	pigortc := PiGoRTC{
		isUseCamera: false,
		device:      nil,
		mediaEngine: webrtc.MediaEngine{},
		ListPeer:    make(map[string]*webrtc.PeerConnection),
	}
	return &pigortc
}

func (pigortc *PiGoRTC) enableMediaStream() error {
	if pigortc.device == nil {
		// select codec VP8
		VPXParams, err := vpx.NewVP8Params()
		if err != nil {
			return err
		}
		VPXParams.BitRate = 500_000 // 5kbps
		codecSelector := mediadevices.NewCodecSelector(
			mediadevices.WithVideoEncoders(&VPXParams),
		)

		codecSelector.Populate(&pigortc.mediaEngine)

		// open media devices with constraint
		pigortc.device, err = mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
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
		pigortc.isUseCamera = true
	}
	return nil
}

func (pigortc *PiGoRTC) NewConnection(uuid string) error {
	// allocate a place with the key is uuid of client
	if _, ok := pigortc.ListPeer[uuid]; ok {
		return errors.New("client exist!")
	} else {
		pigortc.ListPeer[uuid] = nil
		log.Printf("[%s] added", uuid)
	}
	return nil
}

// create a answer session description from a offer Session Description
func (pigortc *PiGoRTC) Answer(uuid string, offerSD webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	if pigortc.device == nil {
		pigortc.enableMediaStream()
	}
	peer, ok := pigortc.ListPeer[uuid]
	if !ok {
		return nil, errors.New("client not exists")
	}
	if peer != nil {
		return nil, errors.New("peer exists")
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&pigortc.mediaEngine))
	peer, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	// add track into peer
	for _, track := range pigortc.device.GetTracks() {
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

	// handlers for peer connection state
	peer.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("[Peer Connection State - %s]: %s", uuid, s.String())
		if s == webrtc.PeerConnectionStateClosed {
			log.Printf("[Peer - %s]: peer closed", uuid)
		}
		// if s == webrtc.PeerConnectionStateClosed {
		// 	log.Printf("[Peer - %s]: remove from the list", uuid)
		// 	delete(pigortc.ListPeer, uuid)
		// }
	})

	// peer.OnSignalingStateChange(func(s webrtc.SignalingState) {
	// 	log.Printf("[Peer Signaling State - %s]: %s", uuid, s.String())
	// })

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
	pigortc.ListPeer[uuid] = peer
	return peer.LocalDescription(), nil
}

func (pigortc *PiGoRTC) RemoveConnection(uuid string) error {
	if peer, ok := pigortc.ListPeer[uuid]; ok && peer != nil {
		// condition to verify if client have a peer connection, disconnect first and remove
		peer.Close()
		// remove client from the list
		delete(pigortc.ListPeer, uuid)
		// verify if it is the last peer in the list, close the camera

	} else {
		return errors.New("UUID does not exist")
	}
	return nil
}

func (pigortc *PiGoRTC) DisconnectPeer(uuid string) error {
	return nil
}
