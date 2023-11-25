package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/pion/mediadevices/pkg/driver/camera"

	"gitlab.lanestel.net/quangdung/gortc/internals/types"
	ws "gitlab.lanestel.net/quangdung/gortc/internals/websocket"
	"gitlab.lanestel.net/quangdung/gortc/internals/wrtc"
	"gitlab.lanestel.net/quangdung/gortc/utils"
)

var socketMutex = sync.Mutex{}
var socketCond = sync.NewCond(&socketMutex)
var socket *ws.WS
var piRTC *wrtc.WRTC

func main() {
	//load .env
	var err error
	err = godotenv.Load()
	if err != nil {
		log.Printf("[Load .env Error ]: %v", err)
	}
	// generate UUID if not exist
	err = generateUuid()
	if err != nil {
		log.Printf("[generateUuid Error]: %v", err)
	}

	log.Printf("UUID Device: %v", os.Getenv("UUID"))
	log.Printf("MAC Address: %v", os.Getenv("MAC_ADDR"))
	log.Println("[GOPIRTC]: Starting...")

	// request API Key from the server in the first time exec
	utils.RequestAPIKey()

	// chanel reserve to quit the program
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	stop := make(chan struct{})

	// bind the api-key into the header
	header := http.Header{}
	header.Set("api-key", os.Getenv("API_KEY"))

	//connect and broadcast the state of websocket to orther goroutine
	socketCond.L.Lock()
	socket, err = ws.Connect(os.Getenv("WS_URI"), header)
	socketCond.Broadcast()
	socketCond.L.Unlock()

	// init a list peer webrtc ready to use
	piRTC = wrtc.InitRTC()

	if err != nil {
		log.Printf("[Websocket Connect Error]: %v", err)
		// if connection error create a go routine to trying reconnect
		go func() {
			attemp := 1
			for {
				log.Printf("[GOROUTINE Reconnect]: Trying to reconnect to server (%v)", attemp)
				time.Sleep(time.Second * 3)
				// trying to reconnect and broadcast the state of websocket to orther goroutine
				socketCond.L.Lock()
				socket, err = ws.Connect(os.Getenv("WS_URI"), header)
				socketCond.Broadcast()
				socketCond.L.Unlock()
				attemp++
				if err == nil {
					log.Printf("[GOROUTINE Reconnect]: Connect successful at %vth time", attemp)
					break
				}
			}
		}()
	}

	// go routine to listen message and handle event
	go func() {
		for {
			select {
			default:
				socketCond.L.Lock()
				for socket == nil {
					socketCond.Wait()
				}
				socketCond.L.Unlock()

				message := socket.ListenMessage()
				handleEvent(message, socket)
			}
			runtime.Gosched()
		}
	}()

	for {
		select {
		case <-quit:
			log.Println("Quitting...")
			close(stop)
			// wg.Wait()
			os.Exit(0)
		}
		runtime.Gosched()
	}
}

// Verify if uuid exist in .env file
// if not create a new one and add into .env file
func generateUuid() error {
	err := godotenv.Load()
	if err != nil {
		return err
	}

	// check if exist
	envMap, err := godotenv.Read()
	if err != nil {
		return err
	}

	_, exist := envMap["UUID"]
	if !exist {
		// write a UUID into
		uuid := uuid.New().String()
		envMap["UUID"] = uuid
		err = godotenv.Write(envMap, ".env")
		if err != nil {
			return err
		}
	}
	return nil
}

// handle the actions depend the event message received
func handleEvent(message types.Message, ws *ws.WS) {
	switch message.Event {
	case "welcome":
		type Payload struct {
			UUID     string `json:"uuid"`
			Name     string `json:"name"`
			Location string `json:"location"`
		}

		payload := Payload{
			UUID:     os.Getenv("UUID"),
			Name:     os.Getenv("NAME"),
			Location: os.Getenv("LOCATION"),
		}

		data := types.Message{
			Event:   "register",
			Payload: payload,
		}
		err := ws.SendMessage(data)
		if err != nil {
			log.Printf("[handleEvent Error] %v", err)
		}
		break
	case "new-client-connected":

		break
	default:
		log.Println("[handleEvent]: Invalid event")
	}
}

// // connect to the websocket server, if failed to connect trying to reconnect
// for {
// 	err := initWS()
// 	if err != nil {
// 		log.Println(err)
// 		log.Println("Trying to reconnect after 3 seconds...")
// 		time.Sleep(3 * time.Second)
// 	} else {
// 		break
// 	}
// }
// message := Message{
// 	Event:   "ready",
// 	Payload: nil,
// }

// b, err := json.Marshal(message)
// if err != nil {
// 	log.Println(err)
// }

// err = wsClient.WriteMessage(websocket.TextMessage, b)
// if err != nil {
// 	log.Println(err)
// }
// go func() {
// 	listenMessageWorker(stop)
// }()
