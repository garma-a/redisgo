package server

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/GARMA-A/redisgo/internal/store"
)

func handleEcho(conn net.Conn, args []string) {
	conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(args[0]), args[0])))
}

func handleGet(conn net.Conn, db *store.DB, args []string) {
	val, exists := db.GetWithTTL(args[0])
	if exists {
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(val), val)))
	} else {
		conn.Write([]byte("$-1\r\n"))
	}
}

func handleRPush(conn net.Conn, db *store.DB, args []string) {
	var length int
	if len(args) > 2 {
		length = db.RPushMany(args[0], args[1:])
	} else {
		length = db.RPush(args[0], args[1])
	}
	conn.Write([]byte(fmt.Sprintf(":%d\r\n", length)))
}

func handleLRange(conn net.Conn, db *store.DB, args []string) {
	start, _ := strconv.ParseInt(args[1], 10, 64)
	stop, _ := strconv.ParseInt(args[2], 10, 64)
	list := db.LRange(args[0], int(start), int(stop))
	conn.Write([]byte(fmt.Sprintf("*%d\r\n", len(list))))
	for _, item := range list {
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(item), item)))
	}
}

func handleLPush(conn net.Conn, db *store.DB, args []string) {
	var length int
	if len(args) > 2 {
		length = db.LPushMany(args[0], args[1:])
	} else {
		length = db.LPush(args[0], args[1])
	}
	conn.Write([]byte(fmt.Sprintf(":%d\r\n", length)))
}

func handleLLEN(conn net.Conn, db *store.DB, args []string) {
	conn.Write([]byte(fmt.Sprintf(":%d\r\n", db.LLEN(args[0]))))
}

func handleLPop(conn net.Conn, db *store.DB, args []string) {
	if len(args) > 1 {
		handleLPopMany(conn, db, args)
		return
	}

	val := db.LPop(args[0])
	if val != "" {
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(val), val)))
	} else {
		conn.Write([]byte("$-1\r\n"))
	}
}

func handleLPopMany(conn net.Conn, db *store.DB, args []string) {
	count, _ := strconv.Atoi(args[1])
	values := db.LPopMany(args[0], count)
	conn.Write([]byte(fmt.Sprintf("*%d\r\n", len(values))))
	for _, val := range values {
		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len(val), val)))
	}
}

func handleBLPOP(conn net.Conn, db *store.DB, args []string) {
	key := args[0]
	timeoutSec, err := strconv.ParseFloat(args[1], 64)
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
		val := <-ch
		conn.Write([]byte(fmt.Sprintf("*2\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n", len(key), key, len(val), val)))
	}
}

func handleType(conn net.Conn, db *store.DB, args []string) {
	myType := db.GetType(args[0])
	conn.Write([]byte(fmt.Sprintf("+%s\r\n", myType)))
}

func handleSet(conn net.Conn, db *store.DB, args []string) {
	key := args[0]
	val := args[1]
	var expiry time.Time

	if len(args) >= 4 {
		option := strings.ToUpper(args[2])
		expiryValue, _ := strconv.Atoi(args[3])

		if option == "EX" {
			expiry = time.Now().Add(time.Duration(expiryValue) * time.Second)
		} else if option == "PX" {
			expiry = time.Now().Add(time.Duration(expiryValue) * time.Millisecond)
		}
	}
	db.Set(key, val, expiry)
	conn.Write([]byte("+OK\r\n"))
}

func handleXAdd(conn net.Conn, db *store.DB, args []string) {
	id, err := db.XAdd(args[0], args[1], args[2:])
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

func handleXRange(conn net.Conn, db *store.DB, args []string) {
	entries, err := db.XRange(args[0], args[1], args[2])
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

func handleIncr(conn net.Conn, db *store.DB, args []string) {
	newVal, err := db.Incr(args[0])
	if err != nil {
		conn.Write([]byte("-ERR value is not an integer or out of range\r\n"))
		return
	}
	conn.Write([]byte(fmt.Sprintf(":%d\r\n", newVal)))
}

func handleInfo(conn net.Conn, db *store.DB, haveReplicationArg bool, isSlave bool) {
	if haveReplicationArg && isSlave {

		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len("role:slave"), "role:slave")))

	} else if haveReplicationArg && !isSlave {

		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len("role:master"), "role:master")))

	} else {

		conn.Write([]byte(fmt.Sprintf("$%d\r\n%s\r\n", len("redis_version:0.1.0"), "redis_version:0.1.0")))

	}

}
