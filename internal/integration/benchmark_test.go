package integration_test

import (
	"bytes"
	"testing"

	"github.com/yourname/fs-engine/internal/disk"
	"github.com/yourname/fs-engine/internal/fs"
	"github.com/yourname/fs-engine/internal/ops"
)

func freshFSBench(b *testing.B) *fs.Filesystem {
	b.Helper()
	dev, err := disk.NewBlockDevice(b.TempDir()+"/disk.img", 64*1024*1024)
	if err != nil {
		b.Fatalf("new block device: %v", err)
	}
	b.Cleanup(func() { _ = dev.Close() })

	filesystem, err := fs.NewFormat(dev)
	if err != nil {
		b.Fatalf("new format: %v", err)
	}
	b.Cleanup(func() { _ = filesystem.Unmount() })
	return filesystem
}

func BenchmarkWriteRead1MB(b *testing.B) {
	filesystem := freshFSBench(b)
	root := ops.RootCredential

	inum, err := ops.Create(filesystem, root, "/tmp/bench.bin", 0644)
	if err != nil {
		b.Fatalf("create bench file: %v", err)
	}

	payload := make([]byte, 1<<20)
	readBuf := make([]byte, len(payload))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = ops.Write(filesystem, root, inum, 0, payload)
		if err != nil {
			b.Fatalf("write bench payload: %v", err)
		}

		_, err = ops.Read(filesystem, root, inum, 0, readBuf)
		if err != nil {
			b.Fatalf("read bench payload: %v", err)
		}

		if !bytes.Equal(payload, readBuf) {
			b.Fatal("benchmark read mismatch")
		}
	}
}

func BenchmarkStatHotPath(b *testing.B) {
	filesystem := freshFSBench(b)
	root := ops.RootCredential

	_, err := ops.Create(filesystem, root, "/tmp/stat-hot.txt", 0644)
	if err != nil {
		b.Fatalf("create stat bench file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ops.Resolve(filesystem, root, "/tmp/stat-hot.txt")
		if err != nil {
			b.Fatalf("resolve stat bench file: %v", err)
		}
	}
}
