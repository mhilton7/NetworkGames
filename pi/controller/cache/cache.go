// Package cache implements a bounded, snapshot-aware, read-through RAM cache.
package cache

import (
	"container/list"
	"sync"
)

type Key struct {
	Snapshot string
	Offset   int64
	Length   int
	Version  uint16
}

type entry struct {
	key  Key
	data []byte
}

type Cache struct {
	mu       sync.Mutex
	maxBytes int
	bytes    int
	items    map[Key]*list.Element
	lru      *list.List
}

func New(maxBytes int) *Cache {
	if maxBytes < 0 {
		maxBytes = 0
	}
	return &Cache{maxBytes: maxBytes, items: make(map[Key]*list.Element), lru: list.New()}
}

func (c *Cache) Put(key Key, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(value) > c.maxBytes || key.Length != len(value) {
		return
	}
	if old, ok := c.items[key]; ok {
		c.remove(old)
	}
	copyValue := append([]byte(nil), value...)
	element := c.lru.PushFront(entry{key: key, data: copyValue})
	c.items[key] = element
	c.bytes += len(copyValue)
	for c.bytes > c.maxBytes {
		c.remove(c.lru.Back())
	}
}

func (c *Cache) Get(key Key) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(element)
	value := element.Value.(entry).data
	return append([]byte(nil), value...), true
}

func (c *Cache) Bytes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[Key]*list.Element)
	c.lru.Init()
	c.bytes = 0
}

func (c *Cache) remove(element *list.Element) {
	if element == nil {
		return
	}
	item := element.Value.(entry)
	delete(c.items, item.key)
	c.bytes -= len(item.data)
	c.lru.Remove(element)
}
