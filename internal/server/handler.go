package server

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/GARMA-A/redisgo/internal/resp"
	"github.com/GARMA-A/redisgo/internal/store"
)

var (
	replicationsConn  map[net.Conn]struct{} = make(map[net.Conn]struct{})
	replicationConnMu sync.RWMutex          = sync.RWMutex{}
)

func addReplicationsConn(conn net.Conn) {
	replicationConnMu.Lock()
	replicationsConn[conn] = struct{}{}
	replicationConnMu.Unlock()
}
func getReplicationsConn() []net.Conn {
	replicationConnMu.RLock()
	defer replicationConnMu.RUnlock()
	conns := make([]net.Conn, 0, len(replicationsConn))
	for conn := range replicationsConn {
		conns = append(conns, conn)
	}
	return conns
}

func encodeRespArray(command string, args []string) []byte {
	var b strings.Builder
	total := 1 + len(args)
	fmt.Fprintf(&b, "*%d\r\n", total)
	fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(command), command)
	for _, arg := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(arg), arg)
	}
	return []byte(b.String())
}
func propagateIfReplicas(command string, args []string) {
	payload := encodeRespArray(command, args)
	for _, conn := range getReplicationsConn() {
		_, err := conn.Write(payload)
		if err != nil {
			fmt.Printf("Error propagating to replica: %v\n", err)
			conn.Close()
			replicationConnMu.Lock()
			delete(replicationsConn, conn)
			replicationConnMu.Unlock()
		}
	}

}

func HandleClient(conn net.Conn, db *store.DB, replicaof string, replicationId string, offset int64) {
	defer conn.Close()

	inMulti := false
	queuedCommands := make([][]string, 0)
	pending := make([]byte, 0, 4096)
	buf := make([]byte, 1024)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				fmt.Println("Read error:", err)
			}
			break
		}
		pending = append(pending, buf[:n]...)
		for {
			parts, consumed, err := resp.ParseNext(pending)
			if err != nil {
				if errors.Is(err, resp.ErrIncomplete) {
					break
				}
				pending = pending[:0]
				break
			}
			pending = pending[consumed:]
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

				offset += int64(consumed)
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

				offset += int64(consumed)

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
					executeCommand(strings.ToUpper(queued[0]), queued[1:], db, capture, replicaof, replicationId, offset, consumed)
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
				offset += int64(consumed)
				inMulti = false
				queuedCommands = queuedCommands[:0]
				conn.Write([]byte("+OK\r\n"))

			default:
				if inMulti {
					queuedCopy := append([]string(nil), parts...)
					queuedCommands = append(queuedCommands, queuedCopy)
					offset += int64(consumed)
					conn.Write([]byte("+QUEUED\r\n"))
					continue
				}
				offset += int64(consumed)
				executeCommand(command, args, db, conn, replicaof, replicationId, offset, consumed)
			}
		}
	}
}

func executeCommand(command string, args []string, db *store.DB, conn net.Conn, replicaof string, replicationID string, offset int64, consumed int) {
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
		propagateIfReplicas("SET", args)

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
		propagateIfReplicas("RPUSH", args)

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
		propagateIfReplicas("LPUSH", args)

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
		propagateIfReplicas("LPOP", args)

	case "BLPOP":
		if len(args) < 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleBLPOP(conn, db, args)
		propagateIfReplicas("BLPOP", args)

	case "XADD":
		if len(args) < 4 || (len(args)-2)%2 != 0 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		handleXAdd(conn, db, args)
		propagateIfReplicas("XADD", args)

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
		propagateIfReplicas("INCR", args)

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
		var isSlave bool = replicaof != ""
		handleInfo(conn, db, haveReplicationInfo, isSlave, replicationID, offset)
	case "REPLCONF":
		if replicaof != "" && len(args) == 2 &&
			strings.ToUpper(args[0]) == "GETACK" && args[1] == "*" {
			offsetStr := fmt.Sprintf("%d", offset-int64(consumed))
			payload := fmt.Appendf(nil, "*3\r\n$8\r\nREPLCONF\r\n$3\r\nACK\r\n$%d\r\n%s\r\n", len(offsetStr), offsetStr)
			if dw, ok := conn.(directWriter); ok {
				dw.WriteDirect(payload)
			} else {
				conn.Write(payload)
			}
			return
		}
		if len(args) != 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		conn.Write([]byte("+OK\r\n"))
	case "PSYNC":
		if len(args) != 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		rdbHex := "524544495330303131fa0972656469732d76657205372e322e30fa0a72656469732d62697473c040fa056374696d65c26d08bc65fa08757365642d6d656dc2b0c41000fa08616f662d62617365c000fff06e3bfec0ff5aa2"
		rdbBytes, err := hex.DecodeString(rdbHex)
		if err != nil {
			conn.Write([]byte("-ERR failed to decode RDB data\r\n"))
			return
		}
		conn.Write([]byte(fmt.Sprintf("+FULLRESYNC %s 0\r\n", replicationID)))
		conn.Write([]byte(fmt.Sprintf("$%d\r\n", len(rdbBytes))))
		conn.Write(rdbBytes)
		addReplicationsConn(conn)
	case "WAIT":
		if len(args) != 2 {
			conn.Write([]byte("-ERR wrong number of arguments\r\n"))
			return
		}
		conn.Write([]byte(":0\r\n"))

	default:
		conn.Write([]byte("-ERR unknown command\r\n"))
	}
}

type bufferConn struct {
	buf bytes.Buffer
}
type directWriter interface {
	WriteDirect([]byte) (int, error)
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
