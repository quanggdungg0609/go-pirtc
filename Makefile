SOURCE := main
GO_CROSS_COMPILER_PATH := /usr/local/go/bin
# Build command for Raspberry Pi
BUILD_CMD := env GOOS=linux GOARCH=arm $(GO_CROSS_COMPILER_PATH)/go build  ./cmd/app/main.go 




.PHONY: install-go install-deps build run

install-go:
	wget https://go.dev/dl/go1.22.3.linux-armv6l.tar.gz
	sudo tar -C /usr/local -xzf go1.22.3.linux-armv6l.tar.gz
	echo "PATH=$PATH:/usr/local/go/bin" >> $HOME/.bashrc
	echo "GOPATH=$HOME/go" >> $HOME/.bashrc
	source $HOME/.bashrc

install-deps:
	sudo apt install libvpx-dev -y

build:
	$(BUILD_CMD)
run:
	./main

