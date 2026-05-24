package fs

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yourname/fs-engine/internal/cache"
	"github.com/yourname/fs-engine/internal/disk"
	"github.com/yourname/fs-engine/internal/journal"
	"github.com/yourname/fs-engine/internal/metrics"
)

// Event types for SSE.
const (
	EventSchemaVersion = 1

	EventCreate = "create"
	EventDelete = "delete"
	EventWrite  = "write"
	EventMkdir  = "mkdir"
	EventRmdir  = "rmdir"
	EventRename = "rename"
	EventLink   = "link"
	EventMount  = "mount"
	EventUmount = "umount"

	EventBlockAlloc       = "block_alloc"
	EventBlockFree        = "block_free"
	EventInodeAlloc       = "inode_alloc"
	EventInodeFree        = "inode_free"
	EventCacheEvict       = "cache_evict"
	EventCacheFlush       = "cache_flush"
	EventJournalCommit    = "journal_commit"
	EventJournalRecover   = "journal_recover"
	EventMutationRollback = "mutation_rollback"

	EventSourceFS        = "fs"
	EventSourceAllocator = "allocator"
	EventSourceCache     = "cache"
	EventSourceJournal   = "journal"
)

// Event is emitted by the filesystem for SSE consumers.
type Event struct {
	Version int                    `json:"version"`
	Type    string                 `json:"type"`
	Source  string                 `json:"source"`
	Path    string                 `json:"path,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
	TS      time.Time              `json:"ts"`
}

// Filesystem is the central filesystem object. It ties together all subsystems.
type Filesystem struct {
	dev        *disk.BlockDevice
	sb         *DiskSuperblock
	bufCache   *cache.BufferCache
	inodeCache *cache.InodeCache
	alloc      *Allocator
	journal    *journal.Journal
	flusher    *cache.WritebackFlusher
	mu         sync.RWMutex
	metrics    *metrics.Metrics
	Events     chan Event
	closed     bool
	mutateMu   sync.Mutex
	mutating   atomic.Bool
}

// Mount opens an existing filesystem from dev. If the filesystem is new
// (mkfs not yet run), it returns ErrBadFS.
func Mount(dev *disk.BlockDevice) (*Filesystem, error) {
	// Try to recover journal first
	if err := journal.Recover(dev, JournalStart, JournalBlocks); err != nil {
		return nil, fmt.Errorf("mount: journal recover: %w", err)
	}

	sb, err := ReadSuperblock(dev)
	if err != nil {
		return nil, fmt.Errorf("mount: read superblock: %w", err)
	}
	if err := sb.Validate(); err != nil {
		return nil, fmt.Errorf("mount: validate superblock: %w", err)
	}

	blockBM, err := ReadBlockBitmap(dev)
	if err != nil {
		return nil, fmt.Errorf("mount: read block bitmap: %w", err)
	}
	inodeBM, err := ReadInodeBitmap(dev)
	if err != nil {
		return nil, fmt.Errorf("mount: read inode bitmap: %w", err)
	}

	bufCache := cache.NewBufferCache(dev, 256)
	m := metrics.NewMetrics()
	alloc := NewAllocator(dev, sb, blockBM, inodeBM, m)
	j := journal.NewJournal(dev, JournalStart, JournalBlocks, m)

	// Flush function for the inode cache: writes a raw 128-byte inode back to disk.
	flushFn := func(inum uint32, data cache.InodeData) error {
		blockAddr, offset := InodeNumToBlock(inum)
		blk, err := dev.ReadBlock(blockAddr)
		if err != nil {
			return err
		}
		copy(blk[offset:offset+InodeSize], data[:])
		return dev.WriteBlock(blockAddr, blk)
	}
	inodeCache := cache.NewInodeCache(512, flushFn)

	flusher := cache.NewWritebackFlusher(bufCache, 5*time.Second)
	flusher.Start()

	// Update mount info
	sb.MountCount++
	sb.LastMount = uint32(time.Now().Unix())
	sb.State = StateClean
	if err := WriteSuperblock(dev, sb); err != nil {
		return nil, fmt.Errorf("mount: update superblock: %w", err)
	}

	fs := &Filesystem{
		dev:        dev,
		sb:         sb,
		bufCache:   bufCache,
		inodeCache: inodeCache,
		alloc:      alloc,
		journal:    j,
		flusher:    flusher,
		metrics:    m,
		Events:     make(chan Event, 64),
	}

	bufCache.SetEventHook(func(eventType string, fields map[string]interface{}) {
		if m != nil && eventType == EventCacheEvict {
			m.CacheEvictions.Add(1)
		}
		fs.EmitTypedEvent(eventType, EventSourceCache, "", fields)
	})
	alloc.SetEventHook(func(eventType string, fields map[string]interface{}) {
		fs.EmitTypedEvent(eventType, EventSourceAllocator, "", fields)
	})
	j.SetEventHook(func(eventType string, fields map[string]interface{}) {
		fs.EmitTypedEvent(eventType, EventSourceJournal, "", fields)
	})

	m.Writes.Add(1) // mount event
	fs.EmitTypedEvent(EventMount, EventSourceFS, "/", nil)
	return fs, nil
}

// Unmount flushes caches and closes the filesystem.
func (fs *Filesystem) Unmount() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.closed {
		return nil
	}
	fs.closed = true

	fs.flusher.Stop()
	if fs.metrics != nil {
		fs.metrics.Close()
	}

	if err := fs.inodeCache.FlushAll(); err != nil {
		if fs.metrics != nil {
			fs.metrics.CacheFlushErrors.Add(1)
		}
		return fmt.Errorf("unmount: flush inodes: %w", err)
	}
	if err := fs.bufCache.FlushAll(); err != nil {
		if fs.metrics != nil {
			fs.metrics.CacheFlushErrors.Add(1)
		}
		return fmt.Errorf("unmount: flush blocks: %w", err)
	}

	// Update superblock
	fs.sb.State = StateClean
	fs.sb.LastWrite = uint32(time.Now().Unix())
	if err := WriteSuperblock(fs.dev, fs.sb); err != nil {
		return fmt.Errorf("unmount: write superblock: %w", err)
	}

	fs.EmitTypedEvent(EventUmount, EventSourceFS, "/", nil)
	return fs.dev.Sync()
}

// Fsck performs a basic filesystem consistency check.
func (fs *Filesystem) Fsck() error {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	sb, err := ReadSuperblock(fs.dev)
	if err != nil {
		return fmt.Errorf("fsck: read superblock: %w", err)
	}
	if err := sb.Validate(); err != nil {
		return fmt.Errorf("fsck: invalid superblock: %w", err)
	}

	// Read root inode
	root, err := ReadInode(fs.dev, 1)
	if err != nil {
		return fmt.Errorf("fsck: read root inode: %w", err)
	}
	if !root.IsDir() {
		return fmt.Errorf("fsck: root inode is not a directory: %w", ErrBadFS)
	}
	if root.Links < 2 {
		return fmt.Errorf("fsck: root inode has too few links: %w", ErrBadFS)
	}

	return nil
}

// Df returns filesystem disk usage statistics.
func (fs *Filesystem) Df() (total, used, free uint64) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	total = uint64(fs.sb.BlockCount) * BlockSize
	free = uint64(fs.sb.FreeBlocks) * BlockSize
	if total > free {
		used = total - free
	}
	return
}

// Dev returns the underlying block device.
func (fs *Filesystem) Dev() *disk.BlockDevice {
	return fs.dev
}

// Superblock returns the in-memory superblock.
func (fs *Filesystem) Superblock() *DiskSuperblock {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.sb
}

// Alloc returns the allocator.
func (fs *Filesystem) Alloc() *Allocator {
	return fs.alloc
}

// Journal returns the journal.
func (fs *Filesystem) Journal() *journal.Journal {
	return fs.journal
}

// GetMetrics returns the metrics tracker.
func (fs *Filesystem) GetMetrics() *metrics.Metrics {
	return fs.metrics
}

// InodeCache returns the inode cache.
func (fs *Filesystem) InodeCache() *cache.InodeCache {
	return fs.inodeCache
}

// BufCache returns the buffer cache.
func (fs *Filesystem) BufCache() *cache.BufferCache {
	return fs.bufCache
}

// EmitEvent sends an event to the SSE channel (non-blocking).
func (fs *Filesystem) EmitEvent(eventType, path string, data interface{}) {
	fields, _ := data.(map[string]interface{})
	fs.EmitTypedEvent(eventType, EventSourceFS, path, fields)
}

// EmitTypedEvent sends a structured event to the SSE channel (non-blocking).
func (fs *Filesystem) EmitTypedEvent(eventType, source, path string, data map[string]interface{}) {
	select {
	case fs.Events <- Event{
		Version: EventSchemaVersion,
		Type:    eventType,
		Source:  source,
		Path:    path,
		Data:    data,
		TS:      time.Now(),
	}:
	default:
	}
}

// ReadInode reads an inode via the inode cache.
func (fs *Filesystem) ReadInode(inum uint32) (*DiskInode, error) {
	// Try cache first
	if data, ok := fs.inodeCache.Get(inum); ok {
		di := &DiskInode{}
		decodeInode(data[:], di)
		return di, nil
	}

	// Load from disk
	di, err := ReadInode(fs.dev, inum)
	if err != nil {
		return nil, err
	}

	// Cache it
	var data cache.InodeData
	encodeInode(di, data[:])
	fs.inodeCache.Put(inum, data)

	return di, nil
}

// WriteInodeFS writes an inode via the inode cache and journal.
func (fs *Filesystem) WriteInodeFS(inum uint32, di *DiskInode) error {
	var data cache.InodeData
	encodeInode(di, data[:])
	fs.inodeCache.Update(inum, data)

	if fs.mutating.Load() {
		blockAddr, off := InodeNumToBlock(inum)
		blk, err := fs.dev.ReadBlock(blockAddr)
		if err != nil {
			return err
		}
		copy(blk[off:off+InodeSize], data[:])
		return fs.dev.WriteBlock(blockAddr, blk)
	}

	// Journal the inode block
	blockAddr, _ := InodeNumToBlock(inum)
	blk, err := fs.dev.ReadBlock(blockAddr)
	if err != nil {
		return err
	}
	_, off := InodeNumToBlock(inum)
	copy(blk[off:off+InodeSize], data[:])

	tx := fs.journal.OpenTransaction()
	if err := tx.LogBlock(blockAddr, blk); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateInodeCache updates only the in-memory inode cache entry.
func (fs *Filesystem) UpdateInodeCache(inum uint32, di *DiskInode) {
	var data cache.InodeData
	encodeInode(di, data[:])
	fs.inodeCache.Update(inum, data)
}

// UpdateInodeCacheRaw updates the inode cache with an already-encoded inode.
func (fs *Filesystem) UpdateInodeCacheRaw(inum uint32, raw []byte) {
	var data cache.InodeData
	copy(data[:], raw)
	fs.inodeCache.Update(inum, data)
}

// InvalidateInodeCache removes an inode from the inode cache.
func (fs *Filesystem) InvalidateInodeCache(inum uint32) {
	fs.inodeCache.Invalidate(inum)
}

func (fs *Filesystem) resetCaches() {
	fs.inodeCache.InvalidateAll()
	fs.bufCache.InvalidateAll()
}

func (fs *Filesystem) reloadFromDevice() error {
	sb, err := ReadSuperblock(fs.dev)
	if err != nil {
		return err
	}
	blockBM, err := ReadBlockBitmap(fs.dev)
	if err != nil {
		return err
	}
	inodeBM, err := ReadInodeBitmap(fs.dev)
	if err != nil {
		return err
	}
	fs.sb = sb
	fs.alloc = NewAllocator(fs.dev, sb, blockBM, inodeBM, fs.metrics)
	fs.alloc.SetEventHook(func(eventType string, fields map[string]interface{}) {
		fs.EmitTypedEvent(eventType, EventSourceAllocator, "", fields)
	})
	fs.resetCaches()
	return nil
}

// RunMutation snapshots the current device state, executes fn, journals the
// resulting changed block set, and only then applies the home writes.
func (fs *Filesystem) RunMutation(fn func() error) error {
	fs.mutateMu.Lock()
	defer fs.mutateMu.Unlock()

	snapshot := fs.dev.Snapshot()
	sbSnapshot := *fs.sb
	fs.mutating.Store(true)
	err := fn()
	fs.mutating.Store(false)
	if err != nil {
		if fs.metrics != nil {
			fs.metrics.MutationRollbacks.Add(1)
		}
		fs.EmitTypedEvent(EventMutationRollback, EventSourceFS, "", map[string]interface{}{"error": err.Error()})
		restoreErr := fs.dev.Reload()
		if restoreErr == nil {
			fs.sb = &sbSnapshot
			restoreErr = fs.reloadFromDevice()
		}
		if restoreErr != nil {
			return fmt.Errorf("mutation failed (%v); rollback failed: %w", err, restoreErr)
		}
		return err
	}

	if err := fs.inodeCache.FlushAll(); err != nil {
		if fs.metrics != nil {
			fs.metrics.CacheFlushErrors.Add(1)
		}
		_ = fs.dev.Reload()
		_ = fs.reloadFromDevice()
		return err
	}
	if err := fs.bufCache.FlushAll(); err != nil {
		if fs.metrics != nil {
			fs.metrics.CacheFlushErrors.Add(1)
		}
		_ = fs.dev.Reload()
		_ = fs.reloadFromDevice()
		return err
	}
	if err := WriteSuperblock(fs.dev, fs.sb); err != nil {
		_ = fs.dev.Reload()
		_ = fs.reloadFromDevice()
		return err
	}

	changed, err := fs.dev.ChangedBlocks(snapshot)
	if err != nil {
		return err
	}
	filtered := changed[:0]
	for _, addr := range changed {
		if addr >= JournalStart && addr < JournalStart+JournalBlocks {
			continue
		}
		filtered = append(filtered, addr)
	}
	changed = slices.Clip(filtered)
	if len(changed) == 0 {
		return fs.dev.Sync()
	}

	finalBlocks := make(map[uint32][]byte, len(changed))
	for _, addr := range changed {
		blk, readErr := fs.dev.ReadBlock(addr)
		if readErr != nil {
			return readErr
		}
		finalBlocks[addr] = blk
	}

	if err := fs.dev.RestoreBlocks(snapshot, changed); err != nil {
		return err
	}

	tx := fs.journal.OpenTransaction()
	for _, addr := range changed {
		if err := tx.LogBlock(addr, finalBlocks[addr]); err != nil {
			_ = fs.dev.Reload()
			_ = fs.reloadFromDevice()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		_ = fs.dev.Reload()
		_ = fs.reloadFromDevice()
		return err
	}
	return nil
}

// FlushInodeCache flushes a specific inode.
func (fs *Filesystem) FlushInodeCache(inum uint32) error {
	return fs.inodeCache.FlushInode(inum)
}

// NewFormat formats the device and mounts it.
func NewFormat(dev *disk.BlockDevice) (*Filesystem, error) {
	if _, err := Format(dev); err != nil {
		return nil, err
	}
	return Mount(dev)
}

// FsckResult holds the results of a filesystem check.
type FsckResult struct {
	Clean  bool     `json:"clean"`
	Issues []string `json:"issues"`
	Fixed  []string `json:"fixed"`
}

// FsckFull runs a comprehensive filesystem consistency check.
func (fs *Filesystem) FsckFull() *FsckResult {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	result := &FsckResult{Clean: true, Issues: []string{}, Fixed: []string{}}

	// Check 1: superblock magic.
	sb, err := ReadSuperblock(fs.dev)
	if err != nil {
		result.Clean = false
		result.Issues = append(result.Issues, "cannot read superblock: "+err.Error())
		return result
	}
	if err := sb.Validate(); err != nil {
		result.Clean = false
		result.Issues = append(result.Issues, "superblock invalid: "+err.Error())
	}

	// Check 2: root inode is a directory.
	root, err := ReadInode(fs.dev, 1)
	if err != nil {
		result.Clean = false
		result.Issues = append(result.Issues, "cannot read root inode")
		return result
	}
	if !root.IsDir() {
		result.Clean = false
		result.Issues = append(result.Issues, "inode 1 is not a directory")
	}

	// Check 3: root . and .. point to root.
	dotInum, _, err := LookupEntry(fs.dev, root, ".")
	if err != nil || dotInum != 1 {
		result.Clean = false
		result.Issues = append(result.Issues, "root . does not point to inode 1")
	}
	dotdotInum, _, err := LookupEntry(fs.dev, root, "..")
	if err != nil || dotdotInum != 1 {
		result.Clean = false
		result.Issues = append(result.Issues, "root .. does not point to inode 1")
	}

	// Check 4: superblock free block count matches bitmap.
	blockBM, err := ReadBlockBitmap(fs.dev)
	if err != nil {
		result.Clean = false
		result.Issues = append(result.Issues, "cannot read block bitmap")
	} else {
		expected := uint32(TotalBlocks) - uint32(blockBM.Count())
		if expected != sb.FreeBlocks {
			result.Clean = false
			result.Issues = append(result.Issues,
				fmt.Sprintf("superblock.FreeBlocks=%d but bitmap shows %d free", sb.FreeBlocks, expected))
		}
	}

	// Check 5: superblock free inode count matches bitmap.
	inodeBM, err := ReadInodeBitmap(fs.dev)
	if err != nil {
		result.Clean = false
		result.Issues = append(result.Issues, "cannot read inode bitmap")
	} else {
		expected := uint32(InodeCount) - uint32(inodeBM.Count())
		if expected != sb.FreeInodes {
			result.Clean = false
			result.Issues = append(result.Issues,
				fmt.Sprintf("superblock.FreeInodes=%d but bitmap shows %d free", sb.FreeInodes, expected))
		}
	}

	// Check 6: All allocated inodes have valid file type bits.
	// Note: inode numbers are 1-indexed; bitmap bits are 0-indexed (bit n-1 = inode n).
	if inodeBM != nil {
		for inum := uint32(1); inum <= InodeCount; inum++ {
			if !inodeBM.IsSet(inum - 1) {
				continue
			}
			di, err := ReadInode(fs.dev, inum)
			if err != nil {
				result.Clean = false
				result.Issues = append(result.Issues, fmt.Sprintf("allocated inode %d cannot be read: %v", inum, err))
				continue
			}
			ft := di.Mode & 0xF000
			if ft != 0x8000 && ft != 0x4000 && ft != 0xA000 {
				result.Clean = false
				result.Issues = append(result.Issues, fmt.Sprintf("inode %d: invalid file type bits 0x%04x", inum, ft))
			}
		}
	}

	// Check 7: All directory entries reference allocated inodes.
	if inodeBM != nil {
		for inum := uint32(1); inum <= InodeCount; inum++ {
			if !inodeBM.IsSet(inum - 1) {
				continue
			}
			di, err := ReadInode(fs.dev, inum)
			if err != nil || !di.IsDir() {
				continue
			}
			entries, err := ReadAllEntries(fs.dev, di)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.Name == "." || e.Name == ".." || e.Inode == 0 {
					continue
				}
				if !inodeBM.IsSet(e.Inode - 1) {
					result.Clean = false
					result.Issues = append(result.Issues,
						fmt.Sprintf("dir inode %d: entry %q references unallocated inode %d", inum, e.Name, e.Inode))
				}
			}
		}
	}

	// Check 8: All inode-referenced blocks are marked used; detect double-references.
	if blockBM != nil && inodeBM != nil {
		refBlocks := make(map[uint32]uint32)
		for inum := uint32(1); inum <= InodeCount; inum++ {
			if !inodeBM.IsSet(inum - 1) {
				continue
			}
			di, err := ReadInode(fs.dev, inum)
			if err != nil {
				continue
			}
			blks, err := BlockList(fs.dev, di)
			if err != nil {
				continue
			}
			for _, blk := range blks {
				if blk == 0 {
					continue
				}
				if !blockBM.IsSet(blk) {
					result.Clean = false
					result.Issues = append(result.Issues,
						fmt.Sprintf("inode %d references block %d not marked in block bitmap", inum, blk))
				}
				if prev, dup := refBlocks[blk]; dup {
					result.Clean = false
					result.Issues = append(result.Issues,
						fmt.Sprintf("block %d doubly referenced by inode %d and inode %d", blk, prev, inum))
				} else {
					refBlocks[blk] = inum
				}
			}
		}
	}

	// Check 9+10: BFS from root — detect circular directory refs and orphaned inodes.
	if inodeBM != nil {
		visited := make(map[uint32]bool)
		visited[1] = true
		queue := []uint32{1}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			di, err := ReadInode(fs.dev, cur)
			if err != nil || !di.IsDir() {
				continue
			}
			entries, err := ReadAllEntries(fs.dev, di)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.Name == "." || e.Name == ".." || e.Inode == 0 {
					continue
				}
				if visited[e.Inode] {
					// Check 9: circular reference
					child, cerr := ReadInode(fs.dev, e.Inode)
					if cerr == nil && child.IsDir() {
						result.Clean = false
						result.Issues = append(result.Issues,
							fmt.Sprintf("circular directory reference: inode %d (%q in dir %d) already visited", e.Inode, e.Name, cur))
					}
					continue
				}
				visited[e.Inode] = true
				queue = append(queue, e.Inode)
			}
		}
		// Check 10: orphaned inodes (allocated in bitmap but unreachable from root)
		for inum := uint32(1); inum <= InodeCount; inum++ {
			if inodeBM.IsSet(inum-1) && !visited[inum] {
				result.Clean = false
				result.Issues = append(result.Issues,
					fmt.Sprintf("inode %d is allocated but not reachable from root (orphan)", inum))
			}
		}
	}

	return result
}

// Reformat re-formats the filesystem in place (destructive).
func (fs *Filesystem) Reformat() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Stop the flusher.
	fs.flusher.Stop()

	// Re-format the device.
	if _, err := Format(fs.dev); err != nil {
		return fmt.Errorf("reformat: %w", err)
	}

	// Re-read superblock and re-initialize subsystems.
	sb, err := ReadSuperblock(fs.dev)
	if err != nil {
		return fmt.Errorf("reformat: read sb: %w", err)
	}
	blockBM, err := ReadBlockBitmap(fs.dev)
	if err != nil {
		return fmt.Errorf("reformat: read block bm: %w", err)
	}
	inodeBM, err := ReadInodeBitmap(fs.dev)
	if err != nil {
		return fmt.Errorf("reformat: read inode bm: %w", err)
	}

	fs.sb = sb
	fs.bufCache.Invalidate(SuperblockAddr)
	fs.alloc = NewAllocator(fs.dev, sb, blockBM, inodeBM, fs.metrics)
	fs.journal = journal.NewJournal(fs.dev, JournalStart, JournalBlocks, fs.metrics)
	fs.alloc.SetEventHook(func(eventType string, fields map[string]interface{}) {
		fs.EmitTypedEvent(eventType, EventSourceAllocator, "", fields)
	})
	fs.journal.SetEventHook(func(eventType string, fields map[string]interface{}) {
		fs.EmitTypedEvent(eventType, EventSourceJournal, "", fields)
	})
	_ = fs.inodeCache.FlushAll()
	fs.closed = false

	// Restart flusher.
	fs.flusher = cache.NewWritebackFlusher(fs.bufCache, 5*time.Second)
	fs.flusher.Start()

	return nil
}

// SimulateCrash simulates a crash: marks filesystem dirty, stops flushing,
// then recovers via journal replay.
func (fs *Filesystem) SimulateCrash() error {
	if fs.metrics != nil {
		fs.metrics.RecoveryRuns.Add(1)
	}
	fs.mu.Lock()
	fs.flusher.Stop()
	fs.mu.Unlock()

	// Drop any in-memory state that wasn't durably synced.
	if err := fs.dev.Reload(); err != nil {
		return fmt.Errorf("crash reload: %w", err)
	}

	// Replay journal to recover.
	if err := journal.RecoverWithHook(fs.dev, JournalStart, JournalBlocks, func(eventType string, fields map[string]interface{}) {
		if fs.metrics != nil && eventType == EventJournalRecover {
			if count, ok := fields["blockCount"].(int); ok {
				fs.metrics.RecoveryBlocks.Add(int64(count))
			}
		}
		fs.EmitTypedEvent(eventType, EventSourceJournal, "", fields)
	}); err != nil {
		if fs.metrics != nil {
			fs.metrics.RecoveryErrors.Add(1)
		}
		return fmt.Errorf("crash recovery: %w", err)
	}
	if err := fs.dev.Sync(); err != nil {
		return fmt.Errorf("crash recovery sync: %w", err)
	}

	fs.mu.Lock()
	if err := fs.reloadFromDevice(); err != nil {
		fs.mu.Unlock()
		return fmt.Errorf("crash reload state: %w", err)
	}
	fs.sb.State = StateClean
	if err := WriteSuperblock(fs.dev, fs.sb); err != nil {
		fs.mu.Unlock()
		return err
	}
	if err := fs.dev.Sync(); err != nil {
		fs.mu.Unlock()
		return err
	}

	// Restart flusher.
	fs.flusher = cache.NewWritebackFlusher(fs.bufCache, 5*time.Second)
	fs.flusher.Start()
	fs.mu.Unlock()

	return nil
}
