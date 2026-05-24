package fs

import (
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInodeSize(t *testing.T) {
	assert.Equal(t, uintptr(128), unsafe.Sizeof(DiskInode{}), "DiskInode must be exactly 128 bytes")
}

func TestInodeReadWrite(t *testing.T) {
	dev := newTestDev(t)

	// Zero inode table first
	zero := make([]byte, BlockSize)
	for i := uint32(0); i < InodeTableBlocks; i++ {
		require.NoError(t, dev.WriteBlock(InodeTableStart+i, zero))
	}

	di := &DiskInode{
		Mode:      S_IFREG | 0644,
		UID:       1000,
		GID:       1000,
		Links:     1,
		Blocks512: 8,
		ATime:     uint32(time.Now().Unix()),
		MTime:     uint32(time.Now().Unix()),
	}
	di.SetSize(12345)
	di.Direct[0] = 500
	di.Direct[11] = 999

	require.NoError(t, WriteInode(dev, 1, di))

	got, err := ReadInode(dev, 1)
	require.NoError(t, err)

	assert.Equal(t, di.Mode, got.Mode)
	assert.Equal(t, di.UID, got.UID)
	assert.Equal(t, di.GID, got.GID)
	assert.Equal(t, di.Links, got.Links)
	assert.Equal(t, int64(12345), got.Size())
	assert.Equal(t, uint32(500), got.Direct[0])
	assert.Equal(t, uint32(999), got.Direct[11])
}

func TestInodeNumToBlock(t *testing.T) {
	// Inode 1: first inode, should be at InodeTableStart+0, offset 0
	addr, off := InodeNumToBlock(1)
	assert.Equal(t, uint32(InodeTableStart), addr)
	assert.Equal(t, 0, off)

	// Inode 33: 32nd inode (0-indexed), should be at InodeTableStart, offset 32*128=4096 which is block 1
	addr, off = InodeNumToBlock(33)
	assert.Equal(t, uint32(InodeTableStart+1), addr)
	assert.Equal(t, 0, off)
}

func TestInode_FileType(t *testing.T) {
	dir := &DiskInode{Mode: S_IFDIR | 0755}
	assert.True(t, dir.IsDir())
	assert.False(t, dir.IsRegular())
	assert.False(t, dir.IsSymlink())

	reg := &DiskInode{Mode: S_IFREG | 0644}
	assert.False(t, reg.IsDir())
	assert.True(t, reg.IsRegular())

	sym := &DiskInode{Mode: S_IFLNK | 0777}
	assert.True(t, sym.IsSymlink())
}

func TestInode_ModeString(t *testing.T) {
	di := &DiskInode{Mode: S_IFDIR | 0755}
	assert.Equal(t, "drwxr-xr-x", di.ModeString())

	di2 := &DiskInode{Mode: S_IFREG | 0644}
	assert.Equal(t, "-rw-r--r--", di2.ModeString())

	di3 := &DiskInode{Mode: S_IFLNK | 0777}
	assert.Equal(t, "lrwxrwxrwx", di3.ModeString())
}

func TestInode_SetTimes(t *testing.T) {
	di := &DiskInode{}
	atime := time.Unix(1000000, 0)
	mtime := time.Unix(2000000, 0)
	di.SetTimes(atime, mtime)
	assert.Equal(t, uint32(1000000), di.ATime)
	assert.Equal(t, uint32(2000000), di.MTime)
}

func TestInode_InvalidNum(t *testing.T) {
	dev := newTestDev(t)
	_, err := ReadInode(dev, 0)
	assert.Error(t, err)

	_, err = ReadInode(dev, InodeCount+1)
	assert.Error(t, err)
}
