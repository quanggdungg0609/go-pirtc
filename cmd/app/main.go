package main

import (
	readenv "gitlab.lanestel.net/quangdung/go-pirtc/internal/read_env"
	"gitlab.lanestel.net/quangdung/go-pirtc/internal/ws"
)

var disconnectChan = make(chan struct{})

func main() {
	env, err := readenv.ReadEnv()
	if err != nil {
		panic(err)
	}
	wsClient, err := ws.Connect(env.WsUri, nil)
	if err != nil {
		panic(err)
	}
	callbacks := createCallBacks()

	go wsClient.ListenAndServe(callbacks, disconnectChan)


}


func createCallBacks() map[string]func(interface{}){
	callbacks := make(map[string]func(interface{}))

	callbacks[""] = func(i interface{}) {}

	return callbacks
}
