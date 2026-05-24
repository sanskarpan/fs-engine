package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/fs-engine/internal/disk"
)

func newFormattedDev(t *testing.T) (*disk.BlockDevice, *DiskSuperblock) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fs.img")
	dev, err := disk.NewBlockDevice(path, DiskSize)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = dev.Close()
		os.Remove(path)
	})
	sb, err := Format(dev)
	require.NoError(t, err)
	return dev, sb
}

func TestFormat_Superblock(t *testing.T) {
	dev, sb := newFormattedDev(t)

	// Read back the superblock from disk
	got, err := ReadSuperblock(dev)
	require.NoError(t, err)

	assert.Equal(t, uint32(SuperblockMagic), got.Magic)
	assert.Equal(t, uint32(SuperblockMagicV2), got.MagicV2)
	assert.Equal(t, uint32(BlockSize), got.BlockSize)
	assert.Equal(t, uint16(InodeSize), got.InodeSize)
	assert.NoError(t, got.Validate())

	_ = sb
}

func TestFormat_RootInode(t *testing.T) {
	dev, _ := newFormattedDev(t)

	root, err := ReadInode(dev, 1)
	require.NoError(t, err)

	assert.True(t, root.IsDir())
	assert.Equal(t, uint16(S_IFDIR|0755), root.Mode)
	assert.GreaterOrEqual(t, root.Links, uint16(2))
	assert.Greater(t, root.Size(), int64(0))
}

func TestFormat_InodeBitmap(t *testing.T) {
	dev, _ := newFormattedDev(t)

	bm, err := ReadInodeBitmap(dev)
	require.NoError(t, err)

	// Inode 1 (index 0) should be allocated
	assert.True(t, bm.IsSet(0))
	// No other inodes (besides the seeded ones) should be allocated initially
}

func TestFormat_BlockBitmap(t *testing.T) {
	dev, _ := newFormattedDev(t)

	bm, err := ReadBlockBitmap(dev)
	require.NoError(t, err)

	// All metadata blocks should be marked used
	for i := uint32(0); i < DataStart; i++ {
		assert.True(t, bm.IsSet(i), "metadata block %d should be used", i)
	}
}

func TestFormat_RootDirEntries(t *testing.T) {
	dev, _ := newFormattedDev(t)

	root, err := ReadInode(dev, 1)
	require.NoError(t, err)

	entries, err := ReadAllEntries(dev, root)
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}

	assert.True(t, names["."])
	assert.True(t, names[".."])
}

func TestFormat_SeedDirs(t *testing.T) {
	dev, _ := newFormattedDev(t)

	root, err := ReadInode(dev, 1)
	require.NoError(t, err)

	// Check that "bin" exists
	binInum, ft, err := LookupEntry(dev, root, "bin")
	require.NoError(t, err)
	assert.Equal(t, uint8(FT_DIR), ft)

	binInode, err := ReadInode(dev, binInum)
	require.NoError(t, err)
	assert.True(t, binInode.IsDir())
}

func TestFormat_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.img")

	// Format
	dev, err := disk.NewBlockDevice(path, DiskSize)
	require.NoError(t, err)
	_, err = Format(dev)
	require.NoError(t, err)
	require.NoError(t, dev.Sync())
	require.NoError(t, dev.Close())

	// Reopen and validate
	dev2, err := disk.NewBlockDevice(path, DiskSize)
	require.NoError(t, err)
	defer dev2.Close()

	sb, err := ReadSuperblock(dev2)
	require.NoError(t, err)
	assert.NoError(t, sb.Validate())
}
