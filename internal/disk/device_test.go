package disk

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tmpDevice(t *testing.T) (*BlockDevice, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.img")
	dev, err := NewBlockDevice(path, 64*blockSize)
	require.NoError(t, err)
	return dev, path
}

func TestNewBlockDevice_Create(t *testing.T) {
	dev, _ := tmpDevice(t)
	assert.Equal(t, int64(64*blockSize), dev.Size())
	assert.Equal(t, uint32(64), dev.BlockCount())
}

func TestNewBlockDevice_Reopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reopen.img")

	dev, err := NewBlockDevice(path, 8*blockSize)
	require.NoError(t, err)

	data := make([]byte, blockSize)
	data[0] = 0xAB
	data[blockSize-1] = 0xCD
	require.NoError(t, dev.WriteBlock(0, data))
	require.NoError(t, dev.Sync())
	require.NoError(t, dev.Close())

	dev2, err := NewBlockDevice(path, 8*blockSize)
	require.NoError(t, err)

	got, err := dev2.ReadBlock(0)
	require.NoError(t, err)
	assert.Equal(t, byte(0xAB), got[0])
	assert.Equal(t, byte(0xCD), got[blockSize-1])
}

func TestReadBlock_ReturnsCopy(t *testing.T) {
	dev, _ := tmpDevice(t)
	data := make([]byte, blockSize)
	data[42] = 0xFF
	require.NoError(t, dev.WriteBlock(0, data))

	got, err := dev.ReadBlock(0)
	require.NoError(t, err)
	assert.Equal(t, byte(0xFF), got[42])

	// Mutate the returned slice - should not affect device
	got[42] = 0x00
	got2, err := dev.ReadBlock(0)
	require.NoError(t, err)
	assert.Equal(t, byte(0xFF), got2[42], "device data was mutated by modifying returned slice")
}

func TestReadBlock_OutOfRange(t *testing.T) {
	dev, _ := tmpDevice(t)
	_, err := dev.ReadBlock(64)
	assert.Error(t, err)
}

func TestWriteBlock_WrongSize(t *testing.T) {
	dev, _ := tmpDevice(t)
	err := dev.WriteBlock(0, []byte{1, 2, 3})
	assert.Error(t, err)
}

func TestWriteBlock_OutOfRange(t *testing.T) {
	dev, _ := tmpDevice(t)
	data := make([]byte, blockSize)
	err := dev.WriteBlock(100, data)
	assert.Error(t, err)
}

func TestSync_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sync.img")

	dev, err := NewBlockDevice(path, 4*blockSize)
	require.NoError(t, err)

	data := make([]byte, blockSize)
	for i := range data {
		data[i] = byte(i % 256)
	}
	require.NoError(t, dev.WriteBlock(2, data))
	require.NoError(t, dev.Sync())

	// Verify file on disk
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, data, raw[2*blockSize:3*blockSize])
}

func TestConcurrentAccess(t *testing.T) {
	dev, _ := tmpDevice(t)
	var wg sync.WaitGroup
	const goroutines = 16

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			addr := uint32(n % 64)
			data := make([]byte, blockSize)
			data[0] = byte(n)
			_ = dev.WriteBlock(addr, data)
			_, _ = dev.ReadBlock(addr)
		}(i)
	}
	wg.Wait()
}

func TestNewBlockDevice_InvalidSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.img")
	_, err := NewBlockDevice(path, 1000)
	assert.Error(t, err)
}
