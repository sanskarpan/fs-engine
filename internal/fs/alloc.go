package fs

import (
	"fmt"
	"sync"

	"github.com/yourname/fs-engine/internal/disk"
	"github.com/yourname/fs-engine/internal/metrics"
)

// Allocator manages block and inode allocation using in-memory bitmaps.
type Allocator struct {
	mu      sync.Mutex
	dev     *disk.BlockDevice
	blockBM *Bitmap
	inodeBM *Bitmap
	sb      *DiskSuperblock
	metrics *metrics.Metrics
	emit    func(eventType string, fields map[string]interface{})
}

// NewAllocator creates an Allocator that manages the given superblock and bitmaps.
func NewAllocator(dev *disk.BlockDevice, sb *DiskSuperblock, blockBM, inodeBM *Bitmap, m *metrics.Metrics) *Allocator {
	return &Allocator{
		dev:     dev,
		blockBM: blockBM,
		inodeBM: inodeBM,
		sb:      sb,
		metrics: m,
	}
}

// SetEventHook registers a callback for allocator lifecycle events.
func (a *Allocator) SetEventHook(hook func(eventType string, fields map[string]interface{})) {
	a.mu.Lock()
	a.emit = hook
	a.mu.Unlock()
}

// AllocBlock allocates a free data block and returns its address.
func (a *Allocator) AllocBlock() (uint32, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Search from DataStart
	idx, ok := a.blockBM.FindNext(DataStart)
	if !ok {
		return 0, ErrNoSpace
	}
	a.blockBM.Set(idx)
	if a.sb.FreeBlocks > 0 {
		a.sb.FreeBlocks--
	}

	// Write bitmap to disk
	if err := WriteBlockBitmap(a.dev, a.blockBM); err != nil {
		a.blockBM.Clear(idx)
		a.sb.FreeBlocks++
		return 0, fmt.Errorf("alloc: write block bitmap: %w", err)
	}

	// Zero the newly allocated block
	zero := make([]byte, BlockSize)
	if err := a.dev.WriteBlock(idx, zero); err != nil {
		return 0, fmt.Errorf("alloc: zero block %d: %w", idx, err)
	}

	if a.metrics != nil {
		a.metrics.BlocksAllocated.Add(1)
	}
	if a.emit != nil {
		a.emit(EventBlockAlloc, map[string]interface{}{"blockAddr": idx})
	}
	return idx, nil
}

// FreeBlock marks a block as free.
func (a *Allocator) FreeBlock(addr uint32) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if addr < DataStart || addr >= TotalBlocks {
		return fmt.Errorf("alloc: free invalid block %d", addr)
	}
	if !a.blockBM.IsSet(addr) {
		return nil // already free
	}
	a.blockBM.Clear(addr)
	a.sb.FreeBlocks++

	if err := WriteBlockBitmap(a.dev, a.blockBM); err != nil {
		return err
	}
	if a.metrics != nil {
		a.metrics.BlocksFreed.Add(1)
	}
	if a.emit != nil {
		a.emit(EventBlockFree, map[string]interface{}{"blockAddr": addr})
	}
	return nil
}

// AllocInode allocates a free inode (1-indexed) and returns its number.
func (a *Allocator) AllocInode() (uint32, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Inode 0 is reserved; start searching from 1 (index 0 in bitmap)
	idx, ok := a.inodeBM.FindNext(0)
	if !ok {
		return 0, ErrNoSpace
	}
	a.inodeBM.Set(idx)
	if a.sb.FreeInodes > 0 {
		a.sb.FreeInodes--
	}

	if err := WriteInodeBitmap(a.dev, a.inodeBM); err != nil {
		a.inodeBM.Clear(idx)
		a.sb.FreeInodes++
		return 0, fmt.Errorf("alloc: write inode bitmap: %w", err)
	}

	if a.metrics != nil {
		a.metrics.InodesAllocated.Add(1)
	}
	if a.emit != nil {
		a.emit(EventInodeAlloc, map[string]interface{}{"inodeNum": idx + 1})
	}
	// Inode numbers are 1-indexed
	return idx + 1, nil
}

