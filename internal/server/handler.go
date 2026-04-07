package server

import (
	"fmt"
	"github.com/GARMA-A/redisgo/internal/resp"
	"github.com/GARMA-A/redisgo/internal/store"
	"io"
	"net"
	"strings"
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
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		command := strings.ToUpper(parts[0])
		switch command {
		case "PING":
			conn.Write([]byte("+PONG\r\n"))
		case "ECHO":
			if len(parts) < 2 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}
			conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(parts[1]), parts[1])))
		case "SET":
			if len(parts) < 3 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}
			handleSet(conn, db, parts)
		case "GET":
			if len(parts) < 2 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}
			handleGet(conn, db, parts)
		case "RPUSH":
			if len(parts) < 3 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}
			handleRPush(conn, db, parts)
		case "LRANGE":
			if len(parts) < 4 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}
			handleLRange(conn, db, parts)
		case "LPUSH":
			if len(parts) < 3 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}
			handleLPush(conn, db, parts)
		}
	}

}
