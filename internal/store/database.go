package store

import (
	"slices"
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
	lists map[string][]string
}

func New() *DB {
	return &DB{
		data:  make(map[string]Value),
		lists: make(map[string][]string),
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

func (db *DB) RPush(key string, value string) int {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.lists[key] = append(db.lists[key], value)
	return len(db.lists[key])
}

func (db *DB) RPushMany(key string, values []string) int {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.lists[key] = append(db.lists[key], values...)
	return len(db.lists[key])
}

func (db *DB) LRange(key string, start, stop int) []string {
	db.mu.RLock()
	list, exists := db.lists[key]
	db.mu.RUnlock()
	if !exists {
		return []string{}
	}

	if start < 0 {
		start = len(list) + start
	}
	if start < 0 {
		start = 0
	}
	if stop < 0 {
		stop = len(list) + stop
	}
	if stop < 0 {
		stop = 0
	}
	if stop >= len(list) {
		stop = len(list) - 1
	}
	if start > stop {
		return []string{}
	}
	return list[start : stop+1]
}
func (db *DB) LPush(key string, value string) int {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.lists[key] = append([]string{value}, db.lists[key]...)
	return len(db.lists[key])
}

func (db *DB) LPushMany(key string, values []string) int {
	slices.Reverse(values)
	db.mu.Lock()
	defer db.mu.Unlock()
	db.lists[key] = append(values, db.lists[key]...)
	return len(db.lists[key])
}

func (db *DB) LPop(key string) string {
	db.mu.Lock()
	defer db.mu.Unlock()
	val := ""
	if lst, exists := db.lists[key]; exists && len(lst) > 0 {
		val = lst[0]
		db.lists[key] = db.lists[key][1:]
	}
	return val
}

func (db *DB) LLEN(key string) int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if lst, exists := db.lists[key]; exists {
		return len(lst)
	} else {
		return 0
	}
}
func (db *DB) LPopMany(key string, count int) []string {
	db.mu.Lock()
	defer db.mu.Unlock()
	if lst, exists := db.lists[key]; exists && len(lst) > 0 {
		if count > len(lst) {
			count = len(lst)
		}
		vals := lst[:count]
		db.lists[key] = db.lists[key][count:]
		return vals
	}
	return []string{}
}
