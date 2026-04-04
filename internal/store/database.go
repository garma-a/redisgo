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
	mu   sync.RWMutex
	data map[string]Value
}

func New() *DB {
	return &DB{
		data: make(map[string]Value),
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
