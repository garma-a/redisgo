package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/GARMA-A/redisgo/internal/server"
	"github.com/GARMA-A/redisgo/internal/store"
)

func main() {
	db := store.New()
	port := flag.String("port", "6379", "Port to listen on")
	flag.Parse()
	l, err := net.Listen("tcp", "0.0.0.0:"+*port)
	if err != nil {
		fmt.Println("Failed to bind to port 6379")
		os.Exit(1)
	}
	defer l.Close()
	fmt.Println("Redis Go Server started on :6379")

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		go server.HandleClient(conn, db)

	}
}
