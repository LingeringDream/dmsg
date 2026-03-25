package msg

import (
	"container/list"
	"sync"
)

// DedupCache is an LRU cache for message ID deduplication.
type DedupCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
}

type dedupEntry struct {
	key string
}

// NewDedupCache creates a dedup cache with the given capacity.
func NewDedupCache(capacity int) *DedupCache {
	return &DedupCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// Seen returns true if the ID has already been seen, and marks it as seen.
func (dc *DedupCache) Seen(id string) bool {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if el, ok := dc.items[id]; ok {
		// Move to front (most recently seen)
		dc.order.MoveToFront(el)
		return true
	}

	// Add new entry
	if dc.order.Len() >= dc.capacity {
		// Evict oldest
		oldest := dc.order.Back()
		if oldest != nil {
			dc.order.Remove(oldest)
			delete(dc.items, oldest.Value.(dedupEntry).key)
		}
	}
	el := dc.order.PushFront(dedupEntry{key: id})
	dc.items[id] = el
	return false
}

// Size returns the current cache size.
func (dc *DedupCache) Size() int {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	return dc.order.Len()
}
