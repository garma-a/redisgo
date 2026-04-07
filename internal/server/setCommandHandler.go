package server

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/GARMA-A/redisgo/internal/store"
)

func handleSet(conn net.Conn, db *store.DB, parts []string) {
	key := parts[1]
	val := parts[2]
	var expiry time.Time

	if len(parts) >= 5 {
		option := strings.ToUpper(parts[3])
		expiryValue, _ := strconv.Atoi(parts[4])

		if option == "EX" {
			expiry = time.Now().Add(time.Duration(expiryValue) * time.Second)
		} else if option == "PX" {
			expiry = time.Now().Add(time.Duration(expiryValue) * time.Millisecond)
		}
	}
	db.Set(key, val, expiry)
	conn.Write([]byte("+OK\r\n"))
}
