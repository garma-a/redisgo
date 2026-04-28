package store

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

type streamIDKind int

const (
	idExplicit streamIDKind = iota
	idAutoMsAndSeq
	idExplicitMsAutoSeq
)

type parsedStreamID struct {
	kind streamIDKind
	ms   uint64
	seq  uint64
}

type XRangeResult struct {
	ID     string
	Fields []string
}

type value struct {
	Data      string
	ExpiresAt time.Time
}

type list struct {
	values  []string
	waiters []chan string
}

type streamField struct {
	Name  string
	Value string
}

type streamEntry struct {
	ID           string
	Milliseconds uint64
	Sequence     uint64
	Fields       []streamField
}

type stream struct {
	entries []streamEntry
}

type DB struct {
	mu      sync.RWMutex
	data    map[string]value
	lists   map[string]*list
	streams map[string]*stream
}

var (
	ErrXAddInvalidID  = errors.New("invalid stream id")
	ErrXAddIDTooSmall = errors.New("xadd id is equal or smaller than stream top")
	ErrXAddIDZero     = errors.New("xadd id must be greater than 0-0")
)

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

func newStream() *stream {
	return &stream{
		entries: []streamEntry{},
	}
}

func (db *DB) getOrCreateStream(key string) *stream {
	if stm, exists := db.streams[key]; exists && stm != nil {
		return stm
	}
	newStm := newStream()
	db.streams[key] = newStm
	return newStm
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
		data:    make(map[string]value),
		lists:   make(map[string]*list),
		streams: make(map[string]*stream),
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

func parseExplicitStreamID(id string) (parsedStreamID, error) {
	if id == "*" {
		return parsedStreamID{kind: idAutoMsAndSeq}, nil
	}

	parts := strings.Split(id, "-")
	if len(parts) != 2 {
		return parsedStreamID{}, ErrXAddInvalidID
	}

	milliseconds, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return parsedStreamID{}, ErrXAddInvalidID
	}

	if parts[1] == "*" {
		return parsedStreamID{kind: idExplicitMsAutoSeq, ms: milliseconds}, nil
	}

	sequence, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return parsedStreamID{}, ErrXAddInvalidID
	}

	return parsedStreamID{kind: idExplicit, ms: milliseconds, seq: sequence}, nil
}

func resolveStreamID(parsedID parsedStreamID, hasLast bool, lastMilliseconds, lastSequence uint64) (uint64, uint64, error) {
	switch parsedID.kind {
	case idAutoMsAndSeq:
		milliseconds := uint64(time.Now().UnixMilli())
		if hasLast && milliseconds < lastMilliseconds {
			milliseconds = lastMilliseconds
		}
		if hasLast && milliseconds == lastMilliseconds {
			return milliseconds, lastSequence + 1, nil
		}
		if milliseconds == 0 {
			return 0, 1, nil
		}
		return milliseconds, 0, nil

	case idExplicitMsAutoSeq:
		milliseconds := parsedID.ms
		if hasLast {
			if milliseconds < lastMilliseconds {
				return 0, 0, ErrXAddIDTooSmall
			}
			if milliseconds == lastMilliseconds {
				return milliseconds, lastSequence + 1, nil
			}
		}
		if milliseconds == 0 {
			return 0, 1, nil
		}
		return milliseconds, 0, nil

	case idExplicit:
		milliseconds := parsedID.ms
		sequence := parsedID.seq
		if milliseconds == 0 && sequence == 0 {
			return 0, 0, ErrXAddIDZero
		}
		if hasLast && !isIDStrictlyGreater(milliseconds, sequence, lastMilliseconds, lastSequence) {
			return 0, 0, ErrXAddIDTooSmall
		}
		return milliseconds, sequence, nil
	}

	return 0, 0, ErrXAddInvalidID
}

func isIDStrictlyGreater(milliseconds, sequence, lastMilliseconds, lastSequence uint64) bool {
	if milliseconds > lastMilliseconds {
		return true
	}
	if milliseconds < lastMilliseconds {
		return false
	}
	return sequence > lastSequence
}

