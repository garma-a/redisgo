package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Value struct {
	data      string
	expiresAt time.Time
}

var (
	dataStore = make(map[string]Value)
	mu        sync.RWMutex
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
	// The for loop:
	//
	// First pass (i=0): It sees $4. It confirms it starts with $. It skips the $4 line and grabs the next line: "ECHO".
	//
	// Second pass (i=1): It sees $2. It skips it and grabs the next line: "HI".
	//
	// result = append(result, parts[idx]):
	// It adds these words to a Go slice: ["ECHO", "HI"].
	//
	// return result, nil:
	// It sends the clean list of words back to the handler.
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
			switch command {
			case "PING":
				conn.Write([]byte("+PONG\r\n"))
			case "ECHO":
				if len(parts) > 1 {

					response := fmt.Sprintf("$%d\r\n%s\r\n", len(parts[1]), parts[1])
					conn.Write([]byte(response))

				}
			case "SET":
				if len(parts) < 3 {
					conn.Write([]byte("-ERR wrong number of arguments\r\n"))
					continue
				}

				key := parts[1]
				val := parts[2]
				var expiry time.Time

				// Check for PX (milliseconds) or EX (seconds)
				if len(parts) >= 5 {
					option := strings.ToUpper(parts[3])
					expiryValue, _ := strconv.Atoi(parts[4])

					if option == "EX" {
						expiry = time.Now().Add(time.Duration(expiryValue) * time.Second)
					} else if option == "PX" {
						expiry = time.Now().Add(time.Duration(expiryValue) * time.Millisecond)
					}
				}

				mu.Lock()
				dataStore[key] = Value{
					data:      val,
					expiresAt: expiry,
				}
				mu.Unlock()
				conn.Write([]byte("+OK\r\n"))

			case "GET":
				key := parts[1]

				mu.RLock()
				val, exists := dataStore[key]
				mu.RUnlock()

				if exists && !val.expiresAt.IsZero() && time.Now().After(val.expiresAt) {
					mu.Lock()
					delete(dataStore, key)
					mu.Unlock()
					exists = false
				}

				if exists {
					conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(val.data), val.data)))
				} else {
					conn.Write([]byte("$-1\r\n"))
				}
			}
		}
	}
}
