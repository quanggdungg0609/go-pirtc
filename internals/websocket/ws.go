package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"gitlab.lanestel.net/quangdung/gortc/internals/types"
)

// need to modify
type WS struct {
	ws      *websocket.Conn
	uri     string
	headers http.Header
}

// connect to the websocket with the given headers
// return the WS object
func Connect(uri string, header http.Header) (*WS, error) {
	// create a new ws
	ws := WS{
		ws:      nil,
		uri:     uri,
		headers: nil,
	}
	var err error
	if header != nil {
		ws.ws, _, err = websocket.DefaultDialer.Dial(ws.uri, header)
		if err != nil {
			return nil, err
		}
		ws.headers = header
	} else {
		ws.ws, _, err = websocket.DefaultDialer.Dial(ws.uri, nil)
		if err != nil {
			return nil, err
		}
	}
	return &ws, nil
}

func (ws *WS) SendMessage(message interface{}) error {
	// marshal the message (stringtify)
	b, err := json.Marshal(message)
	if err != nil {
		return err
	}

	// emit message to server
	err = ws.ws.WriteMessage(websocket.TextMessage, b)
	if err != nil {
		return err
	}
	return nil
}

// listen message from server, if received a CloseError, trying to reconnect
func (ws *WS) ListenMessage() types.Message {
	_, rawMessage, err := ws.ws.ReadMessage()
	if err != nil {
		ws.reconnect()
	}
	var message types.Message
	json.Unmarshal(rawMessage, &message)
	return message
}

// reconnect function
func (ws *WS) reconnect() {
	err := ws.ws.Close()
	if err != nil {
		log.Printf("[Reconnect Error]: %v", err)
	}
	attemp := 0
	for {
		attemp++
		log.Printf("[reconnect]: Trying to reconnect to the server... (%v)", attemp)
		ws.ws, _, err = websocket.DefaultDialer.Dial(ws.uri, ws.headers)
		if err != nil {
			log.Println(err)
			time.Sleep(3 * time.Second)
		} else {
			break
		}
	}
}
