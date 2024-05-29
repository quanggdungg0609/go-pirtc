package main

import (
	"context"
	"log"
	"net/http"

	"os"
	"os/signal"
	"runtime"
	"time"

	"gitlab.lanestel.net/quangdung/go-pirtc/internal/pirtc"
	readenv "gitlab.lanestel.net/quangdung/go-pirtc/internal/read_env"

	"gitlab.lanestel.net/quangdung/go-pirtc/internal/utils"
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
	log.Println(env.ApiKey)
	// log.Println(env.Uuid)

	prtc, err := pirtc.Init()
	if err != nil {
		panic(err)
	}

	ctx = context.WithValue(ctx, PrtcKey, prtc)

	// take a shot for the thumnail at the start
	go func() {
		if err := prtc.TakeShot(env.Uuid); err != nil {
			panic(err)
		}
		err = utils.UploadImage(env.ApiUri+"camera/upload-image/", env.Uuid+".jpeg", env.ApiKey)
		if err != nil {
			panic(err)
		}
		runtime.Gosched()
	}()

	// test functionaly of camera(record, take shot and upload img to server)
	go func() {
		dest := env.VideoPath + "/" + utils.GetCurrentTimeStr() + ".webM"
		doneChan := prtc.RecordWithTimer(dest, time.Duration(10)*time.Second)

		<-doneChan
		log.Printf("Video saved in: %v \n", dest)
		err := utils.UploadVideo(env.ApiUri+"camera/upload-video/", dest, env.Uuid, env.ApiKey)
		if err != nil {
			panic(err)
		}
		log.Println("Video Uploaded")

	}()

	// connect to websocket
	header := http.Header{}
	header.Set("api-key", env.ApiKey)
	log.Println(env.WsUri+"ws/camera/"+env.ApiKey+"/")
	wsClient, err := ws.Connect(env.WsUri+"ws/camera/"+env.ApiKey+"/", header)
	if err != nil {
		panic(err)
	}

	ctx = context.WithValue(ctx, WsKey, wsClient)

	//create callbacks for each event
	callbacks := createCallBacks(ctx)

	// register with server
	// payload := map[string]string{
	// 	"uuid":     env.Uuid,
	// 	"name":     env.Name,
	// 	"location": env.Location,
	// }
	// wsClient.EmitMessage("camera-connect", payload)
	wsClient.EmitMessage("request-list-users", map[string]string{})
	go wsClient.ListenAndServe(callbacks, disconnectChan)
	go callFunctionTimer(func() {

	}, 2, quitChan)

	for {
		select {
		case <-quitChan:
			log.Println("Quitting....")
			close(disconnectChan)
			os.Exit(0)
		default:

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
		log.Println(data.([]interface{}))
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

func callFunctionTimer(function func(), period int, quitChan chan os.Signal) {
	ticker := time.NewTicker(time.Duration(period) * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-quitChan:
			return
		case <-ticker.C:
			function()
		}
		runtime.Gosched()
	}
}
