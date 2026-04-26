package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/GARMA-A/redisgo/internal/server"
	"github.com/GARMA-A/redisgo/internal/store"
)

func main() {
	port := flag.String("port", "6379", "Port to listen on")
	replicaof := flag.String("replicaof", "", "Address of master to replicate from (host:port)")

	flag.Parse()

	if *replicaof != "" {
		hostPortArr := strings.Fields(*replicaof)
		if len(hostPortArr) != 2 {
			fmt.Fprintf(os.Stderr, "Invalid --replicaof value %q: must be in the format host:port\n", *replicaof)
			os.Exit(1)
		}
		if _, err := strconv.Atoi(hostPortArr[1]); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid port in --replicaof value %q: %v\n", *replicaof, err)
			os.Exit(1)
		}
		conn, err := net.Dial("tcp", net.JoinHostPort(hostPortArr[0], hostPortArr[1]))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to connect to master at %s: %v\n", *replicaof, err)
			os.Exit(1)
		}
		conn.Write([]byte("*2\r\n$3\r\nPING\r\n"))
		conn.Close()
	}

	replicationId := "8371b4fb1155b71f4a04d3e1bc3e18c4a990aeeb"
	offset := int64(0)

	portNum, err := strconv.Atoi(*port)
	if err != nil || portNum < 1 || portNum > 65535 {
		fmt.Fprintf(os.Stderr, "Invalid --port value %q: must be an integer between 1 and 65535\n", *port)
		os.Exit(1)
	}

	db := store.New()
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
		go server.HandleClient(conn, db, *replicaof, replicationId, offset)

	}
}
