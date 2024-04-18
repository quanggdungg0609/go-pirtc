package pirtc

import (
	"errors"
	"fmt"
	"image/jpeg"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

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
	stream      mediadevices.MediaStream
	mediaEngine webrtc.MediaEngine
	params      vpx.VP8Params
	Connections map[string]*webrtc.PeerConnection
	mu          sync.Mutex
}

func Init() (*PiRTC, error) {
	VP8Params, err := vpx.NewVP8Params()
	if err != nil {
		return nil, err
	}
	VP8Params.BitRate = 500_000 // 5Kbps

	pirtc := PiRTC{
		stream:      nil,
		params:      VP8Params,
		mediaEngine: webrtc.MediaEngine{},
		Connections: make(map[string]*webrtc.PeerConnection),
	}
	return &pirtc, nil
}

func (pirtc *PiRTC) NewUser(uuid string) error {
	if _, ok := pirtc.Connections[uuid]; ok {
		return errors.New("USER EXIST")
	} else {
		pirtc.Connections[uuid] = nil
	}
	return nil
}

func (pirtc *PiRTC) UserDisconnect(uuid string) error {
	if _, ok := pirtc.Connections[uuid]; ok {
		pirtc.mu.Lock()
		if pirtc.Connections[uuid] != nil {
			err := pirtc.Connections[uuid].Close()
			if err != nil {
				return err
			}
			pirtc.Connections[uuid] = nil
		}
		delete(pirtc.Connections, uuid)
		pirtc.mu.Unlock()
	} else {
		return errors.New("USER NOT FOUND")
	}

	return nil
}

func (pirtc *PiRTC) Answer(uuid string, offerSD webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	if pirtc.stream == nil {
		err := pirtc.enableStream()
		if err != nil {
			return nil, err
		}
	}
	pirtc.mu.Lock()
	peer, ok := pirtc.Connections[uuid]
	if !ok {
		return nil, errors.New("USER NOT EXISTS")
	}
	if peer != nil {
		return nil, errors.New("PEER CONNECTION")
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(&pirtc.mediaEngine))
	peer, err := api.NewPeerConnection(defaultConfig)
	if err != nil {
		return nil, err
	}

	for _, track := range pirtc.stream.GetTracks() {
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

func (pirtc *PiRTC) enableStream() error {
	pirtc.mu.Lock()
	defer pirtc.mu.Unlock()
	if pirtc.stream == nil {
		var err error

		codecSelector := mediadevices.NewCodecSelector(mediadevices.WithVideoEncoders(&pirtc.params))
		codecSelector.Populate(&pirtc.mediaEngine)

		pirtc.stream, err = mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
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
	log.Println("Camera Enabled")
	return nil
}

func (pirtc *PiRTC) disableStream() error {
	if pirtc.stream != nil {
		pirtc.mu.Lock()
		tracks := pirtc.stream.GetTracks()
		if len(tracks) > 0 {
			for _, track := range tracks {
				if err := track.Close(); err != nil {
					return err
				}
			}
		}
		pirtc.stream = nil
		pirtc.mu.Unlock()
	}
	return nil
}

func (pirtc *PiRTC) TakeShot(name string) error {
	/*
	* Take a shot and save with the name given
	* @param name the name of the file will saved
	 */
	if err := pirtc.enableStream(); err != nil {
		panic(err)
	}

	track := pirtc.stream.GetVideoTracks()[0]
	videoTrack := track.(*mediadevices.VideoTrack)

	videoReader := videoTrack.NewReader(false)
	time.AfterFunc(time.Duration(1)*time.Second, func() {
		frame, release, _ := videoReader.Read()
		defer release()

		nameImg := name + ".jpeg"
		output, err := os.Create(nameImg)
		if err != nil {
			panic(err)
		}
		err = jpeg.Encode(output, frame, nil)
		if err != nil {
			panic(err)
		}
	})

	return nil
}

func (pirtc *PiRTC) Record(second int) error {
	//TODO: need to implement
	pirtc.enableStream()

	saver := newWebmSaver()
	dest := getCurrentTimeStr() + ".webM"
	log.Println(pirtc.stream)
	videoTrack := pirtc.stream.GetVideoTracks()[0].(*mediadevices.VideoTrack)

	reader, err := videoTrack.NewRTPReader(pirtc.params.RTPCodec().MimeType, rand.Uint32(), 1000)
	if err != nil {
		return err
	}
	log.Println("Recording video...")
	timer := time.NewTimer(time.Duration(second) * time.Second)

	defer reader.Close()

	for {
		select {
		case <-timer.C:
			log.Println("Video Recorded")
			return nil
		default:
			rtpPacket, release, _ := reader.Read()
			defer release()
			for _, pkt := range rtpPacket {

				saver.PushVP8(dest, pkt)
			}
		}
	}

}

func getCurrentTimeStr() string {
	currentTime := time.Now()

	hour := currentTime.Hour()
	minute := currentTime.Minute()
	second := currentTime.Second()
	day := currentTime.Day()
	month := currentTime.Month()
	year := currentTime.Year()

	timeString := fmt.Sprintf("%02d%02d%02d_%02d%02d%d", hour, minute, second, day, month, year)

	return timeString
}
