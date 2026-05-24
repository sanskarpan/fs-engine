package journal

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/fs-engine/internal/disk"
)

func newTestDisk(t *testing.T) *disk.BlockDevice {
	t.Helper()
	path := filepath.Join(t.TempDir(), "journal_test.img")
	dev, err := disk.NewBlockDevice(path, 64*1024*1024)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dev.Close() })
	return dev
}

func TestJournal_Init(t *testing.T) {
	dev := newTestDisk(t)
	j := NewJournal(dev, 10, 20, nil)
	require.NoError(t, j.Init())
}

func TestTransaction_Commit(t *testing.T) {
	dev := newTestDisk(t)
	j := NewJournal(dev, 10, 50, nil)
	require.NoError(t, j.Init())

	// Write a block
	data := make([]byte, BlockSize)
	data[0] = 0xDE
	data[1] = 0xAD

	tx := j.OpenTransaction()
	require.NoError(t, tx.LogBlock(260, data))
	require.NoError(t, tx.Commit())

	// Verify the data was written to its real location
	got, err := dev.ReadBlock(260)
	require.NoError(t, err)
	assert.Equal(t, byte(0xDE), got[0])
	assert.Equal(t, byte(0xAD), got[1])
}

func TestTransaction_MultiBlock(t *testing.T) {
	dev := newTestDisk(t)
	j := NewJournal(dev, 10, 100, nil)
	require.NoError(t, j.Init())

	tx := j.OpenTransaction()
	for i := 0; i < 5; i++ {
		data := make([]byte, BlockSize)
		data[0] = byte(i + 1)
		require.NoError(t, tx.LogBlock(uint32(260+i), data))
	}
	require.NoError(t, tx.Commit())

	for i := 0; i < 5; i++ {
		got, err := dev.ReadBlock(uint32(260 + i))
		require.NoError(t, err)
		assert.Equal(t, byte(i+1), got[0], "block %d mismatch", i)
	}
}

func TestTransaction_EmptyCommit(t *testing.T) {
	dev := newTestDisk(t)
	j := NewJournal(dev, 10, 20, nil)
	require.NoError(t, j.Init())

	tx := j.OpenTransaction()
	assert.NoError(t, tx.Commit()) // empty transaction should succeed
}

func TestJournal_Checkpoint(t *testing.T) {
	dev := newTestDisk(t)
	j := NewJournal(dev, 10, 50, nil)
	require.NoError(t, j.Init())

	data := make([]byte, BlockSize)
	data[5] = 0xFF
	tx := j.OpenTransaction()
	require.NoError(t, tx.LogBlock(260, data))
	require.NoError(t, tx.Commit())

	require.NoError(t, j.Checkpoint())
}

func TestRecover_NoJournal(t *testing.T) {
	dev := newTestDisk(t)
	// Empty journal - should succeed with no-op
	assert.NoError(t, Recover(dev, 10, 20))
}

func TestRecover_CommittedTransaction(t *testing.T) {
	dev := newTestDisk(t)
	j := NewJournal(dev, 132, 128, nil)
	require.NoError(t, j.Init())

	data := make([]byte, BlockSize)
	data[0] = 0xBE
	data[1] = 0xEF

	tx := j.OpenTransaction()
	require.NoError(t, tx.LogBlock(300, data))
	require.NoError(t, tx.Commit())

	// Zero the real block to simulate crash before data write
	zero := make([]byte, BlockSize)
	require.NoError(t, dev.WriteBlock(300, zero))

	// Run recovery
	require.NoError(t, Recover(dev, 132, 128))

	// Data should be restored
	got, err := dev.ReadBlock(300)
	require.NoError(t, err)
	assert.Equal(t, byte(0xBE), got[0])
	assert.Equal(t, byte(0xEF), got[1])
}
