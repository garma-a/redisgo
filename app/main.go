package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync/atomic"
)

var clientCount int64 = 0

func main() {
	l, err := net.Listen("tcp", "0.0.0.0:6379")
	if err != nil {
		fmt.Println("Failed to bind to port 6379")
		os.Exit(1)
	}
	defer l.Close()

	fmt.Println("Server is listening on port 6379...")

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			continue
		}
		newCount := atomic.AddInt64(&clientCount, 1)
		fmt.Printf("New client connected. Total clients: %d\n", newCount)

		go handleClient(conn)
	}
}

func handleClient(conn net.Conn) {
	defer func() {
		conn.Close()
		newCount := atomic.AddInt64(&clientCount, -1)
		fmt.Printf("Client disconnected. Total clients: %d\n", newCount)
	}()
	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				break // Client disconnected gracefully
			}
			fmt.Println("Read error:", err)
			break
		}

		if string(buf[:n]) == "*1\r\n$4\r\nPING\r\n" {
			conn.Write([]byte("+PONG\r\n"))
		} else if n > 0 {
			conn.Write([]byte(buf[:n]))
		}
	}
}
