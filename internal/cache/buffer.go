package cache

import (
	"container/list"
	"fmt"
	"sync"

	"github.com/yourname/fs-engine/internal/disk"
)

const defaultCacheSize = 256

// bufEntry is a cached buffer entry.
type bufEntry struct {
	addr     uint32
	data     []byte
	dirty    bool
	pinCount int
	lruElem  *list.Element
}

// CacheStats holds buffer cache statistics.
type CacheStats struct {
	Hits       uint64
	Misses     uint64
	Evictions  uint64
	Size       int
	Capacity   int
	DirtyAddrs []uint32 // addresses of currently dirty blocks
}

// BufferCache is an LRU write-back buffer cache.
type BufferCache struct {
	mu       sync.Mutex
	dev      *disk.BlockDevice
	cache    map[uint32]*bufEntry
	lru      *list.List
	capacity int
	stats    CacheStats
	emit     func(eventType string, fields map[string]interface{})
}

// NewBufferCache creates a new BufferCache backed by dev.
func NewBufferCache(dev *disk.BlockDevice, capacity int) *BufferCache {
	if capacity <= 0 {
		capacity = defaultCacheSize
	}
	return &BufferCache{
		dev:      dev,
		cache:    make(map[uint32]*bufEntry, capacity),
		lru:      list.New(),
		capacity: capacity,
	}
}

// SetEventHook registers a callback for cache lifecycle events.
func (c *BufferCache) SetEventHook(hook func(eventType string, fields map[string]interface{})) {
	c.mu.Lock()
	c.emit = hook
	c.mu.Unlock()
}

