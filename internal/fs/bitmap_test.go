package fs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBitmap_SetClearIsSet(t *testing.T) {
	bm := NewBitmap(100)
	assert.False(t, bm.IsSet(0))
	assert.False(t, bm.IsSet(99))

	bm.Set(0)
	assert.True(t, bm.IsSet(0))
	assert.False(t, bm.IsSet(1))

	bm.Set(99)
	assert.True(t, bm.IsSet(99))

	bm.Clear(0)
	assert.False(t, bm.IsSet(0))
	assert.True(t, bm.IsSet(99))
}

func TestBitmap_FindFirst(t *testing.T) {
	bm := NewBitmap(64)
	// All clear - should find 0
	idx, ok := bm.FindFirst()
	assert.True(t, ok)
	assert.Equal(t, uint32(0), idx)

	bm.Set(0)
	idx, ok = bm.FindFirst()
	assert.True(t, ok)
	assert.Equal(t, uint32(1), idx)

	// Fill all bits
	for i := uint32(0); i < 64; i++ {
		bm.Set(i)
	}
	_, ok = bm.FindFirst()
	assert.False(t, ok)
}

func TestBitmap_FindNext(t *testing.T) {
	bm := NewBitmap(16)
	bm.Set(0)
	bm.Set(1)
	bm.Set(2)

	idx, ok := bm.FindNext(0)
	assert.True(t, ok)
	assert.Equal(t, uint32(3), idx)

	idx, ok = bm.FindNext(5)
	assert.True(t, ok)
	assert.Equal(t, uint32(5), idx)
}

func TestBitmap_Count(t *testing.T) {
	bm := NewBitmap(32)
	assert.Equal(t, uint32(0), bm.Count())

	bm.Set(0)
	bm.Set(15)
	bm.Set(31)
	assert.Equal(t, uint32(3), bm.Count())

	bm.Clear(15)
	assert.Equal(t, uint32(2), bm.Count())
}

func TestReadWriteBlockBitmap(t *testing.T) {
	dev := newTestDev(t)

	bm := NewBitmap(TotalBlocks)
	bm.Set(0)
	bm.Set(100)
	bm.Set(TotalBlocks - 1)

	require.NoError(t, WriteBlockBitmap(dev, bm))

	got, err := ReadBlockBitmap(dev)
	require.NoError(t, err)

	assert.True(t, got.IsSet(0))
	assert.True(t, got.IsSet(100))
	assert.True(t, got.IsSet(TotalBlocks-1))
	assert.False(t, got.IsSet(50))
}

func TestReadWriteInodeBitmap(t *testing.T) {
	dev := newTestDev(t)

	bm := NewBitmap(InodeCount)
	bm.Set(0) // inode 1
	bm.Set(5)

	require.NoError(t, WriteInodeBitmap(dev, bm))

	got, err := ReadInodeBitmap(dev)
	require.NoError(t, err)

	assert.True(t, got.IsSet(0))
	assert.True(t, got.IsSet(5))
	assert.False(t, got.IsSet(2))
}
