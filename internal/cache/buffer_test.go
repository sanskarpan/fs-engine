package cache

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/fs-engine/internal/disk"
)

const blockSz = 4096

func newTestCache(t *testing.T, capacity int) (*BufferCache, *disk.BlockDevice) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cache_test.img")
	dev, err := disk.NewBlockDevice(path, 64*blockSz)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dev.Close() })
	return NewBufferCache(dev, capacity), dev
}

func TestBufferCache_GetBlock(t *testing.T) {
	c, dev := newTestCache(t, 16)

	data := make([]byte, blockSz)
	data[0] = 0xAB
	require.NoError(t, dev.WriteBlock(5, data))

	got, err := c.GetBlock(5)
	require.NoError(t, err)
	assert.Equal(t, byte(0xAB), got[0])
}

func TestBufferCache_WriteBlock_MarkDirty_FlushBlock(t *testing.T) {
	c, dev := newTestCache(t, 16)

	data := make([]byte, blockSz)
	data[10] = 0xFF
	require.NoError(t, c.WriteBlock(3, data))

	// Verify it's in cache (hit on second read)
	stats1 := c.Stats()
	got, err := c.GetBlock(3)
	require.NoError(t, err)
	stats2 := c.Stats()
	assert.Equal(t, byte(0xFF), got[10])
	assert.Greater(t, stats2.Hits, stats1.Hits)

	// Flush and read from device directly
	require.NoError(t, c.FlushBlock(3))
	blk, err := dev.ReadBlock(3)
	require.NoError(t, err)
	assert.Equal(t, byte(0xFF), blk[10])
}

func TestBufferCache_Invalidate(t *testing.T) {
	c, _ := newTestCache(t, 16)
	data := make([]byte, blockSz)
	require.NoError(t, c.WriteBlock(0, data))

	stats1 := c.Stats()
	c.Invalidate(0)
	stats2 := c.Stats()
	assert.Less(t, stats2.Size, stats1.Size)
}

func TestBufferCache_Eviction(t *testing.T) {
	c, _ := newTestCache(t, 4)
	data := make([]byte, blockSz)

	// Fill cache
	for i := 0; i < 6; i++ {
		require.NoError(t, c.WriteBlock(uint32(i), data))
	}
	// Cache should not grow beyond capacity
	assert.LessOrEqual(t, c.Stats().Size, 4)
}

func TestBufferCache_ConcurrentAccess(t *testing.T) {
	c, _ := newTestCache(t, 32)
	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			data := make([]byte, blockSz)
			data[0] = byte(n)
			_ = c.WriteBlock(uint32(n%8), data)
		}(i)
		go func(n int) {
			defer wg.Done()
			_, _ = c.GetBlock(uint32(n % 8))
		}(i)
	}
	wg.Wait()
}

func TestBufferCache_FlushAll(t *testing.T) {
	c, dev := newTestCache(t, 16)

	for i := 0; i < 5; i++ {
		data := make([]byte, blockSz)
		data[0] = byte(i + 1)
		require.NoError(t, c.WriteBlock(uint32(i), data))
	}
	require.NoError(t, c.FlushAll())

	// Verify all blocks are on disk
	for i := 0; i < 5; i++ {
		blk, err := dev.ReadBlock(uint32(i))
		require.NoError(t, err)
		assert.Equal(t, byte(i+1), blk[0])
	}
}
