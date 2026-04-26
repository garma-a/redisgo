package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

func runReplicationHandshake(replicaof string, listeningPort int) {
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
	defer conn.Close()

	if err := sendPing(conn); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send PING to master at %s: %v\n", replicaof, err)
		os.Exit(1)
	}

	if err := waitForPong(conn); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read PING response from master at %s: %v\n", replicaof, err)
		os.Exit(1)
	}

	if err := sendReplconfListeningPort(conn, listeningPort); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send REPLCONF listening-port to master at %s: %v\n", replicaof, err)
		os.Exit(1)
	}

	if err := sendReplconfCapaPsync2(conn); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to send REPLCONF capa psync2 to master at %s: %v\n", replicaof, err)
		os.Exit(1)
	}
}

func sendPing(conn net.Conn) error {
	_, err := conn.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	return err
}

func waitForPong(conn net.Conn) error {
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if line != "+PONG\r\n" {
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
