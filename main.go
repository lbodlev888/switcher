package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/lbodlev888/switcher/client"
	"github.com/lbodlev888/switcher/server"
)

var (
	serverAddr = flag.String("switch", "", "Address of the switching server")
	destinationAddr = flag.String("dest", "", "Full destination address with IP and Port. e.g. 10.0.0.1:1234")
	bindAddr = flag.String("listen", ":3001", "The bind address to listen at")
	serverMode = flag.Bool("server", false, "Run in server mode. Defaults to false")
)

func main() {
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if *serverMode {
		server.RunServer(ctx, *bindAddr)
	}

	if *serverAddr == "" || *destinationAddr == "" {
		log.Println("Missing required params")
		flag.Usage()
		return
	}
	client.RunClient(ctx, *destinationAddr, *serverAddr)
}
