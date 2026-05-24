package fs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirEntry_ParseEncode(t *testing.T) {
	de := DirEntry{
		Inode:    42,
		RecLen:   16,
		NameLen:  5,
		FileType: FT_REG_FILE,
		Name:     "hello",
	}

	buf := make([]byte, 16)
	encodeDirEntry(de, buf)

	got, consumed, err := ParseDirEntry(buf)
	require.NoError(t, err)
	assert.Equal(t, 16, consumed)
	assert.Equal(t, uint32(42), got.Inode)
	assert.Equal(t, "hello", got.Name)
	assert.Equal(t, uint8(FT_REG_FILE), got.FileType)
}

func TestAlign4(t *testing.T) {
	assert.Equal(t, 0, align4(0))
	assert.Equal(t, 4, align4(1))
	assert.Equal(t, 4, align4(4))
	assert.Equal(t, 8, align4(5))
	assert.Equal(t, 8, align4(8))
}

func TestDirEntry_AddLookupRemove(t *testing.T) {
	dev, sb := newFormattedDev(t)

	blockBM, err := ReadBlockBitmap(dev)
	require.NoError(t, err)
	inodeBM, err := ReadInodeBitmap(dev)
	require.NoError(t, err)
	alloc := NewAllocator(dev, sb, blockBM, inodeBM, nil)

	// Create a directory inode
	blkAddr, err := alloc.AllocBlock()
	require.NoError(t, err)

	di := &DiskInode{
		Mode:      S_IFDIR | 0755,
		Links:     2,
		Blocks512: BlockSize / 512,
	}
	di.Direct[0] = blkAddr
	di.SetSize(BlockSize)

	// Initialize with "." and ".."
	require.NoError(t, InitDirBlock(dev, di, 10, 1))

	// Add an entry
	require.NoError(t, AddEntry(dev, alloc, di, "testfile", 100, FT_REG_FILE))

	// Lookup
	inum, ft, err := LookupEntry(dev, di, "testfile")
	require.NoError(t, err)
	assert.Equal(t, uint32(100), inum)
	assert.Equal(t, uint8(FT_REG_FILE), ft)

	// Not found
	_, _, err = LookupEntry(dev, di, "nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)

	// Remove
	require.NoError(t, RemoveEntry(dev, di, "testfile"))

	// Should not be found anymore
	_, _, err = LookupEntry(dev, di, "testfile")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDirEntry_IsEmptyDir(t *testing.T) {
	dev, sb := newFormattedDev(t)

	blockBM, _ := ReadBlockBitmap(dev)
	inodeBM, _ := ReadInodeBitmap(dev)
	alloc := NewAllocator(dev, sb, blockBM, inodeBM, nil)

	blkAddr, err := alloc.AllocBlock()
	require.NoError(t, err)

	di := &DiskInode{
		Mode:      S_IFDIR | 0755,
		Links:     2,
		Blocks512: BlockSize / 512,
	}
	di.Direct[0] = blkAddr
	di.SetSize(BlockSize)
	require.NoError(t, InitDirBlock(dev, di, 5, 1))

	empty, err := IsEmptyDir(dev, di)
	require.NoError(t, err)
	assert.True(t, empty)

	require.NoError(t, AddEntry(dev, alloc, di, "child", 42, FT_REG_FILE))

	empty, err = IsEmptyDir(dev, di)
	require.NoError(t, err)
	assert.False(t, empty)
}

func TestReadAllEntries(t *testing.T) {
	dev, _ := newFormattedDev(t)

	root, err := ReadInode(dev, 1)
	require.NoError(t, err)

	entries, err := ReadAllEntries(dev, root)
	require.NoError(t, err)

	assert.Greater(t, len(entries), 0)
	hasDot := false
	hasDotDot := false
	for _, e := range entries {
		if e.Name == "." {
			hasDot = true
		}
		if e.Name == ".." {
			hasDotDot = true
		}
	}
	assert.True(t, hasDot)
	assert.True(t, hasDotDot)
}
