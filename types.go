package main

// interface of the websocket message
type Message struct {
	Event   string      `json:"event"`
	Payload interface{} `json:"payload"`
}
