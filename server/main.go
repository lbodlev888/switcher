package server

import (
	"context"
	"log"
	"net"
	"sync"

	"github.com/lbodlev888/switcher/network"
)

var (
	wg sync.WaitGroup
)

func RunServer(ctx context.Context, bindAddr string) {
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		log.Println("Could not bind address: " + err.Error())
		return
	}
	defer listener.Close()

	log.Println("Server running on: " + bindAddr)

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
	defer conn.Close()
	dstBuf := make([]byte, 6)
	n, err := conn.Read(dstBuf)
	if err != nil {
		log.Println("handleClient: failed to read destination address: " + err.Error())
		conn.Write([]byte{0x00})
		return
	}
	if n != 6 {
		log.Printf("Invalid destination address packet length: required 6 got %d\n", n)
		conn.Write([]byte{0x00})
		return
	}

	dstAddr, err := network.DecodeDestination(dstBuf)
	if err != nil {
		log.Println("handleClient: failed to decode destination: " + err.Error())
		conn.Write([]byte{0x00})
		return
	}

	remoteConn, err := net.Dial("tcp", dstAddr)
	if err != nil {
		log.Printf("handleClient: Failed to dial %s: %s", dstAddr, err.Error())
		conn.Write([]byte{0x00})
		return
	}
	defer remoteConn.Close()
	conn.Write([]byte{0x01})

	wg.Go(func() {
		<-ctx.Done()
		conn.Close()
		remoteConn.Close()
	})

	log.Printf("Relay from %s to %s started\n", conn.RemoteAddr().String(), dstAddr)
	network.Relay(conn, remoteConn)
	log.Printf("Relay from %s to %s ended\n", conn.RemoteAddr().String(), dstAddr)
}
