package server

import (
	"fmt"
	"net"

	"github.com/GARMA-A/redisgo/internal/store"
)

func handleGet(conn net.Conn, db *store.DB, parts []string) {
	val, exists := db.GetWithTTL(parts[1])
	if exists {
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(val), val)))
	} else {
		conn.Write([]byte("$-1\r\n"))
	}
}
