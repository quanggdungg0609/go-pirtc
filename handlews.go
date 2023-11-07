package main

import (
	"log"
	"os"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

var wsClient *websocket.Conn

func initWS() {
	var err error
	err = godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	log.Println(os.Getenv("WS_URI"))
	wsClient, _, err = websocket.DefaultDialer.Dial(os.Getenv("WS_URI"), nil)
	if err != nil {
		log.Printf("Dial error: %v", err)
	}

	log.Println("Connected to server, waiting OfferSDP ...")
}

func listenMessageWorker(stop chan struct{}) {
	for {
		select {
		case <-stop:
			log.Println("Stopping listen message from websocket worker...")
			err := wsClient.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("Write close error:", err)
				return
			}
			return
		default:
			_, message, err := wsClient.ReadMessage()
			if err != nil {
				log.Println("Read message error:", err)
			}
			log.Printf("recv %v", message)
		}
	}
}
