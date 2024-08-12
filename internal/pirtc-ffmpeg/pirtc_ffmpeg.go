package pirtc_ffmpeg

import (
	"errors"
	"io"
	"log"
	"net"
	"runtime"
	"sync"

	"github.com/pion/webrtc/v3"
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
	ffmpegRtp        *FFmpegRTP
	listener         *net.UDPConn
	Connections      map[string]*webrtc.PeerConnection
	mu               sync.Mutex
}

func Init() (*PiRTC, error) {
	pirtc := &PiRTC{
		usageStreamCount: 0,
		listener:         nil,
		Connections:      make(map[string]*webrtc.PeerConnection),
	}
	return pirtc, nil
}

func (pirtc *PiRTC) NewUser(uuid string) error {
	pirtc.mu.Lock()
	defer pirtc.mu.Unlock()

	if _, ok := pirtc.Connections[uuid]; ok {
		return errors.New("USER EXISTS")
	}
	pirtc.Connections[uuid] = nil
	log.Println(pirtc.Connections)
	return nil
}

func (pirtc *PiRTC) UserDisconnect(uuid string) error {
	pirtc.mu.Lock()
	defer pirtc.mu.Unlock()

	if conn, ok := pirtc.Connections[uuid]; ok {
		if conn != nil {
			err := conn.Close()
			if err != nil {
				return err
			}
		}
		delete(pirtc.Connections, uuid)
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

	peer, ok := pirtc.Connections[uuid]
	if !ok {
		return nil, errors.New("USER NOT EXISTS")
	}

	peer, err = webrtc.NewPeerConnection(defaultConfig)
	if err != nil {
		return nil, err
	}

	

	// Tạo và thêm videoTrack tại thời điểm Answer
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
	if err != nil {
		return nil, err
	}

	rtpSender, err := peer.AddTrack(videoTrack)
	if err != nil {
		return nil, err
	}

	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()
	
	peer.OnICEConnectionStateChange(func(is webrtc.ICEConnectionState) {
		if is == webrtc.ICEConnectionStateDisconnected {
			log.Printf("[Peer - %s]: peer disconnected\n", uuid)
			peer.Close()
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
	// write rtp into videoTrack
	go pirtc.receiveRTP(videoTrack)
	return peer.LocalDescription(), nil
}

func (pirtc *PiRTC) enableStream() error {
	pirtc.mu.Lock()
	defer pirtc.mu.Unlock()
	if pirtc.listener == nil{
		pirtc.ffmpegRtp = NewFFmpegRTP("/dev/video0", "rtp://127.0.0.1:5004")
		err := pirtc.ffmpegRtp.Start()
		if err != nil {
			return err
		}
		log.Println("RTP Stream Enabled")
		pirtc.listener, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5004})
		if err != nil {
			panic(err)
		}
		bufferSize := 300000 // 300KB
		err = pirtc.listener.SetReadBuffer(bufferSize)
		if err != nil {
			panic(err)
		}
	}
	return nil
}

func (pirtc *PiRTC) receiveRTP(videoTrack *webrtc.TrackLocalStaticRTP) {
	inboundRTPPacket := make([]byte, 1600)
	for {
		n, _, err := pirtc.listener.ReadFrom(inboundRTPPacket)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Println("RTP listener closed, exiting receiveRTP")
				return
			}
			log.Printf("RTP packet read error: %v\n", err)
			continue
		}

		if _, err = videoTrack.Write(inboundRTPPacket[:n]); err != nil {
				if errors.Is(err, io.ErrClosedPipe) {
					// The peerConnection has been closed.
					return
		}
			panic(err)
		}
		runtime.Gosched()
	}
}

func (pirtc *PiRTC) incrementStreamUsage() {
	pirtc.mu.Lock()
	defer pirtc.mu.Unlock()
	pirtc.usageStreamCount++
	log.Println("Stream usage count: ", pirtc.usageStreamCount)
}

func (pirtc *PiRTC) disableStream() error {
	if pirtc.ffmpegRtp != nil {
		log.Println("Heerree")
		err := pirtc.ffmpegRtp.Stop()
		if err != nil {
			return err
		}
		pirtc.ffmpegRtp = nil
		err = pirtc.listener.Close()
		if err != nil {
			return err
		}
		pirtc.listener =nil
	}

	return nil
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

