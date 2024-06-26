package pirtc

import (
	"errors"
	"fmt"
	"image/jpeg"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
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
	usageStreamCount int
	stream           mediadevices.MediaStream
	mediaEngine      webrtc.MediaEngine
	params           vpx.VP8Params
	Connections      map[string]*webrtc.PeerConnection
	mu               sync.Mutex
}

func Init() (*PiRTC, error) {
	VP8Params, err := vpx.NewVP8Params()
	if err != nil {
		return nil, err
	}
	VP8Params.BitRate = 500_000 // 5Kbps

	pirtc := PiRTC{
		usageStreamCount: 0,
		stream:           nil,
		params:           VP8Params,
		mediaEngine:      webrtc.MediaEngine{},
		Connections:      make(map[string]*webrtc.PeerConnection),
	}
	return &pirtc, nil
}

func (pirtc *PiRTC) NewUser(uuid string) error {
	if _, ok := pirtc.Connections[uuid]; ok {
		return errors.New("USER EXIST")
		} else {
			pirtc.Connections[uuid] = nil
		}
	log.Println(pirtc.Connections)
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

	pirtc.incrementStreamUsage()

	err := pirtc.enableStream()
	if err != nil {
		return nil, err
	}
	pirtc.mu.Lock()
	defer pirtc.mu.Unlock()

	log.Println(uuid)
	peer, ok := pirtc.Connections[uuid]
	if !ok {
		return nil, errors.New("USER NOT EXISTS")
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(&pirtc.mediaEngine))
	peer, err = api.NewPeerConnection(defaultConfig)
	if err != nil {
		panic(err)
	}

	for _, track := range pirtc.stream.GetTracks() {
		track.OnEnded(func(err error) {
			if err != nil {
				log.Printf("Track error: %v\n", err)
			}
			log.Printf("Track (ID: %s) ended \n", track.ID())
			pirtc.decrementStreamUsage()
		})
		_, err = peer.AddTransceiverFromTrack(track, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionSendonly,
		})
		if err != nil {
			return nil, err
		}
	}

	peer.OnICEConnectionStateChange(func(is webrtc.ICEConnectionState) {
		if is == webrtc.ICEConnectionStateDisconnected {
			log.Printf("[Peer - %s]: peer disconnected\n", uuid)
			peer.Close()
			// TODO: need to do something to remove the closed peer from the list
		} else if is == webrtc.ICEConnectionStateFailed {
			log.Printf("[Peer - %s]: peer failed\n", uuid)
			peer.Close()
		} else if is == webrtc.ICEConnectionStateClosed {
			log.Printf("[Peer - %s]: peer closed\n", uuid)
			pirtc.decrementStreamUsage()

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
	return peer.LocalDescription(), nil
}

func (pirtc *PiRTC) enableStream() error {
	/*
	* Enable stream if not exist
	 */
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
		log.Println("Camera Enabled")
	}
	return nil
}

func (pirtc *PiRTC) disableStream() error {
	tracks := pirtc.stream.GetTracks()
	if len(tracks) > 0 {
		for _, track := range tracks {
			if err := track.Close(); err != nil {
				return err
			}
		}
	}
	pirtc.stream = nil
	log.Println("Camera disable")
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
	pirtc.incrementStreamUsage()
	defer pirtc.decrementStreamUsage()

	track := pirtc.stream.GetVideoTracks()[0]
	videoTrack := track.(*mediadevices.VideoTrack)

	videoReader := videoTrack.NewReader(false)

	// skip first frame for warm up camera	
	for i := 0; i < 1; i++ {
		_, release, err := videoReader.Read()
		if err != nil {
			return fmt.Errorf("failed to read frame: %v", err)
		}
		release()
	}
	
	// take image
	frame, release, _ := videoReader.Read()
	defer release()

	nameImg := name + ".jpeg"
	dir := filepath.Dir(nameImg)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
        // Nếu chưa tồn tại, tạo thư mục
        err := os.MkdirAll(dir, 0755)
        if err != nil {
            panic(fmt.Sprintf("Failed to create directory: %s", err))
        }
    }
	output, err := os.Create(nameImg)
	if err != nil {
		panic(err)
	}
	err = jpeg.Encode(output, frame, nil)
	if err != nil {
		panic(err)
	}
	// 	runtime.Gosched()
	// }
	// time.AfterFunc(time.Duration(1)*time.Second, func() {

	// })
	log.Println("Captured Image")
	return nil
}

func (pirtc *PiRTC) Record(savePath string, stopCh chan struct{}) chan struct{} {
	// enableStream if necessary

	doneChan := make(chan struct{})

	go pirtc.record(savePath, doneChan)
	<-stopCh
	close(doneChan)

	return doneChan
}

func (pirtc *PiRTC) RecordWithTimer(savePath string, duration time.Duration) chan struct{} {
	/*
	* Record video to @params savePath in @params second seconds
	* Return the name of video after recored
	 */
	doneChan := make(chan struct{})
	timer := time.NewTimer(duration)
	go pirtc.record(savePath, doneChan)
	<-timer.C
	close(doneChan)
	timer.Stop()

	return doneChan
}

func (pirtc *PiRTC) record(savePath string, stopChan <-chan struct{}) {
	pirtc.enableStream()
	pirtc.incrementStreamUsage()
	defer pirtc.decrementStreamUsage()

	saver := newWebmSaver()
	videoTrack := pirtc.stream.GetVideoTracks()[0].(*mediadevices.VideoTrack)
	reader, err := videoTrack.NewRTPReader(pirtc.params.RTPCodec().MimeType, rand.Uint32(), 1000)
	if err != nil {
		panic(err)
	}
	defer reader.Close()

	log.Println("Recording video...")
	for {
		select {
		case <-stopChan:
			return
		default:
			rtpPacket, release, _ := reader.Read()
			defer release()
			for _, pkt := range rtpPacket {
				saver.PushVP8(savePath, pkt)
			}
		}
		runtime.Gosched()
	}
}

func (pirtc *PiRTC) incrementStreamUsage() {
	pirtc.mu.Lock()
	pirtc.usageStreamCount = pirtc.usageStreamCount + 1
	log.Println("Stream usage count: ", pirtc.usageStreamCount)
	pirtc.mu.Unlock()
}

func (pirtc *PiRTC) decrementStreamUsage() {
	pirtc.mu.Lock()
	defer pirtc.mu.Unlock()
	pirtc.usageStreamCount--
	if pirtc.usageStreamCount < 0 {
		pirtc.usageStreamCount = 0
	}
	log.Println("Stream usage count: ", pirtc.usageStreamCount)

	if pirtc.usageStreamCount == 0 {
		pirtc.disableStream()
	}

}

func CreateSessionDescription(typeSd string, sdp string) webrtc.SessionDescription {
	sd := webrtc.SessionDescription{}
	switch typeSd {
	case "offer":
		sd.Type = webrtc.SDPTypeOffer
	case "answer":
		sd.Type = webrtc.SDPTypeAnswer

	case "rollback":
		sd.Type = webrtc.SDPTypeRollback

	case "pranswer":
		sd.Type = webrtc.SDPTypePranswer
	}

	sd.SDP = sdp
	return sd
}
