package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

func parseRESP(data []byte) ([]string, error) {
	// Imagine the client sends: *2\r\n$4\r\nECHO\r\n$2\r\nHI\r\n
	//
	// parts := strings.Split(str, "\r\n"):
	// This turns the string into an array: ["*2", "$4", "ECHO", "$2", "HI", ""].
	// if parts[0][0] == '*':
	// It checks the very first character. * tells the parser: "An Array is coming!"
	//
	// numElements, _ := strconv.Atoi(parts[0][1:]):
	// It looks at *2, skips the *, and converts the 2 into an integer. Now the code knows it needs to find 2 words
	str := string(data)
	parts := strings.Split(str, "\r\n")
	if len(parts) < 1 {
		return nil, fmt.Errorf("empty input")
	}
	if parts[0][0] == '*' {
		numElements, err := strconv.Atoi(parts[0][1:])
		if err != nil {
			return nil, fmt.Errorf("invalid array format")
		}
		result := []string{}
		idx := 1
		for i := 0; i < numElements; i++ {
			if idx >= len(parts) {
				return nil, fmt.Errorf("incomplete data")
			}
			if parts[idx][0] != '$' {
				return nil, fmt.Errorf("expected bulk string")
			}
			_, err := strconv.Atoi(parts[idx][1:])
			if err != nil {
				return nil, fmt.Errorf("invalid length")
			}
			idx++
			if idx >= len(parts) {
				return nil, fmt.Errorf("incomplete data")
			}
			result = append(result, parts[idx])
			idx++
		}
		return result, nil
	}
	return nil, fmt.Errorf("unsupported format")
}

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
		go handleClient(conn)
	}
}
func handleClient(conn net.Conn) {
	defer func() {
		conn.Close()
	}()
	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Println("Read error:", err)
			break
		}

		parts, err := parseRESP(buf[:n])
		if err == nil && len(parts) > 0 {
			command := strings.ToUpper(parts[0])
			if command == "PING" {
				conn.Write([]byte("+PONG\r\n"))
			} else if command == "ECHO" && len(parts) > 1 {
				response := fmt.Sprintf("$%d\r\n%s\r\n", len(parts[1]), parts[1])
				conn.Write([]byte(response))
			}
		}
	}
}
