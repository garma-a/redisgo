package main

import (
	"fmt"
	"io"
	"net"
	"os"
)

func main() {
	// 1. Bind to the port
	l, err := net.Listen("tcp", "0.0.0.0:6379")
	if err != nil {
		fmt.Println("Failed to bind to port 6379")
		os.Exit(1)
	}
	defer l.Close()

	// 2. Accept a single connection
	conn, err := l.Accept()
	if err != nil {
		fmt.Println("Error accepting connection: ", err.Error())
		os.Exit(1)
	}
	defer conn.Close()

	// 3. Loop to handle multiple commands on the same connection
	buf := make([]byte, 1024)
	for {
		// Read data from the connection
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				// Client closed the connection
				break
			}
			fmt.Println("Error reading from connection: ", err.Error())
			break
		}

		// Since the task says we can hardcode PONG for now:
		if n > 0 {
			conn.Write([]byte("+PONG\r\n"))
		}
	}
}
