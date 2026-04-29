package server

import (
	"github.com/GARMA-A/redisgo/internal/store"
	"net"
	"testing"
)

func Test_executeCommand(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		command       string
		args          []string
		db            *store.DB
		conn          net.Conn
		replicaof     string
		replicationID string
		offset        int64
		consumed      int
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executeCommand(tt.command, tt.args, tt.db, tt.conn, tt.replicaof, tt.replicationID, tt.offset, tt.consumed)
		})
	}
}
