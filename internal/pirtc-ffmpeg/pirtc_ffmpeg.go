package pirtc_ffmpeg

import (
	"bytes"
	"errors"
	"image/jpeg"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/samplebuilder"
	"golang.org/x/image/vp8"
)

var defaultConfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{"stun:stun.l.google.com:19302"},
		},
	},
}

type PiWebRTC interface {
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
	track            *webrtc.TrackLocalStaticRTP

	stopChan         chan struct{}
	snapChan         chan struct{}
	snapCompleteChan chan struct{}
	packetChan       chan *rtp.Packet
	isStreaming      bool

	Connections map[string]*webrtc.PeerConnection
	mu          sync.Mutex
}

func Init() (*PiRTC, error) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5004})
	if err != nil {
		return nil, err
	}

	if err := listener.SetReadBuffer(300000); err != nil {
		return nil, err
	}

	return &PiRTC{
		listener:         listener,
		stopChan:         make(chan struct{}),
		snapChan:         make(chan struct{}),
		snapCompleteChan: make(chan struct{}),
		packetChan:       make(chan *rtp.Packet, 1000),
		Connections:      make(map[string]*webrtc.PeerConnection),
	}, nil
}

func (pirtc *PiRTC) NewUser(uuid string) error {
	pirtc.mu.Lock()
	defer pirtc.mu.Unlock()

	if _, exists := pirtc.Connections[uuid]; exists {
		return errors.New("user exists")
	}

	pirtc.Connections[uuid] = nil
	log.Println(pirtc.Connections)
	return nil
}

func (pirtc *PiRTC) UserDisconnect(uuid string) error {
	pirtc.mu.Lock()
	defer pirtc.mu.Unlock()

	conn, exists := pirtc.Connections[uuid]
	if !exists {
		return errors.New("user not found")
	}

	if conn != nil {
		if err := conn.Close(); err != nil {
			return err
		}
	}

	delete(pirtc.Connections, uuid)
	return nil
}

func (pirtc *PiRTC) Answer(uuid string, offerSD webrtc.SessionDescription) (*webrtc.SessionDescription, error) {
	pirtc.incrementStreamUsage()

	if err := pirtc.enableStream(); err != nil {
		return nil, err
	}

	pirtc.mu.Lock()
	defer pirtc.mu.Unlock()

	peer, exists := pirtc.Connections[uuid]
	if !exists {
		return nil, errors.New("user does not exist")
	}

	var err error
	peer, err = webrtc.NewPeerConnection(defaultConfig)
	if err != nil {
		return nil, err
	}

	rtpSender, err := peer.AddTrack(pirtc.track)
	if err != nil {
		return nil, err
	}

	go pirtc.handleRTCP(rtpSender)

	peer.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		pirtc.handleICEConnectionStateChange(uuid, peer, state)
	})

	if err := peer.SetRemoteDescription(offerSD); err != nil {
		return nil, err
	}

	answerSD, err := peer.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}

	gatherComplete := webrtc.GatheringCompletePromise(peer)

	if err := peer.SetLocalDescription(answerSD); err != nil {
		return nil, err
	}

	<-gatherComplete
	pirtc.Connections[uuid] = peer

	return peer.LocalDescription(), nil
}

func (pirtc *PiRTC) enableStream() error {
	pirtc.mu.Lock()
	defer pirtc.mu.Unlock()

	if pirtc.ffmpegRtp == nil {
		pirtc.ffmpegRtp = NewFFmpegRTP("/dev/video0", "rtp://127.0.0.1:5004")
		if err := pirtc.ffmpegRtp.Start(); err != nil {
			return err
		}

		videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8}, "video", "pion")
		if err != nil {
			return err
		}

		pirtc.track = videoTrack
		pirtc.isStreaming = true

		go pirtc.receiveRTP()
		log.Println("RTP Stream Enabled")
	}

	return nil
}

func (pirtc *PiRTC) receiveRTP() {
	inboundRTPPacket := make([]byte, 1600)

	for {
		select {
		case <-pirtc.stopChan:
			return
		case <-pirtc.snapChan:
			pirtc.handleSnapshot()
		default:
			pirtc.handleRTPPacket(inboundRTPPacket)
		}

		runtime.Gosched()
	}
}

func (pirtc *PiRTC) handleSnapshot() {
	log.Println("Creating snapshot....")

	for {
		select {
		case <-pirtc.snapCompleteChan:
			return
		case <-pirtc.stopChan:
			return
		default:
			pirtc.processIncomingPacket()
		}
	}
}

