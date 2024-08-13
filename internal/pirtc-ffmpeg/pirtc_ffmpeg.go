package pirtc_ffmpeg

import (
	"bytes"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sync"

	// "github.com/pion/rtp/codecs"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/samplebuilder"
	"golang.org/x/image/vp8"
	// "github.com/pion/webrtc/v4/pkg/media/samplebuilder"
)

var defaultConfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	},
}

type PiWebRTC interface{
	Init() error
	NewUser(uuid string) error
	UserDisconnect(uuid string) error
	Answer(uuid string, offerSD webrtc.SessionDescription) (*webrtc.SessionDescription, error)
	CreateSessionDescription(typeSd string, sdp string) webrtc.SessionDescription
}

type PiRTC struct {
	usageStreamCount int

	ffmpegRtp        *FFmpegRTP
	listener         *net.UDPConn
	track *webrtc.TrackLocalStaticRTP
	Connections      map[string]*webrtc.PeerConnection
	rtpChan chan *rtp.Packet 
	mu               sync.Mutex
}

func Init() (*PiRTC, error) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5004})
		if err != nil {
			panic(err)
		}
		bufferSize := 300000 // 300KB
		err = listener.SetReadBuffer(bufferSize)
		if err != nil {
			panic(err)
		}
	pirtc := &PiRTC{
		usageStreamCount: 0,
		listener:         listener,
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
	
	rtpSender, err := peer.AddTrack(pirtc.track)
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
	return peer.LocalDescription(), nil
}

func (pirtc *PiRTC) enableStream() error {
	pirtc.mu.Lock()
	defer pirtc.mu.Unlock()
	if pirtc.ffmpegRtp == nil{
		pirtc.ffmpegRtp = NewFFmpegRTP("/dev/video0", "rtp://127.0.0.1:5004")
		err := pirtc.ffmpegRtp.Start()
		if err != nil {
			return err
		}
		log.Println("RTP Stream Enabled")
		videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion")
		if err != nil {
			return  err
		}
		pirtc.track = videoTrack
		go pirtc.receiveRTP()
	}
	return nil
}

func (pirtc *PiRTC) receiveRTP() {
	inboundRTPPacket := make([]byte, 1600)
	var packet rtp.Packet
	pirtc.rtpChan = make(chan *rtp.Packet)
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
		err = packet.Unmarshal(inboundRTPPacket[:n])
		if err !=nil{
			panic(err)
		}
		select {
		case pirtc.rtpChan <- &packet:
		default:
			if _, err = pirtc.track.Write(inboundRTPPacket[:n]); err != nil {
				if errors.Is(err, io.ErrClosedPipe) {
					// The peerConnection has been closed.
					return	
				}
				panic(err)
			}
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

func (pirtc *PiRTC) disableStream() error {
	if pirtc.ffmpegRtp != nil {
		err := pirtc.ffmpegRtp.Stop()
		if err != nil {
			return err
		}
	
		pirtc.ffmpegRtp = nil
		pirtc.track = nil
	}

	return nil
}

func (p *PiRTC) TakeShot(fileName string) error{
	err := os.MkdirAll("images", os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create images directory: %w", err)
	}

	if p.track == nil || p.rtpChan == nil {
		// Command to capture image using FFmpeg
		cmd := exec.Command("ffmpeg", "-f", "v4l2", "-i", "/dev/video0", "-vframes", "1", "images/"+fileName+".jpg")
		
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("failed to capture image using FFmpeg: %w", err)
		}

		log.Printf("Image captured using FFmpeg and saved to images/%s.jpg\n", fileName)
		return nil
	}

	sampleBuilder := samplebuilder.New(20, &codecs.VP8Packet{}, 90000)
	decoder := vp8.NewDecoder()

	for packet := range p.rtpChan {
		sampleBuilder.Push(packet)
		sample := sampleBuilder.Pop()
		if sample == nil {
			continue
		}

		videoKeyframe := (sample.Data[0] & 0x1) == 0
		if !videoKeyframe {
			continue
		}

		decoder.Init(bytes.NewReader(sample.Data), len(sample.Data))

		if _, err := decoder.DecodeFrameHeader(); err != nil {
			return err
		}

		img, err := decoder.DecodeFrame()
		if err != nil {
			return err
		}

		buffer := new(bytes.Buffer)
		if err := jpeg.Encode(buffer, img, nil); err != nil {
			return err
		}

		if err := saveImageToFile("images/"+fileName+".jpg", buffer.Bytes()); err != nil {
			return err
		}

		log.Printf("Image captured from RTP stream and saved to images/%s.jpg\n", fileName)
		break
	}

	return nil
}


func saveImageToFile(filePath string, data []byte) error {
	return os.WriteFile(filePath, data, 0644)
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

