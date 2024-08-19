package rtp_broker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"

	"github.com/pion/rtp"
)

type Broker interface{
	NewBroker(address string, port int) (*RTPBroker, error)
		// Start starts the RTPBroker.
	Start() error

	// Sub subscribes to RTP packets and returns a channel for receiving them.
	Sub(subID string) chan *rtp.Packet
	
	// UnSub unsubscribes from RTP packets for the given subscription ID.
	UnSub(subID string)
	
	// Stop stops the RTPBroker and cleans up resources.
	Stop()
}


type RTPBroker struct{
	port int
	address string

	isBroadcast bool
	cmd *exec.Cmd
    ctx context.Context
    cancel context.CancelFunc

	listerner *net.UDPConn
	subcriptions map[string]chan *rtp.Packet
	stopChan chan struct{}
	mu          sync.Mutex
}

func NewBroker(address string, port int) (*RTPBroker, error){
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(address), Port: port})
	if err != nil {
		return nil, err
	}

	if err := listener.SetReadBuffer(300000); err != nil {
		return nil, err
	}

	return &RTPBroker{
		port: port,
		address: address,
		listerner: listener,
		isBroadcast: false,

		subcriptions: make(map[string]chan *rtp.Packet),
		stopChan: make(chan struct{}),
		cmd: nil,
		ctx: nil,
		cancel: nil,
	}, nil
}


func (r *RTPBroker) Start() error{
	r.mu.Lock()

	if r.stopChan == nil{
		r.stopChan = make(chan struct{})
	}

	if r.cmd == nil  && r.ctx == nil && r.cancel == nil{
		r.ctx, r.cancel = context.WithCancel(context.Background())
		r.cmd = exec.CommandContext(r.ctx, "ffmpeg", "-f", "v4l2", "-i", "/dev/video0", "-vf", "format=yuv420p", "-c:v", "libvpx", "-deadline", "realtime", "-cpu-used", "3", "-b:v", "500k", "-f", "rtp", "rtp://"+r.address+":"+strconv.Itoa(r.port)+"?pkt_size=1200")
		r.cmd.Stdout = io.Discard
    	r.cmd.Stderr = io.Discard

		err := r.cmd.Start()
		if err != nil {
			return fmt.Errorf("failed to start ffmpeg: %v", err)
		}
		log.Println("FFmpeg process started")

		
	}
	r.isBroadcast = true
	r.mu.Unlock()

	go r.handleListeningRTP()
	return nil
}

func (r *RTPBroker) handleListeningRTP(){
	inboundRTPPacket := make([]byte, 1600)
	for{
		select{
		case <-r.stopChan:
			r.mu.Lock()
			r.isBroadcast = false
			r.mu.Unlock()
			return
		default:
			n, _, err := r.listerner.ReadFrom(inboundRTPPacket)
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
			// Send RTPPacket to all subscribers
			r.mu.Lock()
			// send RTPPacket to all subcriber
			for id, rtpPacketChan := range r.subcriptions{
				clonePacket:=cloneRTPPacket(packet)
				select {
				case rtpPacketChan <- clonePacket:
				default:
					// Nếu không thể gửi, có thể kênh đã bị đóng, loại bỏ nó
					close(rtpPacketChan)
					delete(r.subcriptions, id)
				}
			}
			r.mu.Unlock()
		}
		runtime.Gosched()
	}
}

func (r *RTPBroker) Sub(subID string) chan *rtp.Packet{
	r.mu.Lock()
	defer r.mu.Unlock()
	newChan := make(chan *rtp.Packet, 1000)
	r.subcriptions[subID] = newChan
	return newChan
}

func (r *RTPBroker) UnSub(subID string){
	r.mu.Lock()
	defer r.mu.Unlock()

	// Verify if ID is exists
	if ch, ok := r.subcriptions[subID]; ok {
		// close chanel
		close(ch)
		delete(r.subcriptions, subID)
	}else {
		log.Printf("Unsubscribe failed: subscription ID %s not found", subID)
	}
}

func (r *RTPBroker) Stop() error{
	r.mu.Lock()
	defer r.mu.Unlock()
	// stop diffusing data
	if r.isBroadcast{
		r.stopChan <- struct{}{}
		if r.cancel != nil {
			r.cancel()
			log.Println("Canceled FFmpeg context")
			if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
				log.Printf("Failed to send SIGTERM to ffmpeg: %v", err)
				return fmt.Errorf("failed to send SIGTERM to ffmpeg process: %v", err)
			}
			log.Println("SIGTERM signal sent to ffmpeg process")
		}
		r.cmd = nil
		r.cancel = nil
		r.ctx = nil
	}
	return nil
}

func (r *RTPBroker) Dispose() error{
	if r.isBroadcast {
		if err := r.Stop(); err != nil {
			return err
		}
	}

	// Close the UDP listener
	if err := r.listerner.Close(); err != nil {
		return fmt.Errorf("failed to close UDP listener: %v", err)
	}
	
	// Clean up subscription channels
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, ch := range r.subcriptions {
		close(ch)
		delete(r.subcriptions, id)
	}

	return nil
}


func cloneRTPPacket(packet *rtp.Packet) *rtp.Packet{
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

