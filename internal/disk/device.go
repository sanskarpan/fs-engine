package disk

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

const blockSize = 4096

// BlockDevice is an in-memory block device backed by a file.
type BlockDevice struct {
	mu    sync.RWMutex
	data  []byte
	path  string
	size  int64
	dirty atomic.Bool

	faultMu sync.RWMutex
	fault   func(Operation) error
}

// Operation describes a block device action for deterministic fault injection.
type Operation struct {
	Kind string
	Addr uint32
}

// NewBlockDevice opens or creates a block device backed by the file at path.
// If the file exists and is the right size, it is read into memory.
// Otherwise a new zeroed device of the given size is created.
func NewBlockDevice(path string, size int64) (*BlockDevice, error) {
	if size <= 0 || size%blockSize != 0 {
		return nil, fmt.Errorf("disk: size must be a positive multiple of %d", blockSize)
	}

	d := &BlockDevice{
		path: path,
		size: size,
		data: make([]byte, size),
	}

	info, err := os.Stat(path)
	if err == nil && info.Size() == size {
		// Read existing file
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("disk: open %s: %w", path, err)
		}
		defer f.Close()
		if _, err := f.Read(d.data); err != nil {
			return nil, fmt.Errorf("disk: read %s: %w", path, err)
		}
	} else {
		// Create new zeroed file
		f, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("disk: create %s: %w", path, err)
		}
		defer f.Close()
		if _, err := f.Write(d.data); err != nil {
			return nil, fmt.Errorf("disk: write %s: %w", path, err)
		}
	}

	return d, nil
}

// BlockCount returns the total number of blocks.
func (d *BlockDevice) BlockCount() uint32 {
	return uint32(d.size / blockSize)
}

// ReadBlock returns a copy of block addr.
func (d *BlockDevice) ReadBlock(addr uint32) ([]byte, error) {
	if err := d.checkFault(Operation{Kind: "read", Addr: addr}); err != nil {
		return nil, err
	}
	if addr >= uint32(d.size/blockSize) {
		return nil, fmt.Errorf("disk: block %d out of range (max %d)", addr, d.size/blockSize-1)
	}
	offset := int64(addr) * blockSize
	out := make([]byte, blockSize)
	d.mu.RLock()
	copy(out, d.data[offset:offset+blockSize])
	d.mu.RUnlock()
	return out, nil
}

// WriteBlock writes data to block addr. data must be exactly blockSize bytes.
func (d *BlockDevice) WriteBlock(addr uint32, data []byte) error {
	if err := d.checkFault(Operation{Kind: "write", Addr: addr}); err != nil {
		return err
	}
	if addr >= uint32(d.size/blockSize) {
		return fmt.Errorf("disk: block %d out of range (max %d)", addr, d.size/blockSize-1)
	}
	if len(data) != blockSize {
		return fmt.Errorf("disk: WriteBlock requires %d bytes, got %d", blockSize, len(data))
	}
	offset := int64(addr) * blockSize
	d.mu.Lock()
	copy(d.data[offset:offset+blockSize], data)
	d.mu.Unlock()
	d.dirty.Store(true)
	return nil
}

// Sync flushes all dirty blocks to the backing file.
func (d *BlockDevice) Sync() error {
	if err := d.checkFault(Operation{Kind: "sync"}); err != nil {
		return err
	}
	if !d.dirty.Load() {
		return nil
	}
	f, err := os.OpenFile(d.path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("disk: sync open %s: %w", d.path, err)
	}
	defer f.Close()

	d.mu.RLock()
	_, err = f.WriteAt(d.data, 0)
	d.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("disk: sync write %s: %w", d.path, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("disk: fsync %s: %w", d.path, err)
	}
	d.dirty.Store(false)
	return nil
}

// Close syncs the device and releases resources.
func (d *BlockDevice) Close() error {
	return d.Sync()
}

// SetFaultInjector registers a deterministic failure hook for device operations.
func (d *BlockDevice) SetFaultInjector(injector func(Operation) error) {
	d.faultMu.Lock()
	d.fault = injector
	d.faultMu.Unlock()
}

// ClearFaultInjector removes any deterministic failure hook.
func (d *BlockDevice) ClearFaultInjector() {
	d.SetFaultInjector(nil)
}

// Size returns the total size in bytes.
func (d *BlockDevice) Size() int64 {
	return d.size
}

// Snapshot returns a full copy of the in-memory device state.
func (d *BlockDevice) Snapshot() []byte {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]byte, len(d.data))
	copy(out, d.data)
	return out
}

// ChangedBlocks returns the block addresses that differ from snapshot.
func (d *BlockDevice) ChangedBlocks(snapshot []byte) ([]uint32, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if len(snapshot) != len(d.data) {
		return nil, fmt.Errorf("disk: snapshot size mismatch: got %d want %d", len(snapshot), len(d.data))
	}

	count := len(d.data) / blockSize
	changed := make([]uint32, 0)
	for i := 0; i < count; i++ {
		start := i * blockSize
		end := start + blockSize
		if !equalBlock(snapshot[start:end], d.data[start:end]) {
			changed = append(changed, uint32(i))
		}
	}
	return changed, nil
}

// RestoreBlocks restores the specified blocks from snapshot into memory.
func (d *BlockDevice) RestoreBlocks(snapshot []byte, addrs []uint32) error {
	if len(snapshot) != len(d.data) {
		return fmt.Errorf("disk: snapshot size mismatch: got %d want %d", len(snapshot), len(d.data))
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	for _, addr := range addrs {
		if addr >= uint32(d.size/blockSize) {
			return fmt.Errorf("disk: block %d out of range (max %d)", addr, d.size/blockSize-1)
		}
		start := int(addr) * blockSize
		end := start + blockSize
		copy(d.data[start:end], snapshot[start:end])
	}
	d.dirty.Store(true)
	return nil
}

// Reload discards in-memory state and reloads the device from its backing file.
func (d *BlockDevice) Reload() error {
	f, err := os.Open(d.path)
	if err != nil {
		return fmt.Errorf("disk: reload open %s: %w", d.path, err)
	}
	defer f.Close()

	buf := make([]byte, d.size)
	if _, err := f.Read(buf); err != nil {
		return fmt.Errorf("disk: reload read %s: %w", d.path, err)
	}

	d.mu.Lock()
	copy(d.data, buf)
	d.mu.Unlock()
	d.dirty.Store(false)
	return nil
}

func equalBlock(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (d *BlockDevice) checkFault(op Operation) error {
	d.faultMu.RLock()
	fault := d.fault
	d.faultMu.RUnlock()
	if fault == nil {
		return nil
	}
	return fault(op)
}
