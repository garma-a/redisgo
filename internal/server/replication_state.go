package server

import (
	"net"
	"sync"
	"time"
)

type ReplicationState struct {
	mu            sync.Mutex
	cond          *sync.Cond
	replicas      map[net.Conn]struct{}
	acks          map[net.Conn]int64
	streamOffset  int64
	writesPending bool
}

func NewReplicationState() *ReplicationState {
	rs := &ReplicationState{
		replicas: make(map[net.Conn]struct{}),
		acks:     make(map[net.Conn]int64),
	}
	rs.cond = sync.NewCond(&rs.mu)
	return rs
}

func (rs *ReplicationState) AddReplica(conn net.Conn) {
	rs.mu.Lock()
	rs.replicas[conn] = struct{}{}
	if _, ok := rs.acks[conn]; !ok {
		rs.acks[conn] = 0
	}
	rs.cond.Broadcast()
	rs.mu.Unlock()
}

func (rs *ReplicationState) RemoveReplica(conn net.Conn) {
	rs.mu.Lock()
	delete(rs.replicas, conn)
	delete(rs.acks, conn)
	rs.cond.Broadcast()
	rs.mu.Unlock()
}

func (rs *ReplicationState) ReplicaCount() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return len(rs.replicas)
}

func (rs *ReplicationState) ReplicaConns() []net.Conn {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	conns := make([]net.Conn, 0, len(rs.replicas))
	for conn := range rs.replicas {
		conns = append(conns, conn)
	}
	return conns
}

func (rs *ReplicationState) NoteWrite(n int) {
	rs.mu.Lock()
	rs.streamOffset += int64(n)
	rs.writesPending = true
	rs.mu.Unlock()
}

func (rs *ReplicationState) NoteReplicationCommand(n int) {
	rs.mu.Lock()
	rs.streamOffset += int64(n)
	rs.mu.Unlock()
}

func (rs *ReplicationState) StreamOffset() int64 {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.streamOffset
}

func (rs *ReplicationState) WritesPending() bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.writesPending
}

func (rs *ReplicationState) ClearWritesPending() {
	rs.mu.Lock()
	rs.writesPending = false
	rs.mu.Unlock()
}

func (rs *ReplicationState) ConsumeWritesPending() bool {
	rs.mu.Lock()
	pending := rs.writesPending
	rs.writesPending = false
	rs.mu.Unlock()
	return pending
}

func (rs *ReplicationState) RecordAck(conn net.Conn, offset int64) {
	rs.mu.Lock()
	if _, ok := rs.replicas[conn]; ok {
		rs.acks[conn] = offset
	}
	rs.cond.Broadcast()
	rs.mu.Unlock()
}

func (rs *ReplicationState) AckCountAtLeast(target int64) int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	count := 0
	for conn := range rs.replicas {
		if rs.acks[conn] >= target {
			count++
		}
	}
	return count
}

func (rs *ReplicationState) WaitForAcks(target int64, numReplicas int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for {
		count := 0
		for conn := range rs.replicas {
			if rs.acks[conn] >= target {
				count++
			}
		}
		if count >= numReplicas {
			return count
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return count
		}
		timer := time.AfterFunc(remaining, func() {
			rs.cond.Broadcast()
		})
		rs.cond.Wait()
		timer.Stop()
	}
}