func (pirtc *PiRTC) handleRTPPacket(inboundRTPPacket []byte) {
	n, _, err := pirtc.listener.ReadFrom(inboundRTPPacket)
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			log.Println("RTP listener closed, exiting receiveRTP")
			return
		}
		log.Printf("RTP packet read error: %v\n", err)
		return
	}

	packet := &rtp.Packet{}
	if err := packet.Unmarshal(inboundRTPPacket[:n]); err != nil {
		log.Printf("Failed to unmarshal RTP packet: %v\n", err)
		return
	}

	if err := pirtc.track.WriteRTP(packet); err != nil {
		log.Println("VideoTrack.WriteRTP error")
		return
	}
}

func (pirtc *PiRTC) processIncomingPacket() {
	inboundRTPPacket := make([]byte, 1600)
	n, _, err := pirtc.listener.ReadFrom(inboundRTPPacket)
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			log.Println("RTP listener closed, exiting receiveRTP")
			return
		}
		log.Printf("RTP packet read error: %v\n", err)
		return
	}

	packet := &rtp.Packet{}
	if err := packet.Unmarshal(inboundRTPPacket[:n]); err != nil {
		log.Printf("Failed to unmarshal RTP packet: %v\n", err)
		return
	}

	clonedPacket := clonePacket(packet)
	pirtc.packetChan <- clonedPacket

	if err := pirtc.track.WriteRTP(packet); err != nil {
		log.Println("VideoTrack.WriteRTP error")
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
		if err := pirtc.ffmpegRtp.Stop(); err != nil {
			return err
		}

		pirtc.ffmpegRtp = nil
		pirtc.track = nil
	}

	return nil
}

func (p *PiRTC) Snapshot(fileName string) {
	nameImg := fileName + ".jpeg"
	dir := filepath.Dir(nameImg)

	if err := os.MkdirAll(dir, 0755); err != nil && !os.IsExist(err) {
		log.Printf("Failed to create directory: %v", err)
		return
	}

	output, err := os.Create(nameImg)
	if err != nil {
		log.Printf("Failed to create file: %v", err)
		return
	}
	defer output.Close()

	sampleBuilder := samplebuilder.New(20, &codecs.VP8Packet{}, 90000)
	decoder := vp8.NewDecoder()

	p.snapChan <- struct{}{}
	defer func() { p.snapCompleteChan <- struct{}{} }()

	for {
		select {
		case packet := <-p.packetChan:
			sampleBuilder.Push(packet)
			if sample := sampleBuilder.Pop(); sample != nil && isKeyframe(sample.Data) {
				if err := p.saveJPEG(output, decoder, sample.Data); err != nil {
					log.Println("Failed to save JPEG:", err)
					return
				}
				log.Println("Image encoded successfully")
				return
			}
		}
	}
}

func (p *PiRTC) saveJPEG(output *os.File, decoder *vp8.Decoder, data []byte) error {
	decoder.Init(bytes.NewReader(data), len(data))
	if _, err := decoder.DecodeFrameHeader(); err != nil {
		return err
	}

	img, err := decoder.DecodeFrame()
	if err != nil {
		return err
	}

	return jpeg.Encode(output, img, nil)
}

func isKeyframe(data []byte) bool {
	return (data[0] & 0x1) == 0
}

func clonePacket(packet *rtp.Packet) *rtp.Packet {
	buf, err := packet.Marshal()
	if err != nil {
		return nil
	}

	var clonedPacket rtp.Packet
	if err := clonedPacket.Unmarshal(buf); err != nil {
		return nil
	}

	return &clonedPacket
}

func CreateSessionDescription(typeSd string, sdp string) webrtc.SessionDescription {
	return webrtc.SessionDescription{
		Type: webrtc.NewSDPType(typeSd),
		SDP:  sdp,
	}
}

func (pirtc *PiRTC) handleRTCP(rtpSender *webrtc.RTPSender) {
	rtcpBuf := make([]byte, 1500)
	for {
		if _, _, err := rtpSender.Read(rtcpBuf); err != nil {
			return
		}
	}
}

func (pirtc *PiRTC) handleICEConnectionStateChange(uuid string, peer *webrtc.PeerConnection, state webrtc.ICEConnectionState) {
	switch state {
	case webrtc.ICEConnectionStateDisconnected:
		log.Printf("[Peer - %s]: peer disconnected\n", uuid)
		peer.Close()
	case webrtc.ICEConnectionStateFailed:
		log.Printf("[Peer - %s]: peer failed\n", uuid)
		peer.Close()
	case webrtc.ICEConnectionStateClosed:
		log.Printf("[Peer - %s]: peer closed\n", uuid)
		pirtc.decrementStreamUsage()
	}
}
