package metrics

import (
	"sync/atomic"
)

// Metrics holds atomic counters for filesystem operations.
type Metrics struct {
	Reads             atomic.Int64
	Writes            atomic.Int64
	Creates           atomic.Int64
	Deletes           atomic.Int64
	Lookups           atomic.Int64
	MkdirOps          atomic.Int64
	RmdirOps          atomic.Int64
	LinkOps           atomic.Int64
	UnlinkOps         atomic.Int64
	RenameOps         atomic.Int64
	SymlinkOps        atomic.Int64
	StatOps           atomic.Int64
	ChmodOps          atomic.Int64
	ChownOps          atomic.Int64
	ReadBytes         atomic.Int64
	WriteBytes        atomic.Int64
	Errors            atomic.Int64
	CacheHits         atomic.Int64
	CacheMiss         atomic.Int64
	JournalCommits    atomic.Int64
	BlocksAllocated   atomic.Int64
	BlocksFreed       atomic.Int64
	InodesAllocated   atomic.Int64
	InodesFreed       atomic.Int64
	PermDenied        atomic.Int64
	FsyncCalls        atomic.Int64
	RecoveryRuns      atomic.Int64
	RecoveryErrors    atomic.Int64
	RecoveryBlocks    atomic.Int64
	MutationRollbacks atomic.Int64
	CacheFlushErrors  atomic.Int64
	CacheEvictions    atomic.Int64
	APIErrors         atomic.Int64

	// Windowed provides a 60-second rolling window of per-second deltas.
	Windowed *WindowedStats
}

// NewMetrics creates a new Metrics instance and starts the windowed stats ticker.
func NewMetrics() *Metrics {
	m := &Metrics{}
	m.Windowed = NewWindowedStats(m)
	m.Windowed.Start()
	return m
}

// Close stops the windowed stats background ticker.
func (m *Metrics) Close() {
	if m.Windowed != nil {
		m.Windowed.Stop()
	}
}

// Snapshot returns a non-atomic snapshot of all counters.
func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"reads":              m.Reads.Load(),
		"writes":             m.Writes.Load(),
		"creates":            m.Creates.Load(),
		"deletes":            m.Deletes.Load(),
		"lookups":            m.Lookups.Load(),
		"mkdirs":             m.MkdirOps.Load(),
		"rmdirs":             m.RmdirOps.Load(),
		"links":              m.LinkOps.Load(),
		"unlinks":            m.UnlinkOps.Load(),
		"renames":            m.RenameOps.Load(),
		"symlinks":           m.SymlinkOps.Load(),
		"stats":              m.StatOps.Load(),
		"chmods":             m.ChmodOps.Load(),
		"chowns":             m.ChownOps.Load(),
		"read_bytes":         m.ReadBytes.Load(),
		"write_bytes":        m.WriteBytes.Load(),
		"errors":             m.Errors.Load(),
		"cache_hits":         m.CacheHits.Load(),
		"cache_miss":         m.CacheMiss.Load(),
		"journal_commits":    m.JournalCommits.Load(),
		"blocks_allocated":   m.BlocksAllocated.Load(),
		"blocks_freed":       m.BlocksFreed.Load(),
		"inodes_allocated":   m.InodesAllocated.Load(),
		"inodes_freed":       m.InodesFreed.Load(),
		"perm_denied":        m.PermDenied.Load(),
		"fsync_calls":        m.FsyncCalls.Load(),
		"recovery_runs":      m.RecoveryRuns.Load(),
		"recovery_errors":    m.RecoveryErrors.Load(),
		"recovery_blocks":    m.RecoveryBlocks.Load(),
		"mutation_rollbacks": m.MutationRollbacks.Load(),
		"cache_flush_errors": m.CacheFlushErrors.Load(),
		"cache_evictions":    m.CacheEvictions.Load(),
		"api_errors":         m.APIErrors.Load(),
	}
}
