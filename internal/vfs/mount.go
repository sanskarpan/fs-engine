package vfs

import (
	"fmt"
	"sync"
)

// MountEntry describes a mounted filesystem.
type MountEntry struct {
	Mountpoint string
	FSType     string
	Device     string
	ReadOnly   bool
}

// MountTable tracks all mounted filesystems.
type MountTable struct {
	mu      sync.RWMutex
	mounts  map[string]MountEntry
}

// NewMountTable creates a new MountTable.
func NewMountTable() *MountTable {
	return &MountTable{
		mounts: make(map[string]MountEntry),
	}
}

// Add registers a mount point.
func (mt *MountTable) Add(entry MountEntry) error {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	if _, exists := mt.mounts[entry.Mountpoint]; exists {
		return fmt.Errorf("vfs: %s is already mounted", entry.Mountpoint)
	}
	mt.mounts[entry.Mountpoint] = entry
	return nil
}

// Remove unregisters a mount point.
func (mt *MountTable) Remove(mountpoint string) error {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	if _, exists := mt.mounts[mountpoint]; !exists {
		return fmt.Errorf("vfs: %s is not mounted", mountpoint)
	}
	delete(mt.mounts, mountpoint)
	return nil
}

// Get returns the mount entry for a mountpoint.
func (mt *MountTable) Get(mountpoint string) (MountEntry, bool) {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	e, ok := mt.mounts[mountpoint]
	return e, ok
}

// List returns all mount entries.
func (mt *MountTable) List() []MountEntry {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	result := make([]MountEntry, 0, len(mt.mounts))
	for _, e := range mt.mounts {
		result = append(result, e)
	}
	return result
}
