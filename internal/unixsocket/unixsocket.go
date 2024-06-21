package unixsocket

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"runtime"
	"strings"
)

type UnixSocketClient struct{
	pid int
	isConnected bool
	socketClient net.Conn
	
}


func (us *UnixSocketClient) Init(socketPath string) error{
	var err error
	us.pid = os.Getegid()
	us.socketClient, err= net.Dial("unix",socketPath)
	if err!=nil{
		return err
	}
	us.isConnected = true
	return nil
}

func (us *UnixSocketClient) ListenAndServe(handleMessageMap map[string]map[string]func(string), disconnect chan struct{}){
	for{
		select{
		case <-disconnect:
			_ =us.socketClient.Close()
			us.isConnected = false
			return
		default:
			buf:= make([]byte, 1024)
			byteRead, err:= us.socketClient.Read(buf)
			if(err!=nil){
				if (err == io.EOF){
					log.Println("[Unix Socket] - Connection closed by server")
					return
				}else{
					return
				}
			}
			data:= string(buf[:byteRead])
			if data == "HANDSHAKE" {
				handshakeMessage := fmt.Sprintf("HANDSHAKE %d", us.pid)
				_, err = us.socketClient.Write([]byte(handshakeMessage))
				if err != nil {
					fmt.Println("Error sending handshake message:", err.Error())
					break
				}
			}else{
				parts := strings.SplitN(data, " ", 3)
				if len(parts) >= 2{
					typeAction := parts[0]
					action := parts[1]
					param := ""
					if len(parts) == 3{
						param = parts[2]
					}
					if actionFuncs, ok := handleMessageMap[typeAction]; ok {
                        if actionFunc, ok := actionFuncs[action]; ok {
                            actionFunc(param)
                        } else {
                            log.Printf("Unknown action %s for typeAction %s\n", action, typeAction)
                        }
                    } else {
                        log.Printf("Unknown typeAction %s\n", typeAction)
                    }

				}else{
					log.Println(data)
				}
			}
			runtime.Gosched()
		}
	}
}

func (us *UnixSocketClient) SendMessage(message string) error {
    messageWithPID := fmt.Sprintf("[%d] %s", us.pid, message)
	_, err := us.socketClient.Write([]byte(messageWithPID))
	if err != nil {
		return err
	}
	return nil
}