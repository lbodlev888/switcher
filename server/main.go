package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/lbodlev888/switcher/network"
)

var (
	bindAddr = flag.String("listen", ":3001", "The bind address to listen at")
	templateAddr = flag.String("netaddr", "192.168.100.", "Template address that will be joined with input from control stream")
	remotePort = flag.String("rport", "554", "Remote port to connect to template address")
	wg sync.WaitGroup
)

func main() {
	flag.Parse()

	listener, err := net.Listen("tcp", *bindAddr)
	if err != nil {
		log.Println("Could not bind address: " + err.Error())
	}
	defer listener.Close()

	log.Println("Server running on: " + *bindAddr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	wg.Go(func() {
		<-ctx.Done()
		log.Println("Stop signal received. Stopping everything gracefully")
		listener.Close()
	})
	wg.Go(func() { acceptClients(ctx, listener) })

	wg.Wait()
}

func acceptClients(ctx context.Context, l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			log.Println("Could not accept client: " + err.Error())
			continue
		}

		wg.Go(func() { handleClient(ctx, conn) })
	}
}

func handleClient(ctx context.Context, conn net.Conn) {
	session, err := yamux.Server(conn, yamux.DefaultConfig())
	if err != nil {
		log.Println("Failed to start mux session: " + err.Error())
		return
	}
	defer session.Close()

	wg.Go(func() {
		<-ctx.Done()
		session.Close()
	})

	controlStream, err := session.Accept()
	if err != nil {
		log.Println("Failed to open control stream: " + err.Error())
		return
	}
	defer controlStream.Close()

	for {
		if ctx.Err() != nil {
			return
		}

		id, err := network.ReadData(controlStream)
		if err != nil {
			log.Println("Failed to read data: " + err.Error())
			return
		}
		finalAddr := fmt.Sprintf("%s%s:%s", *templateAddr, string(id), *remotePort)
		wg.Go(func() { startStream(ctx, session, controlStream, finalAddr) })
	}
}

func startStream(ctx context.Context, session *yamux.Session, ctrlStream net.Conn, addr string) {
	log.Println("Attempting to connect to " + addr)
	conn, err := net.DialTimeout("tcp", addr, 5 * time.Second)
	if err != nil {
		log.Printf("Could not connect to %s: %s\n", addr, err)
		network.SendData(ctrlStream, []byte(err.Error()))
		return
	}
	defer conn.Close()
	network.SendData(ctrlStream, []byte("ok"))
	log.Println("Connection successfully")

	dataStream, err := session.Accept()
	if err != nil {
		log.Println("Failed to init a new data stream: " + err.Error())
		return
	}
	defer dataStream.Close()

	wg.Go(func() {
		<-ctx.Done()
		conn.Close()
		dataStream.Close()
	})

	log.Println("Relay started")
	network.Relay(conn, dataStream)
	log.Println("Relay ended")
}
