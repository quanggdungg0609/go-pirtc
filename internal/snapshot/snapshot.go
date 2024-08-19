package snapshot

import "github.com/pion/rtp"


type Snapshot interface{

}

type RTPSnapshot struct{
	savePath string
	rtpPcketChan       chan *rtp.Packet
	snapCompleteChan chan struct{}
}

