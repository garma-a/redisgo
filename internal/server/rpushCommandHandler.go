package server

import (
	"fmt"
	"net"

	"github.com/GARMA-A/redisgo/internal/store"
)

func handleRPush(conn net.Conn, db *store.DB, parts []string) {
	if len(parts) > 3 {
		db.RPushMany(parts[1], parts[2:])
	} else {
		db.RPush(parts[1], parts[2])
	}
	conn.Write([]byte(fmt.Sprintf(":%d\r\n", db.LLen())))
}
