// Package cache provides the Cache port implementations: an in-process LRU and
// Redis. The LRU is the default on edge devices; Redis is used when several
// AIOS instances share state.
package cache

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Memory is a bounded LRU cache with per-entry TTL. It is safe for concurrent
// use and never grows beyond MaxEntries (protecting a 1 GB host).
type Memory struct {
	mu         sync.Mutex
	maxEntries int
	ttlDefault time.Duration
	items      map[string]*list.Element
	order      *list.List // front = most recently used
}

type entry struct {
	key     string
	value   []byte
	expires time.Time // zero = no expiry
}

// NewMemory builds an LRU cache. maxEntries <= 0 defaults to 10k.
func NewMemory(maxEntries int, ttlDefault time.Duration) *Memory {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	return &Memory{
		maxEntries: maxEntries,
		ttlDefault: ttlDefault,
		items:      make(map[string]*list.Element, maxEntries/4),
		order:      list.New(),
	}
}

var _ ports.Cache = (*Memory)(nil)

// Get returns a copy of the cached bytes.
func (m *Memory) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	el, ok := m.items[key]
	if !ok {
		return nil, false, nil
	}
	e := el.Value.(*entry)
	if !e.expires.IsZero() && time.Now().After(e.expires) {
		m.removeElement(el)
		return nil, false, nil
	}
	m.order.MoveToFront(el)
	out := make([]byte, len(e.value))
	copy(out, e.value)
	return out, true, nil
}

// Set stores bytes, evicting the least-recently-used entry when full.
func (m *Memory) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = m.ttlDefault
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	exp := time.Time{}
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}
	if el, ok := m.items[key]; ok {
		e := el.Value.(*entry)
		e.value = append(e.value[:0], value...)
		e.expires = exp
		m.order.MoveToFront(el)
		return nil
	}
	el := m.order.PushFront(&entry{key: key, value: append([]byte(nil), value...), expires: exp})
	m.items[key] = el
	for m.order.Len() > m.maxEntries {
		if back := m.order.Back(); back != nil {
			m.removeElement(back)
		}
	}
	return nil
}

// Delete removes keys.
func (m *Memory) Delete(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		if el, ok := m.items[k]; ok {
			m.removeElement(el)
		}
	}
	return nil
}

// Incr increments an integer counter atomically inside the LRU.
func (m *Memory) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if el, ok := m.items[key]; ok {
		e := el.Value.(*entry)
		if e.expires.IsZero() || now.Before(e.expires) {
			n := parseInt(e.value) + 1
			e.value = []byte(formatInt(n))
			m.order.MoveToFront(el)
			return n, nil
		}
		m.removeElement(el)
	}
	el := m.order.PushFront(&entry{key: key, value: []byte("1"), expires: now.Add(ttl)})
	m.items[key] = el
	for m.order.Len() > m.maxEntries {
		if back := m.order.Back(); back != nil {
			m.removeElement(back)
		}
	}
	return 1, nil
}

func (m *Memory) removeElement(el *list.Element) {
	e := el.Value.(*entry)
	delete(m.items, e.key)
	m.order.Remove(el)
}

// Ping always succeeds for the local cache.
func (m *Memory) Ping(context.Context) error { return nil }

// Close drops all entries.
func (m *Memory) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make(map[string]*list.Element)
	m.order.Init()
	return nil
}

// Len reports the current entry count (metrics/tests).
func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.order.Len()
}

func parseInt(b []byte) int64 {
	var n int64
	for _, c := range b {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

func formatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
