package cache

import (
	"sync"
	"time"
)

// WritebackFlusher runs a background goroutine that periodically flushes
// the buffer cache. It stops when the done channel is closed.
type WritebackFlusher struct {
	cache    *BufferCache
	interval time.Duration
	done     chan struct{}
	once     sync.Once
	wg       sync.WaitGroup
}

// NewWritebackFlusher creates a new flusher.
func NewWritebackFlusher(c *BufferCache, interval time.Duration) *WritebackFlusher {
	return &WritebackFlusher{
		cache:    c,
		interval: interval,
		done:     make(chan struct{}),
	}
}

// Start launches the background flusher goroutine.
func (f *WritebackFlusher) Start() {
	f.wg.Add(1)
	go f.run()
}

// Stop signals the flusher to stop and waits for it to exit.
func (f *WritebackFlusher) Stop() {
	f.once.Do(func() {
		close(f.done)
	})
	f.wg.Wait()
}

func (f *WritebackFlusher) run() {
	defer f.wg.Done()
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = f.cache.FlushAll()
		case <-f.done:
			// Final flush on shutdown
			_ = f.cache.FlushAll()
			return
		}
	}
}
