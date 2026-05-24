package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"

	"github.com/hashicorp/yamux"
	"github.com/lbodlev888/switcher/network"
)

var (
	serverAddr = flag.String("switch", "", "Address of the switching server")
)

func main() {
	flag.Parse()

	if *serverAddr == "" {
		log.Println("switching server addr cannot be empty")
		flag.Usage()
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	conn, err := net.Dial("tcp", *serverAddr)
	if err != nil {
		log.Println("Failed to dial to server: " + err.Error())
		return
	}

	session, err := yamux.Client(conn, yamux.DefaultConfig())
	if err != nil {
		log.Println("Failed to init mux session: " + err.Error())
		return
	}
	defer session.Close()

	controlStream, err := session.Open()
	if err != nil {
		log.Println("Failed to open controlStream: " + err.Error())
		return
	}
	defer controlStream.Close()

	reader := bufio.NewReader(os.Stdin)

	for {
		if ctx.Err() != nil {
			return
		}

		id, err := getID(reader)
		if err != nil {
			continue
		}

		listener, err := net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			log.Println("could not bind addr: " + err.Error())
			continue
		}
		log.Printf("Listening on: %d\nWaiting for local interaction...\n", listener.Addr().(*net.TCPAddr).Port)
		client, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Println("Failed to accept client: " + err.Error())
			continue
		}

		go handleStream(ctx, session, controlStream, client, id)
	}
}

func handleStream(ctx context.Context, session *yamux.Session, controlStream, client net.Conn, id string) {
	defer client.Close()

	if err := network.SendData(controlStream, []byte(id)); err != nil {
		log.Println("Failed to send id: " + err.Error())
		return
	}
	status, err := network.ReadData(controlStream)
	if err != nil {
		log.Println("Failed to read request status: " + err.Error())
		return
	}
	if string(status) != "ok" {
		log.Println("Server refused: " + string(status))
		return
	}

	dataStream, err := session.Open()
	if err != nil {
		log.Println("Failed to open data stream: " + err.Error())
		return
	}
	defer dataStream.Close()

	go func() {
		<-ctx.Done()
		client.Close()
		dataStream.Close()
	}()

	log.Println("Relay started")
	network.Relay(dataStream, client)
	log.Println("Relay ended")
}

func getID(reader *bufio.Reader) (string, error) {
	fmt.Print("target id: ")
	text, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Failed to read target id: " + err.Error())
		return "", err
	}
	id := strings.TrimSpace(text)
	if id == "" {
		return "", fmt.Errorf("getID: target id cannot be empty")
	}

	return id, nil
}
