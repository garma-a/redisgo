package server

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/GARMA-A/redisgo/internal/resp"
	"github.com/GARMA-A/redisgo/internal/store"
)

func HandleClient(conn net.Conn, db *store.DB, isSlave bool) {
	defer conn.Close()

	inMulti := false
	queuedCommands := make([][]string, 0)

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
		args := parts[1:]

		switch command {
		case "MULTI":
			if len(args) != 0 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}
			inMulti = true
			queuedCommands = queuedCommands[:0]
			conn.Write([]byte("+OK\r\n"))

		case "EXEC":
			if len(args) != 0 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}
			if !inMulti {
				conn.Write([]byte("-ERR EXEC without MULTI\r\n"))
				continue
			}

			if len(queuedCommands) == 0 {
				inMulti = false
				queuedCommands = queuedCommands[:0]
				conn.Write([]byte("*0\r\n"))
				continue
			}

			responses := make([][]byte, 0, len(queuedCommands))
			for _, queued := range queuedCommands {
				if len(queued) == 0 {
					responses = append(responses, []byte("-ERR unknown command\r\n"))
					continue
				}
				capture := &bufferConn{}
				executeCommand(strings.ToUpper(queued[0]), queued[1:], db, capture)
				responses = append(responses, capture.Bytes())
			}

			inMulti = false
			queuedCommands = queuedCommands[:0]

			var out strings.Builder
			out.WriteString(fmt.Sprintf("*%d\r\n", len(responses)))
			for _, response := range responses {
				out.Write(response)
			}
			conn.Write([]byte(out.String()))
		case "DISCARD":
			if len(args) != 0 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				continue
			}
			if !inMulti {
				conn.Write([]byte("-ERR DISCARD without MULTI\r\n"))
				continue
			}
			inMulti = false
			queuedCommands = queuedCommands[:0]
			conn.Write([]byte("+OK\r\n"))
		case "INFO":
			if len(args) > 1 {
				conn.Write([]byte("-ERR wrong number of arguments\r\n"))
				return
			}
			if len(args) == 1 && strings.ToLower(args[0]) != "replication" {
				conn.Write([]byte("-ERR invalid argument\r\n"))
				return
			}
			var haveReplicationInfo bool = false
			if len(args) == 1 {
				haveReplicationInfo = true
			}
			handleInfo(conn, db, haveReplicationInfo, isSlave)

		default:
			if inMulti {
				queuedCopy := append([]string(nil), parts...)
				queuedCommands = append(queuedCommands, queuedCopy)
				conn.Write([]byte("+QUEUED\r\n"))
				continue
			}

			executeCommand(command, args, db, conn)
		}
	}
}

func executeCommand(command string, args []string, db *store.DB, conn net.Conn) {
	switch command {
	case "PING":
		if len(args) != 0 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		conn.Write([]byte("+PONG\r\n"))

	case "ECHO":
		if len(args) != 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleEcho(conn, args)

	case "SET":
		if len(args) < 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleSet(conn, db, args)

	case "GET":
		if len(args) != 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleGet(conn, db, args)

	case "RPUSH":
		if len(args) < 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleRPush(conn, db, args)

	case "LRANGE":
		if len(args) != 3 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleLRange(conn, db, args)

	case "LPUSH":
		if len(args) < 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleLPush(conn, db, args)

	case "LLEN":
		if len(args) != 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleLLEN(conn, db, args)

	case "LPOP":
		if len(args) < 1 || len(args) > 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleLPop(conn, db, args)

	case "BLPOP":
		if len(args) < 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleBLPOP(conn, db, args)

	case "XADD":
		if len(args) < 4 || (len(args)-2)%2 != 0 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleXAdd(conn, db, args)

	case "XRANGE":
		if len(args) != 3 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleXRange(conn, db, args)

	case "TYPE":
		if len(args) != 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleType(conn, db, args)

	case "INCR":
		if len(args) != 1 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleIncr(conn, db, args)

	default:
		conn.Write([]byte("-ERR unknown command\r\n"))
	}
}

type bufferConn struct {
	buf bytes.Buffer
}

func (c *bufferConn) Read(_ []byte) (int, error)         { return 0, io.EOF }
func (c *bufferConn) Write(p []byte) (int, error)        { return c.buf.Write(p) }
func (c *bufferConn) Close() error                       { return nil }
func (c *bufferConn) LocalAddr() net.Addr                { return nil }
func (c *bufferConn) RemoteAddr() net.Addr               { return nil }
func (c *bufferConn) SetDeadline(_ time.Time) error      { return nil }
func (c *bufferConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *bufferConn) SetWriteDeadline(_ time.Time) error { return nil }
func (c *bufferConn) Bytes() []byte                      { return c.buf.Bytes() }
