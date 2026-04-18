package server

import (
	"fmt"
	"net"
	"strings"

	"github.com/GARMA-A/redisgo/internal/store"
)

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
