package server

import (
	"fmt"
	"net"

	"github.com/GARMA-A/redisgo/internal/store"
)

func handleXAdd(conn net.Conn, db *store.DB, parts []string) {
	id := db.XAdd(parts[1], parts[2], parts[3:])
	conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(id), id)))
}
