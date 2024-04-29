package unixsocket

import (
	"net"
	"sync"
)

// * Act like a Unix Socket client
type UnixSocket struct {
	mu   sync.Mutex
	conn net.Conn
}

func InitSocket(unixFilePath string) (*UnixSocket, error) {
	conn, err := net.Dial("unix", unixFilePath)
	if err != nil {
		return nil, err
	}

	return &UnixSocket{
		conn: conn,
	}, nil
}

func (us *UnixSocket) ListenAndServe(callback func(interface{}), disChan chan struct{}) {
	buf := make([]byte, 1024) // buffer
	for {
		select {
		case <-disChan:
			us.conn.Close()
			return
		default:
			n, err := us.conn.Read(buf)
			if err != nil {
				panic(err)
			}

			callback(buf[:n])
		}
	}
}

func (us *UnixSocket) Write(message string) {
	us.mu.Lock()
	defer us.mu.Unlock()
	_, err := us.conn.Write([]byte(message))
	if err != nil {
		panic(err)
	}
}
