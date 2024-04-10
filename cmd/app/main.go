package main

import (
	"log"
	"os"
	"os/signal"
	"runtime"

	"gitlab.lanestel.net/quangdung/go-pirtc/internal/pirtc"
	readenv "gitlab.lanestel.net/quangdung/go-pirtc/internal/read_env"
	"gitlab.lanestel.net/quangdung/go-pirtc/internal/ws"
)

func main() {
	// create channels
	var quitChan = make(chan os.Signal, 1)
	signal.Notify(quitChan, os.Interrupt)

	var disconnectChan = make(chan struct{})

	// read file .env
	env, err := readenv.ReadEnv()
	if err != nil {
		panic(err)
	}

	prtc := pirtc.Init()
	// connect to websocket
	wsClient, err := ws.Connect(env.WsUri, nil)
	if err != nil {
		panic(err)
	}
	//create callbacks for each event
	callbacks := createCallBacks(prtc)

	go wsClient.ListenAndServe(callbacks, disconnectChan)

	for {
		select {
		case <-quitChan:
			log.Println("Quitting....")
			close(disconnectChan)
			os.Exit(0)
		}
		runtime.Gosched()
	}
}

func createCallBacks(prtc *pirtc.PiRTC) map[string]func(interface{}) {
	callbacks := make(map[string]func(interface{}))

	callbacks["hello"] = func(payload interface{}) {
		log.Println("hello")
	}

	callbacks["new-client-connected"] = func(payload interface{}) {
		prtc.NewConnection(payload.(string))
	}

	callbacks["list-clients"] = func(payload interface{}) {

	}

	callbacks["client-disconnect"] = func(payload interface{}) {}

	callbacks["offer-sd"] = func(payload interface{}) {

	}

	callbacks["ice-candidate"] = func(payload interface{}) {}

	return callbacks
}
