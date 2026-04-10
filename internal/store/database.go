package store

import (
	"sync"
	"time"
)

type value struct {
	Data      string
	ExpiresAt time.Time
}

type list struct {
	values  []string
	waiters []chan string
}

type DB struct {
	mu    sync.RWMutex
	data  map[string]value
	lists map[string]*list
}

func newList() *list {
	return &list{
		values: []string{},
	}
}

func (db *DB) getOrCreateList(key string) *list {
	if lst, exists := db.lists[key]; exists && lst != nil {
		return lst
	}
	newLst := newList()
	db.lists[key] = newLst
	return newLst
}

func (lst *list) isSendToWaiter(value string) bool {
	if len(lst.waiters) == 0 {
		return false
	}
	ch := lst.waiters[0]
	lst.waiters[0] = nil
	lst.waiters = lst.waiters[1:]
	ch <- value
	return true
}

func New() *DB {
	return &DB{
		data:  make(map[string]value),
		lists: make(map[string]*list),
	}
}

func (db *DB) Set(key, val string, expiry time.Time) {
	db.mu.Lock()
	db.data[key] = value{
		Data:      val,
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
	lst := db.getOrCreateList(key)
	currentLen := len(lst.values)
	if lst.isSendToWaiter(value) {
		return currentLen + 1
	}
	lst.values = append(lst.values, value)
	return len(lst.values)
}

func (db *DB) RPushMany(key string, values []string) int {
	db.mu.Lock()
	defer db.mu.Unlock()
	lst := db.getOrCreateList(key)
	currentLen := len(lst.values)
	for _, value := range values {
		if lst.isSendToWaiter(value) {
			continue
		}
		lst.values = append(lst.values, value)
	}
	return currentLen + len(values)
}

func (db *DB) LRange(key string, start, stop int) []string {
	db.mu.RLock()
	lst := db.lists[key]
	db.mu.RUnlock()
	if lst == nil || len(lst.values) == 0 {
		return []string{}
	}

	list := lst.values

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
	lst := db.getOrCreateList(key)
	currentLen := len(lst.values)
	if lst.isSendToWaiter(value) {
		return currentLen + 1
	}
	lst.values = append([]string{value}, lst.values...)
	return len(lst.values)
}

func (db *DB) LPushMany(key string, values []string) int {
	db.mu.Lock()
	defer db.mu.Unlock()
	lst := db.getOrCreateList(key)
	currentLen := len(lst.values)
	for _, value := range values {
		if !lst.isSendToWaiter(value) {
			lst.values = append([]string{value}, lst.values...)
		}
	}
	return currentLen + len(values)
}

func (db *DB) LPop(key string) string {
	db.mu.Lock()
	defer db.mu.Unlock()
	val := ""
	if lst, exists := db.lists[key]; exists && lst != nil && len(lst.values) > 0 {
		val = lst.values[0]
		lst.values = lst.values[1:]
	}
	return val
}

func (db *DB) LPopWithOK(key string) (string, bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if lst, exists := db.lists[key]; exists && lst != nil && len(lst.values) > 0 {
		val := lst.values[0]
		lst.values = lst.values[1:]
		return val, true
	}
	return "", false
}

func (db *DB) BLPOPWithOk(key string, ch chan string) (string, bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	lst := db.getOrCreateList(key)
	if len(lst.values) == 0 {
		lst.waiters = append(lst.waiters, ch)
		return "", false
	}
	val := lst.values[0]
	lst.values = lst.values[1:]
	return val, true
}

func (db *DB) RemoveWaiter(key string, ch chan string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	lst, exists := db.lists[key]
	if !exists || lst == nil {
		return
	}
	for i, waiter := range lst.waiters {
		if waiter == ch {
			lst.waiters = append(lst.waiters[:i], lst.waiters[i+1:]...)
			break
		}
	}
}

func (db *DB) LLEN(key string) int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if lst, exists := db.lists[key]; exists && lst != nil {
		return len(lst.values)
	} else {
		return 0
	}
}
func (db *DB) LPopMany(key string, count int) []string {
	db.mu.Lock()
	defer db.mu.Unlock()
	if lst, exists := db.lists[key]; exists && lst != nil && len(lst.values) > 0 {
		if count > len(lst.values) {
			count = len(lst.values)
		}
		vals := lst.values[:count]
		lst.values = lst.values[count:]
		return vals
	}
	return []string{}
}

func (db *DB) LPopWaiter(key string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	lst := db.getOrCreateList(key)
	lst.waiters = lst.waiters[1:]
}

func (db *DB) GetType(key string) string {
	if _, ok := db.lists[key]; ok {
		return "list"
	} else if _, ok := db.data[key]; ok {
		return "string"
	} else {
		return "none"
	}

}
