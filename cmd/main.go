package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/GARMA-A/redisgo/internal/server"
	"github.com/GARMA-A/redisgo/internal/store"
)

func main() {
	db := store.New()
	port := flag.String("port", "6379", "Port to listen on")
	flag.Parse()

	portNum, err := strconv.Atoi(*port)
	if err != nil || portNum < 1 || portNum > 65535 {
		fmt.Fprintf(os.Stderr, "Invalid --port value %q: must be an integer between 1 and 65535\n", *port)
		os.Exit(1)
	}

	address := net.JoinHostPort("0.0.0.0", strconv.Itoa(portNum))
	l, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to bind to %s: %v\n", address, err)
		os.Exit(1)
	}
	defer l.Close()
	fmt.Printf("Redis Go Server started on %s\n", address)
	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		go server.HandleClient(conn, db)

	}
}
