package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WS struct {
	ws     *websocket.Conn
	mu     sync.Mutex
	uri    string
	header http.Header
}

type WsMessage struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// * Connect to the websocket, if a header given connect with this header
func Connect(uri string, header http.Header) (*WS, error) {
	ws := WS{}
	var err error
	if header != nil {
		ws.uri = uri
		ws.header = header
		ws.ws, _, err = websocket.DefaultDialer.Dial(uri, header)
		if err != nil {
			log.Print(err)
			return nil, err
		}
	} else {
		ws.uri = uri
		ws.ws, _, err = websocket.DefaultDialer.Dial(uri, nil)
		if err != nil {
			log.Print(err)

			return nil, err
		}
	}
	return &ws, nil
}

func (ws *WS) EmitMessage(event string, payload interface{}) error {
	message := WsMessage{
		Event: event,
		Data:  payload,
	}

	bMessage, err := json.Marshal(message)
	if err != nil {
		return err
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	err = ws.ws.WriteMessage(websocket.TextMessage, bMessage)
	if err != nil {
		return err
	}
	return nil
}

func (ws *WS) ListenAndServe(callbacks map[string]func(interface{}), disconnect chan struct{}) {
	for {
		select {
		case <-disconnect:
			if ws.ws != nil {
				err := ws.ws.Close()
				if err != nil {
					log.Println("Error while closing Websocket connection: ", err)
				}
			}
			return
		default:
			_, rawMessage, err := ws.ws.ReadMessage()
			if err != nil {
				ws.reconnect()
				continue
			}

			var message WsMessage
			if err := json.Unmarshal(rawMessage, &message); err != nil {
				log.Println("Error while decode message", err)
				continue
			}

			if callback, ok := callbacks[message.Event]; ok {
				log.Printf("Received event [%s]\n", message.Event)
				callback(message.Data)
			} else {
				log.Printf("Received event [%s]: %s\n", message.Event, message.Data)
			}
		}
	}
}

func (ws *WS) reconnect() {
	// try to close the exist connection
	err := ws.ws.Close()
	if err != nil {
		log.Printf("Reconnect error : %v", err)
	}
	attemp := 0
	for {
		attemp++
		log.Printf("Trying to reconnect to the server ... (%v)", attemp)
		ws.ws, _, err = websocket.DefaultDialer.Dial(ws.uri, ws.header)
		if err != nil {
			log.Println(err)
			time.Sleep(3 * time.Second)
		} else {
			break
		}
	}
}
