package journal

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yourname/fs-engine/internal/disk"
	"github.com/yourname/fs-engine/internal/metrics"
)

// TxRecord is an in-memory record of a committed transaction (for inspection).
type TxRecord struct {
	ID        uint32    `json:"id"`
	Status    string    `json:"status"`
	Blocks    []uint32  `json:"blocks"`
	Timestamp time.Time `json:"timestamp"`
	Ops       []string  `json:"operations"`
}

// Journal manages a write-ahead log for ordered journaling.
type Journal struct {
	mu       sync.Mutex
	dev      *disk.BlockDevice
	start    uint32 // first block of journal area
	length   uint32 // number of blocks in journal area
	sequence atomic.Uint32
	head     uint32 // next free block offset (relative to start)
	metrics  *metrics.Metrics
	emit     func(eventType string, fields map[string]interface{})

	// Recent transaction history (ring buffer, max 64 entries).
	histMu  sync.RWMutex
	history [64]TxRecord
	histLen int
	histPos int
}

// recordTx appends a committed transaction to the in-memory history.
func (j *Journal) recordTx(tx TxRecord) {
	j.histMu.Lock()
	j.history[j.histPos%64] = tx
	j.histPos++
	if j.histLen < 64 {
		j.histLen++
	}
	j.histMu.Unlock()
}

// RecentTransactions returns up to n most-recent committed transactions (newest first).
func (j *Journal) RecentTransactions(n int) []TxRecord {
	j.histMu.RLock()
	defer j.histMu.RUnlock()
	if n > j.histLen {
		n = j.histLen
	}
	result := make([]TxRecord, 0, n)
	for i := 0; i < n; i++ {
		idx := (j.histPos - 1 - i + 64) % 64
		result = append(result, j.history[idx])
	}
	return result
}

// NewJournal creates a Journal that manages the given block range.
func NewJournal(dev *disk.BlockDevice, start, length uint32, m *metrics.Metrics) *Journal {
	j := &Journal{
		dev:     dev,
		start:   start,
		length:  length,
		head:    0,
		metrics: m,
	}
	j.sequence.Store(1)
	return j
}

// SetEventHook registers a callback for journal lifecycle events.
func (j *Journal) SetEventHook(hook func(eventType string, fields map[string]interface{})) {
	j.mu.Lock()
	j.emit = hook
	j.mu.Unlock()
}

// Init formats the journal area (writes zeros).
func (j *Journal) Init() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	zero := make([]byte, BlockSize)
	for i := uint32(0); i < j.length; i++ {
		if err := j.dev.WriteBlock(j.start+i, zero); err != nil {
			return fmt.Errorf("journal: init block %d: %w", j.start+i, err)
		}
	}
	j.head = 0
	j.sequence.Store(1)
	return nil
}

// OpenTransaction begins a new journal transaction.
func (j *Journal) OpenTransaction() *JournalTransaction {
	seq := j.sequence.Add(1)
	return &JournalTransaction{
		journal:  j,
		sequence: seq,
		blocks:   make([]txBlock, 0, 8),
	}
}

// Checkpoint replays committed transactions and advances the head.
// In this simplified implementation, we simply zero the journal area.
func (j *Journal) Checkpoint() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	zero := make([]byte, BlockSize)
	for i := uint32(0); i < j.length; i++ {
		if err := j.dev.WriteBlock(j.start+i, zero); err != nil {
			return fmt.Errorf("journal: checkpoint block %d: %w", j.start+i, err)
		}
	}
	j.head = 0
	return nil
}

// allocBlocks reserves n consecutive journal blocks (relative offsets returned).
// Caller must hold j.mu.
func (j *Journal) allocBlocks(n uint32) (uint32, error) {
	if n > j.length {
		return 0, fmt.Errorf("journal: transaction too large (%d blocks, max %d)", n, j.length)
	}
	if j.head+n > j.length {
		// Wrap around: checkpoint first
		j.head = 0
	}
	off := j.head
	j.head += n
	return off, nil
}

// writeJournalBlock writes data to the absolute journal block address.
func (j *Journal) writeJournalBlock(offset uint32, data []byte) error {
	return j.dev.WriteBlock(j.start+offset, data)
}

// readJournalBlock reads a journal block by absolute offset.
func (j *Journal) readJournalBlock(offset uint32) ([]byte, error) {
	return j.dev.ReadBlock(j.start + offset)
}

// CurrentSequence returns the current journal sequence number.
func (j *Journal) CurrentSequence() uint32 {
	return j.sequence.Load()
}
