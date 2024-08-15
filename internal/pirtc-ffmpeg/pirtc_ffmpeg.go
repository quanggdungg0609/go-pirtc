package pirtc_ffmpeg

import (
	"bytes"
	"errors"
	"image/jpeg"
	"log"
	"net"
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

	stopChan         chan struct{}
	snapChan         chan struct{}
	snapCompleteChan chan struct{}
	packetChan       chan *rtp.Packet
	isStreaming      bool

	Connections      map[string]*webrtc.PeerConnection
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

		stopChan:         make(chan struct{}),
		snapChan:         make(chan struct{}),
		snapCompleteChan: make(chan struct{}),
		packetChan:       make(chan *rtp.Packet, 1000),

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
		pirtc.isStreaming = true
		go pirtc.receiveRTP()
	}
	return nil
}

func (pirtc *PiRTC) receiveRTP() {

	inboundRTPPacket := make([]byte, 1600)
	StreamLoop:
	for {
		select {
		case <-pirtc.stopChan:
			return
		case <-pirtc.snapChan:
			log.Println("Creating snapshot....")
			for{
				select{
				case <-pirtc.snapCompleteChan:
					continue StreamLoop
				case <-pirtc.stopChan:
					return
				default:
					n, _, err := pirtc.listener.ReadFrom(inboundRTPPacket)
					if err != nil {
						if errors.Is(err, net.ErrClosed) {
							log.Println("RTP listener closed, exiting receiveRTP")
							return
						}
						log.Printf("RTP packet read error: %v\n", err)
						continue
					}
					packet := &rtp.Packet{}
					err = packet.Unmarshal(inboundRTPPacket[:n])
					if err !=nil{
						panic(err)
					}
					// Clone packet and send to snapshot
					clonedPacket := clonePacket(packet)
					pirtc.packetChan <- clonedPacket
					
					if writeErr := pirtc.track.WriteRTP(packet); writeErr != nil {
						log.Println("StartStream VideoTrack.WriteRTP ERROR")
						return
					}
				}
			}
		default:
			n, _, err := pirtc.listener.ReadFrom(inboundRTPPacket)
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					log.Println("RTP listener closed, exiting receiveRTP")
					return
				}
				log.Printf("RTP packet read error: %v\n", err)
				continue
			}
			packet := &rtp.Packet{}
			err = packet.Unmarshal(inboundRTPPacket[:n])
			if err !=nil{
				panic(err)
			}
			
			if writeErr := pirtc.track.WriteRTP(packet); writeErr != nil {
				log.Println("StartStream VideoTrack.WriteRTP ERROR")
				return
			}
			// if _, err = pirtc.track.Write(inboundRTPPacket[:n]); err != nil {
			// 	if errors.Is(err, io.ErrClosedPipe) {
			// 		// The peerConnection has been closed.
			// 		return	
			// 	}
			// 	panic(err)
			// }
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

func (p *PiRTC) Snapshot(fileName string){
	// Initialized with 20 maxLate, my samples sometimes 10-15 packets
	sampleBuild := samplebuilder.New(20, &codecs.VP8Packet{}, 90000)
	decoder := vp8.NewDecoder()

	p.snapChan <- struct{}{}
	defer func() { p.snapCompleteChan <- struct{}{} }()


	for{
		select{
		case RTPpacket := <-p.packetChan:
			sampleBuild.Push(RTPpacket)

			samplePop := sampleBuild.Pop()
			if samplePop == nil {
				continue
			}

			// Read VP8 header.
			videoKeyframe := (samplePop.Data[0]&0x1 == 0)
			if videoKeyframe {
				decoder.Init(bytes.NewReader(samplePop.Data), len(samplePop.Data))
				frameHead, err := decoder.DecodeFrameHeader()
				if err != nil {
					log.Println( "DecodeFrameHeader ERROR")
					return
				}
				log.Printf("FrameHeader: %v",frameHead)
				img, err := decoder.DecodeFrame()
				if err != nil {

					log.Println("DecodeFrame ERROR")
					return
				}
				// Encode to (RGB) jpeg
				buffer := new(bytes.Buffer)
				err = jpeg.Encode(buffer, img, nil)
				if err != nil {
					return
				}

				log.Println("Encode img success")
				return
			}else{
				continue
			}
		}
	}
}

func clonePacket(packet *rtp.Packet) *rtp.Packet {
	buf, err := packet.Marshal()
	if err != nil {
		return nil
	}
	var p rtp.Packet
	err = p.Unmarshal(buf)
	if err != nil {
		return nil
	}
	return &p
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

