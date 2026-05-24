package fs

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/fs-engine/internal/disk"
)

func TestSuperblockSize(t *testing.T) {
	assert.Equal(t, uintptr(4096), unsafe.Sizeof(DiskSuperblock{}), "DiskSuperblock must be exactly 4096 bytes")
}

func newTestDev(t *testing.T) *disk.BlockDevice {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.img")
	dev, err := disk.NewBlockDevice(path, DiskSize)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = dev.Close()
		os.Remove(path)
	})
	return dev
}

func TestReadWriteSuperblock(t *testing.T) {
	dev := newTestDev(t)

	sb := &DiskSuperblock{
		Magic:      SuperblockMagic,
		MagicV2:    SuperblockMagicV2,
		InodeCount: 4096,
		BlockCount: 16384,
		BlockSize:  4096,
		InodeSize:  128,
		State:      StateClean,
		Version:    1,
	}
	copy(sb.VolumeName[:], "test")

	require.NoError(t, WriteSuperblock(dev, sb))

	got, err := ReadSuperblock(dev)
	require.NoError(t, err)

	assert.Equal(t, sb.Magic, got.Magic)
	assert.Equal(t, sb.MagicV2, got.MagicV2)
	assert.Equal(t, sb.InodeCount, got.InodeCount)
	assert.Equal(t, sb.BlockCount, got.BlockCount)
	assert.Equal(t, sb.BlockSize, got.BlockSize)
	assert.Equal(t, sb.InodeSize, got.InodeSize)
	assert.Equal(t, sb.State, got.State)
	assert.Equal(t, "test", string(got.VolumeName[:4]))
}

func TestSuperblock_Validate(t *testing.T) {
	good := &DiskSuperblock{
		Magic:     SuperblockMagic,
		MagicV2:   SuperblockMagicV2,
		BlockSize: BlockSize,
		InodeSize: InodeSize,
	}
	assert.NoError(t, good.Validate())

	bad := &DiskSuperblock{Magic: 0xDEAD}
	assert.Error(t, bad.Validate())
}
