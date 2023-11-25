package main

import (
	"encoding/base64"
	"encoding/json"
	"log"

	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v3"

	"github.com/pion/mediadevices/pkg/codec/vpx"

	_ "github.com/pion/mediadevices/pkg/driver/camera"
)

type sessionDescriptionJson struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

type clientPeer struct {
	Uuid string
	Peer *webrtc.PeerConnection
}

var peerList = make([]clientPeer, 0)

var mediaStream mediadevices.MediaStream

// webrtc list ice server
var config webrtc.Configuration = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	},
}

var peer *webrtc.PeerConnection

func decode(in string, obj interface{}) {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		log.Println(err)
	}

	err = json.Unmarshal(b, obj)
	if err != nil {
		log.Println(err)
	}
}

func encode(obj interface{}) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(b)
}

// creat a new peer
func createPeer(offer string) error {
	offerSDP := webrtc.SessionDescription{}
	decode(offer, &offerSDP)
	log.Printf("%v \n\n\n", offerSDP)
	// select x264 codec
	VPXParams, err := vpx.NewVP8Params()
	if err != nil {
		panic(err)
	}
	VPXParams.BitRate = 500_000 // 500kbps
	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&VPXParams),
	)

	mediaEngine := webrtc.MediaEngine{}
	codecSelector.Populate(&mediaEngine)
	api := webrtc.NewAPI(webrtc.WithMediaEngine(&mediaEngine))
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		panic(err)

	}

	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("Connection State has changed %s \n", connectionState.String())
	})

	// prepare media stream
	if mediaStream == nil {
		mediaStream, err = mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
			Video: func(constraints *mediadevices.MediaTrackConstraints) {
				constraints.FrameFormat = prop.FrameFormat(frame.FormatI420)
				constraints.Width = prop.Int(1280)
				constraints.Height = prop.Int(720)
			},
			Codec: codecSelector,
		})
		if err != nil {
			panic(err)
		}
	}
	log.Println(mediaStream)
	for _, track := range mediaStream.GetTracks() {
		track.OnEnded(func(err error) {
			log.Printf("Track (ID %v) ended with error %v", track.ID(), err)
		})

		_, err = peerConnection.AddTransceiverFromTrack(track,
			webrtc.RtpTransceiverInit{
				Direction: webrtc.RTPTransceiverDirectionSendonly,
			},
		)
		if err != nil {
			panic(err)
		}
	}

	//s set the remote session description
	err = peerConnection.SetRemoteDescription(offerSDP)
	if err != nil {
		panic(err)

	}

	// Create an answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		panic(err)

	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	log.Println(peerConnection.LocalDescription().SDP)
	// b, err := sessionDescToJSON(peerConnection.LocalDescription())
	// if err != nil {
	// 	panic(err)
	// }

	peer = peerConnection
	// wsClient.WriteMessage(websocket.TextMessage, b)

	return nil
}

func sessionDescToJSON(sdp *webrtc.SessionDescription) ([]byte, error) {
	sdpjson := sessionDescriptionJson{
		Type: sdp.Type.String(),
		SDP:  sdp.SDP,
	}
	message := Message{
		Event:   "answer",
		Payload: sdpjson,
	}
	b, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// * add a info of client into  clientPeer and create a peer with these client
func NewClient(clientUUID string) ([]byte, error) {

	return nil, nil
}
