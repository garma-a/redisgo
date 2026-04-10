package server

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/GARMA-A/redisgo/internal/store"
)

func handleRPush(conn net.Conn, db *store.DB, parts []string) {
	var length int
	if len(parts) > 3 {
		length = db.RPushMany(parts[1], parts[2:])
	} else {
		length = db.RPush(parts[1], parts[2])
	}
	conn.Write([]byte(fmt.Sprintf(":%d\r\n", length)))
}

func handleLRange(conn net.Conn, db *store.DB, parts []string) {
	start, _ := strconv.ParseInt(parts[2], 10, 64)
	stop, _ := strconv.ParseInt(parts[3], 10, 64)
	list := db.LRange(parts[1], int(start), int(stop))
	conn.Write([]byte(fmt.Sprintf("*%d\r\n", len(list))))
	for _, item := range list {
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(item), item)))
	}
}
func handleLPush(conn net.Conn, db *store.DB, parts []string) {
	var length int
	if len(parts) > 3 {
		length = db.LPushMany(parts[1], parts[2:])
	} else {
		length = db.LPush(parts[1], parts[2])
	}
	conn.Write([]byte(fmt.Sprintf(":%d\r\n", length)))
}

func handleLLEN(conn net.Conn, db *store.DB, parts []string) {
	conn.Write([]byte(fmt.Sprintf(":%d\r\n", db.LLEN(parts[1]))))
}

func handleLPop(conn net.Conn, db *store.DB, parts []string) {
	if len(parts) > 2 {
		handleLPopMany(conn, db, parts)
	} else {
		val := db.LPop(parts[1])
		if val != "" {
			conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(val), val)))
		} else {
			conn.Write([]byte("$-1\r\n"))
		}
	}

}
func handleLPopMany(conn net.Conn, db *store.DB, parts []string) {
	count, _ := strconv.Atoi(parts[2])
	values := db.LPopMany(parts[1], count)
	conn.Write([]byte(fmt.Sprintf("*%d\r\n", len(values))))
	for _, val := range values {
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(val), val)))
	}
}

func handleBLPOP(conn net.Conn, db *store.DB, parts []string) {
	key := parts[1]
	timeoutSec, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		conn.Write([]byte("-ERR invalid timeout\r\n"))
		return
	}

	ch := make(chan string)
	if val, ok := db.BLPOPWithOk(key, ch); ok {
		conn.Write([]byte(fmt.Sprintf("*2\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(val), val)))
		return
	}

	if timeoutSec > 0 {
		timeoutSec += 0.50
		select {
		case val := <-ch:
			conn.Write([]byte(fmt.Sprintf("*2\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(val), val)))
		case <-time.After(time.Duration(timeoutSec * float64(time.Second))):
			db.RemoveWaiter(key, ch)
			conn.Write([]byte("*-1\r\n"))
		}
	} else {
		// timeout 0 means block forever
		val := <-ch
		conn.Write([]byte(fmt.Sprintf("*2\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(val), val)))
	}
}
