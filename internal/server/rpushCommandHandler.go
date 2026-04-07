package server

import (
	"fmt"
	"net"
	"strconv"

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

func handleLRange(conn net.Conn, db *store.DB, parts []string) {
	start, _ := strconv.ParseInt(parts[2], 10, 64)
	stop, _ := strconv.ParseInt(parts[3], 10, 64)
	list := db.LRange(int(start), int(stop))
	conn.Write([]byte(fmt.Sprintf("*%d\r\n", len(list))))
	for _, item := range list {
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(item), item)))
	}
}
