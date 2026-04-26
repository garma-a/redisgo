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
		handleCommand(command, parts[1:], db, conn, parts)
	}
}

func handleCommand(command string, args []string, db *store.DB, conn net.Conn, parts []string) {
	var weAreInsideMulti = false
	var multiCommands [][]string

	switch command {
	case "PING":
		conn.Write([]byte("+PONG\r\n"))
	case "ECHO":
		if len(parts) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)
			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(parts[1]), parts[1])))
	case "SET":
		if len(parts) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleSet(conn, db, parts)
	case "GET":
		if len(parts) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleGet(conn, db, parts)
	case "RPUSH":
		if len(parts) < 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleRPush(conn, db, parts)
	case "LRANGE":
		if len(parts) < 3 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleLRange(conn, db, parts)
	case "LPUSH":
		if len(parts) < 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleLPush(conn, db, parts)
	case "LLEN":
		if len(parts) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleLLEN(conn, db, parts)
	case "LPOP":
		if len(parts) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleLPop(conn, db, parts)
	case "BLPOP":
		if len(parts) < 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleBLPOP(conn, db, parts)
	case "XADD":
		if len(parts) < 4 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleXAdd(conn, db, parts)
	case "XRANGE":
		if len(parts) != 3 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleXRange(conn, db, parts)

	case "TYPE":
		if len(parts) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleType(conn, db, parts)

	case "INCR":
		if len(parts) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, parts)
			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleIncr(conn, db, parts)

	case "MULTI":
		if len(parts) != 0 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		weAreInsideMulti = true
		conn.Write([]byte("+OK\r\n"))
	case "EXEC":
		if !weAreInsideMulti || len(parts) != 0 {
			conn.Write([]byte("-ERR EXEC without MULTI\r\n"))
		}
		if weAreInsideMulti && len(multiCommands) == 0 {
			weAreInsideMulti = false
			conn.Write([]byte("*0\r\n"))
		}
		for _, cmd := range multiCommands {
			strings.ToUpper(cmd[0])
			handleCommand(cmd[0], cmd[1:], db, conn, cmd)
		}
	}
}