// FreeInode marks an inode as free and zeros its on-disk structure.
func (a *Allocator) FreeInode(inum uint32) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if inum == 0 || inum > InodeCount {
		return fmt.Errorf("alloc: free invalid inode %d", inum)
	}

	idx := inum - 1
	if !a.inodeBM.IsSet(idx) {
		return nil // already free
	}
	a.inodeBM.Clear(idx)
	a.sb.FreeInodes++

	if err := WriteInodeBitmap(a.dev, a.inodeBM); err != nil {
		return fmt.Errorf("alloc: write inode bitmap: %w", err)
	}

	if a.metrics != nil {
		a.metrics.InodesFreed.Add(1)
	}
	if a.emit != nil {
		a.emit(EventInodeFree, map[string]interface{}{"inodeNum": inum})
	}
	// Zero the inode on disk
	empty := &DiskInode{}
	return WriteInode(a.dev, inum, empty)
}

// FreeInodeBlocks frees all data blocks referenced by the inode
// (direct, indirect1, indirect2, indirect3).
func (a *Allocator) FreeInodeBlocks(di *DiskInode) error {
	// Free direct blocks
	for _, addr := range di.Direct {
		if addr != 0 {
			if err := a.FreeBlock(addr); err != nil {
				return err
			}
		}
	}

	// Free indirect1
	if di.Indirect1 != 0 {
		if err := a.freeIndirect1(di.Indirect1); err != nil {
			return err
		}
	}

	// Free indirect2
	if di.Indirect2 != 0 {
		if err := a.freeIndirect2(di.Indirect2); err != nil {
			return err
		}
	}

	// Free indirect3
	if di.Indirect3 != 0 {
		if err := a.freeIndirect3(di.Indirect3); err != nil {
			return err
		}
	}

	return nil
}

func (a *Allocator) freeIndirect1(blockAddr uint32) error {
	blk, err := a.dev.ReadBlock(blockAddr)
	if err != nil {
		return err
	}
	ptrs := decodePointers(blk)
	for _, ptr := range ptrs {
		if ptr != 0 {
			if err := a.FreeBlock(ptr); err != nil {
				return err
			}
		}
	}
	return a.FreeBlock(blockAddr)
}

func (a *Allocator) freeIndirect2(blockAddr uint32) error {
	blk, err := a.dev.ReadBlock(blockAddr)
	if err != nil {
		return err
	}
	ptrs := decodePointers(blk)
	for _, ptr := range ptrs {
		if ptr != 0 {
			if err := a.freeIndirect1(ptr); err != nil {
				return err
			}
		}
	}
	return a.FreeBlock(blockAddr)
}

func (a *Allocator) freeIndirect3(blockAddr uint32) error {
	blk, err := a.dev.ReadBlock(blockAddr)
	if err != nil {
		return err
	}
	ptrs := decodePointers(blk)
	for _, ptr := range ptrs {
		if ptr != 0 {
			if err := a.freeIndirect2(ptr); err != nil {
				return err
			}
		}
	}
	return a.FreeBlock(blockAddr)
}

// decodePointers reads PointersPerBlock uint32 values from a block.
func decodePointers(blk []byte) []uint32 {
	ptrs := make([]uint32, PointersPerBlock)
	for i := range ptrs {
		ptrs[i] = uint32(blk[i*4]) |
			uint32(blk[i*4+1])<<8 |
			uint32(blk[i*4+2])<<16 |
			uint32(blk[i*4+3])<<24
	}
	return ptrs
}

// encodePointer writes a uint32 in little-endian to buf at offset.
func encodePointer(buf []byte, offset int, val uint32) {
	buf[offset] = byte(val)
	buf[offset+1] = byte(val >> 8)
	buf[offset+2] = byte(val >> 16)
	buf[offset+3] = byte(val >> 24)
}
