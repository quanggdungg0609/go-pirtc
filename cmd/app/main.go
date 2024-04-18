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

	// log.Println(env.Uuid)

	prtc, err := pirtc.Init()
	if err != nil {
		panic(err)
	}

	// test functionaly of camera(record, take shot and upload img to server)
	// go func() {
	// 	err := prtc.RecordWithTimer(env.VideoPath, time.Duration(10)*time.Second)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// }()

	// // take a shot
	// time.AfterFunc(time.Duration(5)*time.Second, func() {
	// 	if err := prtc.TakeShot(env.Uuid); err != nil {
	// 		panic(err)
	// 	}
	// 	log.Println("apiUri: ", env.ApiUri+"cameras/upload-thumbnail")
	// 	log.Println("pathFile: ", env.Uuid+".jpeg")

	// 	err = utils.UploadImage(env.ApiUri+"cameras/upload-image", env.Uuid+".jpeg")
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// })

	// connect to websocket
	wsClient, err := ws.Connect(env.WsUri, nil)
	if err != nil {
		panic(err)
	}

	//create callbacks for each event
	callbacks := createCallBacks(prtc)

	// register with server
	payload := map[string]string{
		"uuid":     env.Uuid,
		"name":     env.Name,
		"location": env.Location,
	}
	wsClient.EmitMessage("camera-connect", payload)

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

	callbacks["user-connect"] = func(data interface{}) {
		// data will return @map[string]interface{} with uuid of the new user connect
		err := prtc.NewUser(data.(map[string]interface{})["uuid"].(string))
		if err != nil {
			log.Printf("[user-connect error]: %v\n", err)
		}
		log.Println(prtc.Connections)
		// log.Println(data)
		// pr
	}

	callbacks["user-disconnect"] = func(data interface{}) {
		err := prtc.UserDisconnect(data.(map[string]interface{})["uuid"].(string))
		if err != nil {
			log.Printf("[user-connect error]: %v\n", err)
		}
		log.Println(prtc.Connections)

	}

	callbacks["request-list-users"] = func(data interface{}) {

	}

	callbacks["offer-sd"] = func(data interface{}) {

	}

	callbacks["ice-candidate"] = func(data interface{}) {}

	return callbacks
}
