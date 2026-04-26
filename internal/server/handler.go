package server

import (
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/GARMA-A/redisgo/internal/resp"
	"github.com/GARMA-A/redisgo/internal/store"
)

func HandleClient(conn net.Conn, db *store.DB) {
	defer conn.Close()

	var weAreInsideMulti = false
	var multiCommands [][]string

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
			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(parts[1]), parts[1])))
		case "SET":
			if len(parts) < 3 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}

			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			handleSet(conn, db, parts)
		case "GET":
			if len(parts) < 2 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}

			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			handleGet(conn, db, parts)
		case "RPUSH":
			if len(parts) < 3 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}

			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			handleRPush(conn, db, parts)
		case "LRANGE":
			if len(parts) < 4 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}

			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			handleLRange(conn, db, parts)
		case "LPUSH":
			if len(parts) < 3 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}

			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			handleLPush(conn, db, parts)
		case "LLEN":
			if len(parts) < 2 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}

			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			handleLLEN(conn, db, parts)
		case "LPOP":
			if len(parts) < 2 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}

			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			handleLPop(conn, db, parts)
		case "BLPOP":
			if len(parts) < 3 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}

			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			handleBLPOP(conn, db, parts)
		case "XADD":
			if len(parts) < 5 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}

			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			handleXAdd(conn, db, parts)
		case "XRANGE":
			if len(parts) != 4 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}

			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			handleXRange(conn, db, parts)

		case "TYPE":
			if len(parts) < 2 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}

			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			handleType(conn, db, parts)

		case "INCR":
			if len(parts) < 2 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}

			if weAreInsideMulti {
				multiCommands = append(multiCommands, parts)
				continue
			}
			handleIncr(conn, db, parts)

		case "MULTI":
			if len(parts) != 1 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}
			weAreInsideMulti = true
			conn.Write([]byte("+OK\r\n"))
		}
	}
}
