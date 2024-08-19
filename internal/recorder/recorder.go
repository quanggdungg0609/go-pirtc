package recorder

import (
	"log"
	"sync"

	"github.com/pion/rtp"
	"gitlab.lanestel.net/quangdung/go-pirtc/internal/webmsaver"
)


type Recorder interface{
	Record(savePath string)
	StopRecord()
	Dispose()
}

type RTPRecorder struct{
	isRecord bool
	savePath string

	webmSaver *webmsaver.WebmSaver

	recordComplete chan struct{}
	recordPacketChan  chan *rtp.Packet

	mu sync.Mutex
}


func NewRecorder(packetChan chan *rtp.Packet) *RTPRecorder{
	recorder:= RTPRecorder{
		isRecord: false,
		savePath: "",
		webmSaver: nil,
		recordComplete: make(chan struct{}),
		recordPacketChan: packetChan,
	}
	return &recorder
}

func (r *RTPRecorder) Record(savePath string) {
	r.mu.Lock()
	r.savePath = savePath
	if r.webmSaver == nil{
		r.webmSaver = webmsaver.NewWebmSaver()
	}
	r.isRecord = true
	r.mu.Unlock()
	go r.handlePacket()
}

func (r *RTPRecorder) handlePacket(){
	for{
		select{
		case <-r.recordComplete:
			log.Printf("Video saved in: %v", r.savePath)
			r.mu.Lock()
			r.isRecord = false
			if r.webmSaver != nil {
				r.webmSaver.Close()
				r.webmSaver = nil
			}
			r.mu.Unlock()
			return
		case packet, ok := <-r.recordPacketChan:
			if !ok {
				log.Println("Record packet channel closed.")
				r.StopRecord()
				return
			}
			if r.webmSaver != nil {
				r.webmSaver.PushVP8(r.savePath, packet)
			}
		}
	}
}

func (r *RTPRecorder) StopRecord(){
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isRecord {
		r.recordComplete <- struct{}{}
	}
}

func (r *RTPRecorder) Dispose() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isRecord {
		r.StopRecord()
	}
	close(r.recordComplete)
}