package main

import (
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync"

	"gitlab.lanestel.net/quangdung/gortc/utils"
)

func main() {

	// request API Key from the server in the first time exec
	utils.RequestAPIKey()

	// chanel reserve to quit the program
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	initWS()
	go func() {
		defer wg.Done()
		listenMessageWorker(stop)
	}()

	for {
		select {
		case <-quit:
			log.Println("Quitting...")
			close(stop)
			wg.Wait()
			os.Exit(0)
		}
		runtime.Gosched()
	}
}
