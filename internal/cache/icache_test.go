package cache

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInodeCache_PutGet(t *testing.T) {
	var flushed []uint32
	var mu sync.Mutex
	ic := NewInodeCache(16, func(inum uint32, data InodeData) error {
		mu.Lock()
		flushed = append(flushed, inum)
		mu.Unlock()
		return nil
	})

	var d InodeData
	d[0] = 0xAB
	d[10] = 0xCD

	ic.Put(1, d)

	got, ok := ic.Get(1)
	require.True(t, ok)
	assert.Equal(t, byte(0xAB), got[0])
	assert.Equal(t, byte(0xCD), got[10])
}

func TestInodeCache_Update_MarksDirty(t *testing.T) {
	flushed := make(map[uint32]InodeData)
	var mu sync.Mutex
	ic := NewInodeCache(16, func(inum uint32, data InodeData) error {
		mu.Lock()
		flushed[inum] = data
		mu.Unlock()
		return nil
	})

	var d InodeData
	d[0] = 0x42
	ic.Update(5, d)

	require.NoError(t, ic.FlushInode(5))

	mu.Lock()
	got, ok := flushed[5]
	mu.Unlock()
	require.True(t, ok)
	assert.Equal(t, byte(0x42), got[0])
}

func TestInodeCache_Invalidate(t *testing.T) {
	ic := NewInodeCache(16, nil)
	var d InodeData
	ic.Put(3, d)

	_, ok := ic.Get(3)
	assert.True(t, ok)

	ic.Invalidate(3)
	_, ok = ic.Get(3)
	assert.False(t, ok)
}

func TestInodeCache_FlushAll(t *testing.T) {
	var count int
	var mu sync.Mutex
	ic := NewInodeCache(16, func(inum uint32, data InodeData) error {
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	})

	for i := uint32(1); i <= 5; i++ {
		var d InodeData
		d[0] = byte(i)
		ic.Update(i, d)
	}

	require.NoError(t, ic.FlushAll())
	mu.Lock()
	assert.Equal(t, 5, count)
	mu.Unlock()
}

func TestInodeCache_Eviction(t *testing.T) {
	ic := NewInodeCache(3, nil)

	for i := uint32(1); i <= 5; i++ {
		var d InodeData
		ic.Put(i, d)
	}
	// Size should not exceed capacity
	// We can't directly observe size but can verify new puts work
	var d InodeData
	ic.Put(10, d)
	_, ok := ic.Get(10)
	assert.True(t, ok)
}
