package client

import (
	"context"
	"log"
	"net"

	"github.com/lbodlev888/switcher/network"
)

func RunClient(ctx context.Context, dstAddr, serverAddr string) {
	destRaw, err := network.EncodeDestination(dstAddr)
	if err != nil {
		log.Println("Failed to encode: " + err.Error())
		return
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Println("Failed to bind local random address: " + err.Error())
		return
	}
	defer listener.Close()

	log.Printf("Local listener started on %s\nWaiting for local interaction...", listener.Addr().String())

	go func(){
		<-ctx.Done()
		listener.Close()
	}()

	client, err := listener.Accept()
	if err != nil {
		log.Println("Could not accept local client: " + err.Error())
		return
	}
	defer client.Close()

	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		log.Println("Failed to dial to server: " + err.Error())
		return
	}
	defer conn.Close()

	conn.Write(destRaw)

	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	if err != nil {
		log.Println("Failed to read the result from switch server: " + err.Error())
		return
	}

	if buf[0] != 0x01 {
		switch buf[0] {
		case 0x02:
			log.Println("server could not read destination address")
		case 0x03:
			log.Println("server received an invalid destination address packet")
		case 0x04:
			log.Println("server could not decode destination address")
		case 0x05:
			log.Println("server could not dial destination address")
		}
		return
	}

	go func(){
		<-ctx.Done()
		client.Close()
		conn.Close()
	}()

	log.Printf("Relay to %s via %s started\n", dstAddr, serverAddr)
	network.Relay(client, conn)
	log.Printf("Relay to %s via %s ended\n", dstAddr, serverAddr)
}