// GetBlock returns a copy of block addr, fetching from disk if not cached.
func (c *BufferCache) GetBlock(addr uint32) ([]byte, error) {
	c.mu.Lock()
	if e, ok := c.cache[addr]; ok {
		c.stats.Hits++
		e.pinCount++
		c.lru.MoveToFront(e.lruElem)
		out := make([]byte, len(e.data))
		copy(out, e.data)
		c.mu.Unlock()
		return out, nil
	}
	c.stats.Misses++
	c.mu.Unlock()

	// Fetch from disk without holding the cache lock
	blk, err := c.dev.ReadBlock(addr)
	if err != nil {
		return nil, fmt.Errorf("cache: read block %d: %w", addr, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Another goroutine may have loaded it while we were reading
	if e, ok := c.cache[addr]; ok {
		e.pinCount++
		c.lru.MoveToFront(e.lruElem)
		out := make([]byte, len(e.data))
		copy(out, e.data)
		return out, nil
	}

	// Evict if at capacity
	if err := c.evictIfNeeded(); err != nil {
		return nil, err
	}

	e := &bufEntry{
		addr:     addr,
		data:     blk,
		pinCount: 1,
	}
	elem := c.lru.PushFront(addr)
	e.lruElem = elem
	c.cache[addr] = e
	c.stats.Size = len(c.cache)

	out := make([]byte, len(blk))
	copy(out, blk)
	return out, nil
}

// PutBlock writes data back into the cache entry for addr.
// data must be exactly BlockSize bytes.
func (c *BufferCache) PutBlock(addr uint32, data []byte) error {
	if len(data) != 4096 {
		return fmt.Errorf("cache: PutBlock data must be 4096 bytes")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.cache[addr]; ok {
		copy(e.data, data)
		if e.pinCount > 0 {
			e.pinCount--
		}
		c.lru.MoveToFront(e.lruElem)
		return nil
	}

	// Evict if needed
	if err := c.evictIfNeeded(); err != nil {
		return err
	}

	cp := make([]byte, 4096)
	copy(cp, data)
	e := &bufEntry{
		addr:     addr,
		data:     cp,
		pinCount: 0,
	}
	elem := c.lru.PushFront(addr)
	e.lruElem = elem
	c.cache[addr] = e
	c.stats.Size = len(c.cache)
	return nil
}

// MarkDirty marks a cached block as dirty (needs flushing).
func (c *BufferCache) MarkDirty(addr uint32) {
	c.mu.Lock()
	if e, ok := c.cache[addr]; ok {
		e.dirty = true
	}
	c.mu.Unlock()
}

// WriteBlock writes data to the cache (and marks dirty). Equivalent to
// PutBlock + MarkDirty, but also updates cache data without unpin.
func (c *BufferCache) WriteBlock(addr uint32, data []byte) error {
	if len(data) != 4096 {
		return fmt.Errorf("cache: WriteBlock data must be 4096 bytes")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.cache[addr]; ok {
		copy(e.data, data)
		e.dirty = true
		c.lru.MoveToFront(e.lruElem)
		return nil
	}

	if err := c.evictIfNeeded(); err != nil {
		return err
	}

	cp := make([]byte, 4096)
	copy(cp, data)
	e := &bufEntry{
		addr:  addr,
		data:  cp,
		dirty: true,
	}
	elem := c.lru.PushFront(addr)
	e.lruElem = elem
	c.cache[addr] = e
	c.stats.Size = len(c.cache)
	return nil
}

// FlushBlock flushes a single block to disk if dirty.
func (c *BufferCache) FlushBlock(addr uint32) error {
	c.mu.Lock()
	e, ok := c.cache[addr]
	if !ok || !e.dirty {
		c.mu.Unlock()
		return nil
	}
	data := make([]byte, len(e.data))
	copy(data, e.data)
	c.mu.Unlock()

	if err := c.dev.WriteBlock(addr, data); err != nil {
		return err
	}

	c.mu.Lock()
	if e2, ok := c.cache[addr]; ok {
		e2.dirty = false
	}
	c.mu.Unlock()
	if c.emit != nil {
		c.emit("cache_flush", map[string]interface{}{"blockAddr": addr, "dirty": false})
	}
	return nil
}

// FlushAll flushes all dirty blocks to disk.
func (c *BufferCache) FlushAll() error {
	c.mu.Lock()
	dirty := make([]uint32, 0, len(c.cache))
	for addr, e := range c.cache {
		if e.dirty {
			dirty = append(dirty, addr)
		}
	}
	c.mu.Unlock()

	for _, addr := range dirty {
		if err := c.FlushBlock(addr); err != nil {
			return err
		}
	}
	return nil
}

// Invalidate removes a block from the cache without writing.
func (c *BufferCache) Invalidate(addr uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.cache[addr]; ok {
		c.lru.Remove(e.lruElem)
		delete(c.cache, addr)
		c.stats.Size = len(c.cache)
	}
}

// InvalidateAll drops every cached block without flushing.
func (c *BufferCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[uint32]*bufEntry, c.capacity)
	c.lru.Init()
	c.stats.Size = 0
}

// Stats returns a snapshot of cache statistics.
func (c *BufferCache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.stats
	s.Size = len(c.cache)
	s.Capacity = c.capacity
	// Collect dirty block addresses.
	s.DirtyAddrs = make([]uint32, 0)
	for addr, e := range c.cache {
		if e.dirty {
			s.DirtyAddrs = append(s.DirtyAddrs, addr)
		}
	}
	return s
}

// evictIfNeeded evicts the LRU unpinned entry if at capacity.
// Caller must hold c.mu.
func (c *BufferCache) evictIfNeeded() error {
	for len(c.cache) >= c.capacity {
		elem := c.lru.Back()
		if elem == nil {
			return fmt.Errorf("cache: all blocks pinned, cannot evict")
		}
		addr := elem.Value.(uint32)
		e := c.cache[addr]
		if e.pinCount > 0 {
			// All entries might be pinned - try walking forward
			prev := elem.Prev()
			for prev != nil {
				paddr := prev.Value.(uint32)
				pe := c.cache[paddr]
				if pe.pinCount == 0 {
					// evict this one
					if pe.dirty {
						data := make([]byte, len(pe.data))
						copy(data, pe.data)
						c.mu.Unlock()
						werr := c.dev.WriteBlock(paddr, data)
						c.mu.Lock()
						if werr != nil {
							return werr
						}
					}
					c.lru.Remove(pe.lruElem)
					delete(c.cache, paddr)
					c.stats.Evictions++
					c.stats.Size = len(c.cache)
					if c.emit != nil {
						c.emit("cache_evict", map[string]interface{}{"blockAddr": paddr, "dirty": pe.dirty})
					}
					return nil
				}
				prev = prev.Prev()
			}
			return fmt.Errorf("cache: all blocks pinned, cannot evict")
		}
		if e.dirty {
			data := make([]byte, len(e.data))
			copy(data, e.data)
			c.mu.Unlock()
			werr := c.dev.WriteBlock(addr, data)
			c.mu.Lock()
			if werr != nil {
				return werr
			}
			// Re-check the entry still exists
			if e2, ok := c.cache[addr]; ok {
				e2.dirty = false
			}
		}
		c.lru.Remove(e.lruElem)
		delete(c.cache, addr)
		c.stats.Evictions++
		c.stats.Size = len(c.cache)
		if c.emit != nil {
			c.emit("cache_evict", map[string]interface{}{"blockAddr": addr, "dirty": e.dirty})
		}
	}
	return nil
}
