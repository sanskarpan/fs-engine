package metrics

import (
	"sync"
	"time"
)

// BucketSnapshot is one second of per-counter delta values.
type BucketSnapshot struct {
	TS              int64 `json:"ts"`
	Reads           int64 `json:"reads"`
	Writes          int64 `json:"writes"`
	CacheHits       int64 `json:"cache_hits"`
	CacheMiss       int64 `json:"cache_miss"`
	JournalCommits  int64 `json:"journal_commits"`
	BlocksAllocated int64 `json:"blocks_allocated"`
	BlocksFreed     int64 `json:"blocks_freed"`
	InodesAllocated int64 `json:"inodes_allocated"`
	InodesFreed     int64 `json:"inodes_freed"`
}

// WindowedStats maintains a 60-second rolling window of per-second metric deltas.
// Call Start() to begin ticking; call Stop() to shut down the background goroutine.
type WindowedStats struct {
	mu      sync.Mutex
	ring    [60]BucketSnapshot
	head    int
	filled  int
	prev    map[string]int64
	metrics *Metrics
	stop    chan struct{}
	once    sync.Once
	wg      sync.WaitGroup
}

// NewWindowedStats creates a WindowedStats linked to m.
func NewWindowedStats(m *Metrics) *WindowedStats {
	return &WindowedStats{
		metrics: m,
		prev:    m.Snapshot(),
		stop:    make(chan struct{}),
	}
}

// Start launches a background goroutine that calls Tick every second.
func (w *WindowedStats) Start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				w.Tick()
			case <-w.stop:
				return
			}
		}
	}()
}

// Stop shuts down the background ticker goroutine.
func (w *WindowedStats) Stop() {
	w.once.Do(func() {
		close(w.stop)
	})
	w.wg.Wait()
}

// Tick records one second of counter deltas into the ring buffer.
func (w *WindowedStats) Tick() {
	curr := w.metrics.Snapshot()
	w.mu.Lock()
	defer w.mu.Unlock()

	bucket := BucketSnapshot{
		TS:              time.Now().Unix(),
		Reads:           curr["reads"] - w.prev["reads"],
		Writes:          curr["writes"] - w.prev["writes"],
		CacheHits:       curr["cache_hits"] - w.prev["cache_hits"],
		CacheMiss:       curr["cache_miss"] - w.prev["cache_miss"],
		JournalCommits:  curr["journal_commits"] - w.prev["journal_commits"],
		BlocksAllocated: curr["blocks_allocated"] - w.prev["blocks_allocated"],
		BlocksFreed:     curr["blocks_freed"] - w.prev["blocks_freed"],
		InodesAllocated: curr["inodes_allocated"] - w.prev["inodes_allocated"],
		InodesFreed:     curr["inodes_freed"] - w.prev["inodes_freed"],
	}
	w.ring[w.head%60] = bucket
	w.head++
	if w.filled < 60 {
		w.filled++
	}
	w.prev = curr
}

// Window returns up to 60 seconds of per-second data in chronological order.
func (w *WindowedStats) Window() []BucketSnapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]BucketSnapshot, w.filled)
	start := w.head - w.filled
	for i := 0; i < w.filled; i++ {
		result[i] = w.ring[(start+i+60)%60]
	}
	return result
}
