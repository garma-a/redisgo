package store

import (
	"container/list"
	"sync"
	"time"
)

type Value struct {
	Data      string
	ExpiresAt time.Time
}

type List struct {
	Values []string
	queue  *list.List
}

type DB struct {
	mu    sync.RWMutex
	data  map[string]Value
	lists map[string]*List
}

func newList() *List {
	return &List{
		Values: []string{},
		queue:  list.New(),
	}
}

func (db *DB) getOrCreateList(key string) *List {
	if lst, exists := db.lists[key]; exists && lst != nil {
		if lst.queue == nil {
			lst.queue = list.New()
		}
		return lst
	}
	newLst := newList()
	db.lists[key] = newLst
	return newLst
}

func New() *DB {
	return &DB{
		data:  make(map[string]Value),
		lists: make(map[string]*List),
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
	lst := db.getOrCreateList(key)
	currentLen := len(lst.Values)
	if lst.queue != nil && lst.queue.Len() > 0 {
		waiter := lst.queue.Front()
		lst.queue.Remove(waiter)
		if ch, ok := waiter.Value.(chan string); ok {
			ch <- value
			return currentLen + 1
		}
	}
	lst.Values = append(lst.Values, value)
	return len(lst.Values)
}

func (db *DB) RPushMany(key string, values []string) int {
	db.mu.Lock()
	defer db.mu.Unlock()
	lst := db.getOrCreateList(key)
	currentLen := len(lst.Values)
	for _, value := range values {
		if lst.queue != nil && lst.queue.Len() > 0 {
			waiter := lst.queue.Front()
			lst.queue.Remove(waiter)
			if ch, ok := waiter.Value.(chan string); ok {
				ch <- value
				continue
			}
		}
		lst.Values = append(lst.Values, value)
	}
	return currentLen + len(values)
}

func (db *DB) LRange(key string, start, stop int) []string {
	db.mu.RLock()
	lst := db.lists[key]
	db.mu.RUnlock()
	if lst == nil || len(lst.Values) == 0 {
		return []string{}
	}

	list := lst.Values

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
	currentLen := len(lst.Values)
	if lst.queue != nil && lst.queue.Len() > 0 {
		waiter := lst.queue.Front()
		lst.queue.Remove(waiter)
		if ch, ok := waiter.Value.(chan string); ok {
			ch <- value
			return currentLen + 1
		}
	}
	lst.Values = append([]string{value}, lst.Values...)
	return len(lst.Values)
}

func (db *DB) LPushMany(key string, values []string) int {
	db.mu.Lock()
	defer db.mu.Unlock()
	lst := db.getOrCreateList(key)
	currentLen := len(lst.Values)
	for _, value := range values {
		if lst.queue != nil && lst.queue.Len() > 0 {
			waiter := lst.queue.Front()
			lst.queue.Remove(waiter)
			if ch, ok := waiter.Value.(chan string); ok {
				ch <- value
				continue
			}
		}
		lst.Values = append([]string{value}, lst.Values...)
	}
	return currentLen + len(values)
}

func (db *DB) LPop(key string) string {
	db.mu.Lock()
	defer db.mu.Unlock()
	val := ""
	if lst, exists := db.lists[key]; exists && lst != nil && len(lst.Values) > 0 {
		val = lst.Values[0]
		lst.Values = lst.Values[1:]
	}
	return val
}

func (db *DB) LPopWithOK(key string) (string, bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if lst, exists := db.lists[key]; exists && lst != nil && len(lst.Values) > 0 {
		val := lst.Values[0]
		lst.Values = lst.Values[1:]
		return val, true
	}
	return "", false
}

func (db *DB) RegisterBLPop(key string, ch chan string) (string, bool) {
	db.mu.Lock()
	defer db.mu.Unlock()
	lst := db.getOrCreateList(key)
	if len(lst.Values) > 0 {
		val := lst.Values[0]
		lst.Values = lst.Values[1:]
		return val, true
	}
	if lst.queue == nil {
		lst.queue = list.New()
	}
	lst.queue.PushBack(ch)
	return "", false
}

func (db *DB) LLEN(key string) int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if lst, exists := db.lists[key]; exists && lst != nil {
		return len(lst.Values)
	} else {
		return 0
	}
}
func (db *DB) LPopMany(key string, count int) []string {
	db.mu.Lock()
	defer db.mu.Unlock()
	if lst, exists := db.lists[key]; exists && lst != nil && len(lst.Values) > 0 {
		if count > len(lst.Values) {
			count = len(lst.Values)
		}
		vals := lst.Values[:count]
		lst.Values = lst.Values[count:]
		return vals
	}
	return []string{}
}
