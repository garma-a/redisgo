package store

import (
	"sync"
	"time"
)

type Value struct {
	Data      string
	ExpiresAt time.Time
}

type DB struct {
	mu    sync.RWMutex
	data  map[string]Value
	lists []string
}

func New() *DB {
	return &DB{
		data:  make(map[string]Value),
		lists: make([]string, 0),
	}
}

func (db *DB) Set(key, value string, expiry time.Time) {
	db.mu.Lock()
	db.data[key] = Value{
		Data:      value,
		ExpiresAt: expiry,
	}
	db.mu.Unlock()
}

func (db *DB) Get(key string) (string, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	val, exists := db.data[key]
	return val.Data, exists
}

func (db *DB) GetWithTTL(key string) (string, bool) {
	db.mu.RLock()
	val, exists := db.data[key]
	db.mu.RUnlock()

	if exists && !val.ExpiresAt.IsZero() && time.Now().After(val.ExpiresAt) {
		db.mu.Lock()
		delete(db.data, key)
		db.mu.Unlock()
		exists = false
	}

	if exists {
		return val.Data, true
	}
	return "", false
}

func (db *DB) RPush(key string, values string) {
	db.mu.Lock()
	db.lists = append(db.lists, values)
	db.mu.Unlock()
}

func (db *DB) RPushMany(key string, values []string) []string {
	db.mu.Lock()
	db.lists = append(db.lists, values...)
	db.mu.Unlock()
	return db.lists
}
func (db *DB) LRange(start, stop int) []string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if start < 0 {
		start = len(db.lists) + start
	}
	if start < 0 {
		start = 0
	}
	if stop < 0 {
		stop = len(db.lists) + stop
	}
	if stop < 0 {
		stop = 0
	}
	if stop >= len(db.lists) {
		stop = len(db.lists) - 1
	}
	if start > stop {
		return []string{}
	}
	return db.lists[start : stop+1]
}

func (db *DB) LLen() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.lists)
}