func compareStreamID(aMilliseconds, aSequence, bMilliseconds, bSequence uint64) int {
	if aMilliseconds < bMilliseconds {
		return -1
	}
	if aMilliseconds > bMilliseconds {
		return 1
	}
	if aSequence < bSequence {
		return -1
	}
	if aSequence > bSequence {
		return 1
	}
	return 0
}

func parseXRangeBound(id string, isStart bool) (uint64, uint64, error) {
	if id == "-" {
		return 0, 0, nil
	}
	if id == "+" {
		max := ^uint64(0)
		return max, max, nil
	}

	parts := strings.Split(id, "-")
	if len(parts) == 1 {
		milliseconds, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return 0, 0, ErrXAddInvalidID
		}
		if isStart {
			return milliseconds, 0, nil
		}
		return milliseconds, ^uint64(0), nil
	}

	if len(parts) != 2 {
		return 0, 0, ErrXAddInvalidID
	}

	milliseconds, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, 0, ErrXAddInvalidID
	}
	sequence, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0, 0, ErrXAddInvalidID
	}

	return milliseconds, sequence, nil
}

func (db *DB) XAdd(key, id string, fields []string) (string, error) {
	parsedID, err := parseExplicitStreamID(id)
	if err != nil {
		return "", err
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	var (
		hasLast          bool
		lastMilliseconds uint64
		lastSequence     uint64
	)
	if stm, exists := db.streams[key]; exists && stm != nil && len(stm.entries) > 0 {
		last := stm.entries[len(stm.entries)-1]
		hasLast = true
		lastMilliseconds = last.Milliseconds
		lastSequence = last.Sequence
	}

	milliseconds, sequence, err := resolveStreamID(parsedID, hasLast, lastMilliseconds, lastSequence)
	if err != nil {
		return "", err
	}

	resolvedID := strconv.FormatUint(milliseconds, 10) + "-" + strconv.FormatUint(sequence, 10)

	stm := db.getOrCreateStream(key)
	entryFields := make([]streamField, 0, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		entryFields = append(entryFields, streamField{
			Name:  fields[i],
			Value: fields[i+1],
		})
	}

	stm.entries = append(stm.entries, streamEntry{
		ID:           resolvedID,
		Milliseconds: milliseconds,
		Sequence:     sequence,
		Fields:       entryFields,
	})

	return resolvedID, nil
}

func (db *DB) XRange(key, startID, endID string) ([]XRangeResult, error) {
	startMilliseconds, startSequence, err := parseXRangeBound(startID, true)
	if err != nil {
		return nil, err
	}
	endMilliseconds, endSequence, err := parseXRangeBound(endID, false)
	if err != nil {
		return nil, err
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	stm := db.streams[key]
	if stm == nil || len(stm.entries) == 0 {
		return []XRangeResult{}, nil
	}

	result := make([]XRangeResult, 0)
	for _, entry := range stm.entries {
		if compareStreamID(entry.Milliseconds, entry.Sequence, startMilliseconds, startSequence) < 0 {
			continue
		}
		if compareStreamID(entry.Milliseconds, entry.Sequence, endMilliseconds, endSequence) > 0 {
			continue
		}

		fields := make([]string, 0, len(entry.Fields)*2)
		for _, field := range entry.Fields {
			fields = append(fields, field.Name, field.Value)
		}

		result = append(result, XRangeResult{
			ID:     entry.ID,
			Fields: fields,
		})
	}

	return result, nil
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
	db.mu.RLock()
	defer db.mu.RUnlock()

	if _, ok := db.streams[key]; ok {
		return "stream"
	} else if _, ok := db.lists[key]; ok {
		return "list"
	} else if _, ok := db.data[key]; ok {
		return "string"
	} else {
		return "none"
	}
}

func (db *DB) Incr(key string) (int, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	val, exists := db.data[key]
	if !exists {
		db.data[key] = value{Data: "1"}
		return 1, nil
	}
	num, err := strconv.Atoi(val.Data)
	if err != nil {
		return 0, errors.New("value is not an integer")
	}
	num++
	db.data[key] = value{Data: strconv.Itoa(num)}
	return num, nil
}
