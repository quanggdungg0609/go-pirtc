install-go:
	wget https://go.dev/dl/go1.22.3.linux-armv6l.tar.gz
	sudo tar -C /usr/local -xzf go1.22.3.linux-armv6l.tar.gz
	echo "PATH=$PATH:/usr/local/go/bin" >> $HOME/.bashrc
	echo "GOPATH=$HOME/go" >> $HOME/.bashrc
	source $HOME/.bashrc

install-deps:
	sudo apt install libvpx-dev -y

build:
	go build cmd/app/main.go 
run:
	./main

