package cache

import (
	"fmt"
	"sync"
)

const defaultInodeCacheSize = 512

// InodeData is an opaque 128-byte inode payload stored in the cache.
type InodeData [128]byte

// inodeEntry holds a cached inode.
type inodeEntry struct {
	inum  uint32
	data  InodeData
	dirty bool
	refs  int
}

// FlushFn is called by the inode cache when it needs to flush an inode to disk.
type FlushFn func(inum uint32, data InodeData) error

// InodeCache caches disk inodes in memory.
type InodeCache struct {
	mu       sync.Mutex
	cache    map[uint32]*inodeEntry
	capacity int
	flushFn  FlushFn
}

// NewInodeCache creates an InodeCache with the given flush function.
func NewInodeCache(capacity int, flushFn FlushFn) *InodeCache {
	if capacity <= 0 {
		capacity = defaultInodeCacheSize
	}
	return &InodeCache{
		cache:    make(map[uint32]*inodeEntry, capacity),
		capacity: capacity,
		flushFn:  flushFn,
	}
}

// Get returns the cached inode data for inum, or (zero, false) if not cached.
func (ic *InodeCache) Get(inum uint32) (InodeData, bool) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	e, ok := ic.cache[inum]
	if !ok {
		return InodeData{}, false
	}
	e.refs++
	return e.data, true
}

// Put stores inode data in the cache.
func (ic *InodeCache) Put(inum uint32, data InodeData) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if e, ok := ic.cache[inum]; ok {
		e.data = data
		return
	}
	if len(ic.cache) >= ic.capacity {
		ic.evictOne()
	}
	ic.cache[inum] = &inodeEntry{inum: inum, data: data}
}

// Update stores updated inode data and marks it dirty.
func (ic *InodeCache) Update(inum uint32, data InodeData) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if e, ok := ic.cache[inum]; ok {
		e.data = data
		e.dirty = true
		return
	}
	if len(ic.cache) >= ic.capacity {
		ic.evictOne()
	}
	ic.cache[inum] = &inodeEntry{inum: inum, data: data, dirty: true}
}

// MarkDirty marks the cached inode as needing to be flushed.
func (ic *InodeCache) MarkDirty(inum uint32) {
	ic.mu.Lock()
	if e, ok := ic.cache[inum]; ok {
		e.dirty = true
	}
	ic.mu.Unlock()
}

// FlushInode writes a dirty inode to disk via the flush function.
func (ic *InodeCache) FlushInode(inum uint32) error {
	ic.mu.Lock()
	e, ok := ic.cache[inum]
	if !ok || !e.dirty {
		ic.mu.Unlock()
		return nil
	}
	data := e.data
	ic.mu.Unlock()

	if ic.flushFn != nil {
		if err := ic.flushFn(inum, data); err != nil {
			return fmt.Errorf("icache: flush inode %d: %w", inum, err)
		}
	}

	ic.mu.Lock()
	if e2, ok := ic.cache[inum]; ok {
		e2.dirty = false
	}
	ic.mu.Unlock()
	return nil
}

// FlushAll writes all dirty inodes to disk.
func (ic *InodeCache) FlushAll() error {
	ic.mu.Lock()
	inums := make([]uint32, 0, len(ic.cache))
	for inum, e := range ic.cache {
		if e.dirty {
			inums = append(inums, inum)
		}
	}
	ic.mu.Unlock()

	for _, inum := range inums {
		if err := ic.FlushInode(inum); err != nil {
			return err
		}
	}
	return nil
}

// Invalidate removes an inode from the cache.
func (ic *InodeCache) Invalidate(inum uint32) {
	ic.mu.Lock()
	delete(ic.cache, inum)
	ic.mu.Unlock()
}

// InvalidateAll drops every cached inode without flushing.
func (ic *InodeCache) InvalidateAll() {
	ic.mu.Lock()
	ic.cache = make(map[uint32]*inodeEntry, ic.capacity)
	ic.mu.Unlock()
}

// evictOne evicts the first unreferenced clean entry found.
// Caller must hold ic.mu.
func (ic *InodeCache) evictOne() {
	for inum, e := range ic.cache {
		if e.refs == 0 && !e.dirty {
			delete(ic.cache, inum)
			return
		}
	}
	for inum, e := range ic.cache {
		if e.refs == 0 {
			delete(ic.cache, inum)
			return
		}
	}
}
