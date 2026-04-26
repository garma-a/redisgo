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
		handleCommand(command, parts[1:], db, conn)
	}
}

func handleCommand(command string, args []string, db *store.DB, conn net.Conn) {
	var weAreInsideMulti = false
	var multiCommands [][]string

	switch command {
	case "PING":
		conn.Write([]byte("+PONG\r\n"))
	case "ECHO":
		if len(args) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)
			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(args[1]), args[0])))
	case "SET":
		if len(args) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleSet(conn, db, args)
	case "GET":
		if len(args) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleGet(conn, db, args)
	case "RPUSH":
		if len(args) < 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleRPush(conn, db, args)
	case "LRANGE":
		if len(args) < 3 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleLRange(conn, db, args)
	case "LPUSH":
		if len(args) < 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleLPush(conn, db, args)
	case "LLEN":
		if len(args) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleLLEN(conn, db, args)
	case "LPOP":
		if len(args) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleLPop(conn, db, args)
	case "BLPOP":
		if len(args) < 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleBLPOP(conn, db, args)
	case "XADD":
		if len(args) < 4 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleXAdd(conn, db, args)
	case "XRANGE":
		if len(args) != 3 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleXRange(conn, db, args)

	case "TYPE":
		if len(args) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)

			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleType(conn, db, args)

	case "INCR":
		if len(args) < 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}

		if weAreInsideMulti {
			multiCommands = append(multiCommands, args)
			conn.Write([]byte("+QUEUED\r\n"))
			return
		}
		handleIncr(conn, db, args)

	case "MULTI":
		if len(args) != 0 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		weAreInsideMulti = true
		conn.Write([]byte("+OK\r\n"))
	case "EXEC":
		if !weAreInsideMulti || len(args) != 0 {
			conn.Write([]byte("-ERR EXEC without MULTI\r\n"))
		}
		if weAreInsideMulti && len(multiCommands) == 0 {
			weAreInsideMulti = false
			conn.Write([]byte("*0\r\n"))
		}
		for _, cmd := range multiCommands {
			strings.ToUpper(cmd[0])
			handleCommand(cmd[0], cmd[1:], db, conn)
		}
	}
}
