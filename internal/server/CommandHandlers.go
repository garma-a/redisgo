package server

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

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

func handleType(conn net.Conn, db *store.DB, parts []string) {
	key := parts[1]
	myType := db.GetType(key)
	conn.Write([]byte(fmt.Sprintf("+%s\r\n", myType)))
}

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

func handleXRange(conn net.Conn, db *store.DB, parts []string) {
	entries, err := db.XRange(parts[1], parts[2], parts[3])
	if err != nil {
		conn.Write([]byte("-ERR Invalid stream ID specified as stream command argument\r\n"))
		return
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("*%d\r\n", len(entries)))
	for _, entry := range entries {
		builder.WriteString("*2\r\n")
		builder.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(entry.ID), entry.ID))
		builder.WriteString(fmt.Sprintf("*%d\r\n", len(entry.Fields)))
		for _, item := range entry.Fields {
			builder.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(item), item))
		}
	}

	conn.Write([]byte(builder.String()))
}

func handleIncr(conn net.Conn, db *store.DB, parts []string) {
	newVal, err := db.Incr(parts[1])
	if err != nil {
		conn.Write([]byte("-ERR value is not an integer or out of range\r\n"))
		return
	}
	conn.Write([]byte(fmt.Sprintf(":%d\r\n", newVal)))

}
