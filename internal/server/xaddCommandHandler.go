package server

import (
	"fmt"
	"net"

	"github.com/GARMA-A/redisgo/internal/store"
)

func handleXAdd(conn net.Conn, db *store.DB, parts []string) {
	id, err := db.XAdd(parts[1], parts[2], parts[3:])
	if err != nil {
		switch err {
		case store.ErrXAddIDTooSmall:
			conn.Write([]byte("-ERR The ID specified in XADD is equal or smaller than the target stream top item\r\n"))
		case store.ErrXAddIDZero:
			conn.Write([]byte("-ERR The ID specified in XADD must be greater than 0-0\r\n"))
		default:
			conn.Write([]byte("-ERR Invalid stream ID specified as stream command argument\r\n"))
		}
		return
	}
	conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(id), id)))
}
