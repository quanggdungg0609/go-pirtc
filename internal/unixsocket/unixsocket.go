package unixsocket

import (
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
)

type UnixSocketClient struct{
	pid int
	socketClient net.Conn
}


func (us *UnixSocketClient) Init(socketPath string) error{
	var err error
	us.pid = os.Getegid()
	us.socketClient, err= net.Dial("unix",socketPath)
	if err!=nil{
		return err
	}
	return nil
}

func (us *UnixSocketClient) ListenAndServe(handleMessage func(string), disconnect chan struct{}){
	for{
		select{
		case <-disconnect:
			_ =us.socketClient.Close()
			return

		default:
			buf:= make([]byte, 1024)
			byteRead, err:= us.socketClient.Read(buf)
			if(err!=nil){
				if (err == io.EOF){
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
				handleMessage(data)
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