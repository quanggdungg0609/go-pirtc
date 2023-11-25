package types

// interface of the websocket message
type Message struct {
	Event   string      `json:"event"`
	Payload interface{} `json:"payload"`
}

// interface of the SDP
type SessionDescription struct {
	Type string `json:"type"`
	Sdp  string `json:"sdp"`
}
