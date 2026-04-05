package server

import (
	"fmt"
	"github.com/GARMA-A/redisgo/internal/resp"
	"github.com/GARMA-A/redisgo/internal/store"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

func HandleClient(conn net.Conn, db *store.DB) {
	defer conn.Close()
	buf := make([]byte, 1024)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				fmt.Println("Read error:", err)
			}
			break
		}

		parts, err := resp.Parse(buf[:n])
		if err != nil {
			continue
		}

		command := strings.ToUpper(parts[0])
		switch command {
		case "PING":
			conn.Write([]byte("+PONG\r\n"))
		case "ECHO":
			conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(parts[1]), parts[1])))
		case "SET":
			handleSet(conn, db, parts)
		case "GET":
			handleGet(conn, db, parts)
		}
	}
}

func handleSet(conn net.Conn, db *store.DB, parts []string) {
	if len(parts) < 3 {
		conn.Write([]byte("-ERR wrong number of arguments\r\n"))
	}
	key := parts[1]
	val := parts[2]
	var expiry time.Time

	if len(parts) >= 5 {
		option := strings.ToUpper(parts[3])
		expiryValue, _ := strconv.Atoi(parts[4])

		if option == "EX" {
			expiry = time.Now().Add(time.Duration(expiryValue) * time.Second)
		} else if option == "PX" {
			expiry = time.Now().Add(time.Duration(expiryValue) * time.Millisecond)
		}
	}
	db.Set(key, val, expiry)
	conn.Write([]byte("+OK\r\n"))
}

func handleGet(conn net.Conn, db *store.DB, parts []string) {
	val, exists := db.GetWithTTL(parts[1])
	if exists {
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(val), val)))
	} else {
		conn.Write([]byte("$-1\r\n"))
	}
}
