package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/GARMA-A/redisgo/internal/server"
	"github.com/GARMA-A/redisgo/internal/store"
)

type replicaConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *replicaConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}
func (c *replicaConn) Write(p []byte) (int, error) {
	return len(p), nil
}

func readFullResyncAndRDB(reader *bufio.Reader) error {
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, "+FULLRESYNC") {
		return fmt.Errorf("unexpected PSYNC response: %q", line)
	}
	lenLine, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	lenLine = strings.TrimSpace(lenLine)
	if len(lenLine) == 0 || lenLine[0] != '$' {
		return fmt.Errorf("invalid RDB length line: %q", lenLine)
	}
	rdbLen, err := strconv.Atoi(lenLine[1:])
	if err != nil || rdbLen < 0 {
		return fmt.Errorf("invalid RDB length: %q", lenLine)
	}
	if rdbLen > 0 {
		buf := make([]byte, rdbLen)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return err
		}
	}
	return nil
}

func runReplicationHandshake(replicaof string, listeningPort int, db *store.DB, replicationId string, offset int64) {
	hostPortArr := strings.Fields(replicaof)
	if len(hostPortArr) != 2 {
		fmt.Fprintf(os.Stderr, "Invalid --replicaof value %q: must be in the format host port\n", replicaof)
		os.Exit(1)
	}
	masterPort, err := strconv.Atoi(hostPortArr[1])
	if err != nil || masterPort < 1 || masterPort > 65535 {
		fmt.Fprintf(os.Stderr, "Invalid port in --replicaof value %q: must be an integer between 1 and 65535\n", replicaof)
		os.Exit(1)
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(hostPortArr[0], hostPortArr[1]))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to master at %s: %v\n", replicaof, err)
		os.Exit(1)
	}
	reader := bufio.NewReader(conn)

	if err := sendPing(conn); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send PING to master at %s: %v\n", replicaof, err)
		os.Exit(1)
	}

	if err := waitForSimpleString(reader, "PONG"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read PING response from master at %s: %v\n", replicaof, err)
		os.Exit(1)
	}

	if err := sendReplconfListeningPort(conn, listeningPort); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send REPLCONF listening-port to master at %s: %v\n", replicaof, err)
		os.Exit(1)
	}

	if err := waitForSimpleString(reader, "OK"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read REPLCONF listening-port response from master at %s: %v\n", replicaof, err)
		os.Exit(1)
	}

	if err := sendReplconfCapaPsync2(conn); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send REPLCONF capa psync2 to master at %s: %v\n", replicaof, err)
		os.Exit(1)
	}

	if err := waitForSimpleString(reader, "OK"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read REPLCONF capa psync2 response from master at %s: %v\n", replicaof, err)
		os.Exit(1)
	}

	if err := sendPsync(conn); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send PSYNC to master at %s: %v\n", replicaof, err)
		os.Exit(1)
	}
	if err := readFullResyncAndRDB(reader); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read FULLRESYNC/RDB from master: %v\n", err)
		os.Exit(1)
	}
	replicaStream := &replicaConn{Conn: conn, reader: reader}
	go server.HandleClient(replicaStream, db, replicaof, replicationId, offset)
}

func sendPing(conn net.Conn) error {
	_, err := conn.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	return err
}

func waitForSimpleString(reader *bufio.Reader, expected string) error {
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if line != fmt.Sprintf("+%s\r\n", expected) {
		return fmt.Errorf("unexpected response: %q", line)
	}
	return nil
}

func sendReplconfListeningPort(conn net.Conn, listeningPort int) error {
	payload := fmt.Sprintf("*3\r\n$8\r\nREPLCONF\r\n$14\r\nlistening-port\r\n$%d\r\n%d\r\n", len(strconv.Itoa(listeningPort)), listeningPort)
	_, err := conn.Write([]byte(payload))
	return err
}

func sendReplconfCapaPsync2(conn net.Conn) error {
	_, err := conn.Write([]byte("*3\r\n$8\r\nREPLCONF\r\n$4\r\ncapa\r\n$6\r\npsync2\r\n"))
	return err
}

func sendPsync(conn net.Conn) error {
	_, err := conn.Write([]byte("*3\r\n$5\r\nPSYNC\r\n$1\r\n?\r\n$2\r\n-1\r\n"))
	return err
}
