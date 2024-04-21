package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime"

	"gitlab.lanestel.net/quangdung/go-pirtc/internal/pirtc"
	readenv "gitlab.lanestel.net/quangdung/go-pirtc/internal/read_env"
	"gitlab.lanestel.net/quangdung/go-pirtc/internal/ws"
)

type ContextKey string

const (
	PrtcKey ContextKey = "prtc"
	WsKey   ContextKey = "wsClient"
	EnvKey  ContextKey = "env"
)

func main() {
	// create channels
	var quitChan = make(chan os.Signal, 1)
	signal.Notify(quitChan, os.Interrupt)

	var disconnectChan = make(chan struct{})

	ctx := context.Background()

	// read file .env
	env, err := readenv.ReadEnv()
	if err != nil {
		panic(err)
	}

	ctx = context.WithValue(ctx, EnvKey, env)

	// log.Println(env.Uuid)

	prtc, err := pirtc.Init()
	if err != nil {
		panic(err)
	}

	ctx = context.WithValue(ctx, PrtcKey, prtc)
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

	ctx = context.WithValue(ctx, WsKey, wsClient)

	//create callbacks for each event
	callbacks := createCallBacks(ctx)

	// register with server
	payload := map[string]string{
		"uuid":     env.Uuid,
		"name":     env.Name,
		"location": env.Location,
	}
	wsClient.EmitMessage("camera-connect", payload)
	wsClient.EmitMessage("request-list-users", payload)
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

func createCallBacks(ctx context.Context) map[string]func(interface{}) {
	env := ctx.Value(EnvKey).(*readenv.Env)
	prtc := ctx.Value(PrtcKey).(*pirtc.PiRTC)
	wsClient := ctx.Value(WsKey).(*ws.WS)

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

	callbacks["response-list-users"] = func(data interface{}) {
		listUsers := data.([]interface{})
		for _, raw := range listUsers {
			user := raw.(map[string]interface{})
			prtc.NewUser(user["uuid"].(string))

		}
	}

	callbacks["offer-sd"] = func(data interface{}) {
		if prtc != nil {
			payload := data.(map[string]interface{})
			offerSd := pirtc.CreateSessionDescription(payload["type"].(string), payload["sdp"].(string))
			answerSd, err := prtc.Answer(payload["from"].(string), offerSd)
			if err != nil {
				panic(err)
			}
			log.Println(env.Uuid)

			data := map[string]string{
				"uuid": env.Uuid,
				"to":   payload["from"].(string),
				"type": answerSd.Type.String(),
				"sdp":  answerSd.SDP,
			}
			if wsClient != nil {
				err = wsClient.EmitMessage("answer-sd", data)
				if err != nil {
					log.Println(err)
				}
			}

		}
	}

	callbacks["ice-candidate"] = func(data interface{}) {}

	return callbacks
}
