package fs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetBlockForOffset_Direct(t *testing.T) {
	dev, sb := newFormattedDev(t)

	blockBM, _ := ReadBlockBitmap(dev)
	inodeBM, _ := ReadInodeBitmap(dev)
	alloc := NewAllocator(dev, sb, blockBM, inodeBM, nil)

	di := &DiskInode{Mode: S_IFREG | 0644}

	// Allocate first block
	phys, err := AllocBlockForOffset(dev, alloc, di, 0)
	require.NoError(t, err)
	assert.NotEqual(t, uint32(0), phys)

	// Read it back
	got, err := GetBlockForOffset(dev, di, 0)
	require.NoError(t, err)
	assert.Equal(t, phys, got)
}

func TestGetBlockForOffset_Hole(t *testing.T) {
	dev, _ := newFormattedDev(t)
	di := &DiskInode{Mode: S_IFREG | 0644}

	// No blocks allocated - should return 0 (hole)
	got, err := GetBlockForOffset(dev, di, 5)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), got)
}

func TestAllocBlockForOffset_Indirect1(t *testing.T) {
	dev, sb := newFormattedDev(t)

	blockBM, _ := ReadBlockBitmap(dev)
	inodeBM, _ := ReadInodeBitmap(dev)
	alloc := NewAllocator(dev, sb, blockBM, inodeBM, nil)

	di := &DiskInode{Mode: S_IFREG | 0644}

	// Block 12 is the first single-indirect block
	phys, err := AllocBlockForOffset(dev, alloc, di, 12)
	require.NoError(t, err)
	assert.NotEqual(t, uint32(0), phys)
	assert.NotEqual(t, uint32(0), di.Indirect1)

	// Verify we can read it back
	got, err := GetBlockForOffset(dev, di, 12)
	require.NoError(t, err)
	assert.Equal(t, phys, got)
}

func TestBlockList(t *testing.T) {
	dev, sb := newFormattedDev(t)

	blockBM, _ := ReadBlockBitmap(dev)
	inodeBM, _ := ReadInodeBitmap(dev)
	alloc := NewAllocator(dev, sb, blockBM, inodeBM, nil)

	di := &DiskInode{Mode: S_IFREG | 0644}
	di.SetSize(3 * BlockSize)

	for i := uint32(0); i < 3; i++ {
		_, err := AllocBlockForOffset(dev, alloc, di, i)
		require.NoError(t, err)
	}

	list, err := BlockList(dev, di)
	require.NoError(t, err)
	assert.Len(t, list, 3)
	for _, phys := range list {
		assert.NotEqual(t, uint32(0), phys)
	}
}

func TestFreeBlocksFrom(t *testing.T) {
	dev, sb := newFormattedDev(t)

	blockBM, _ := ReadBlockBitmap(dev)
	inodeBM, _ := ReadInodeBitmap(dev)
	alloc := NewAllocator(dev, sb, blockBM, inodeBM, nil)

	di := &DiskInode{Mode: S_IFREG | 0644}
	di.SetSize(5 * BlockSize)

	for i := uint32(0); i < 5; i++ {
		_, err := AllocBlockForOffset(dev, alloc, di, i)
		require.NoError(t, err)
	}

	freeCount := sb.FreeBlocks
	require.NoError(t, FreeBlocksFrom(dev, alloc, di, 3))

	// Blocks 3 and 4 should now be freed
	assert.Greater(t, sb.FreeBlocks, freeCount)
}
