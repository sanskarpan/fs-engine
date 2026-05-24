package integration_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yourname/fs-engine/internal/disk"
	"github.com/yourname/fs-engine/internal/fs"
	"github.com/yourname/fs-engine/internal/ops"
)

func failAfter(match func(disk.Operation) bool, nth int) func(disk.Operation) error {
	count := 0
	return func(op disk.Operation) error {
		if match(op) {
			count++
			if count == nth {
				return fmt.Errorf("injected %s failure", op.Kind)
			}
		}
		return nil
	}
}

func reconcileSuperblock(t *testing.T, filesystem *fs.Filesystem) {
	t.Helper()
	dev := filesystem.Dev()

	blockBM, err := fs.ReadBlockBitmap(dev)
	require.NoError(t, err, "read block bitmap for reconcile")

	inodeBM, err := fs.ReadInodeBitmap(dev)
	require.NoError(t, err, "read inode bitmap for reconcile")

	sb := filesystem.Superblock()
	sb.FreeBlocks = uint32(fs.TotalBlocks) - uint32(blockBM.Count())
	sb.FreeInodes = uint32(fs.InodeCount) - uint32(inodeBM.Count())

	err = fs.WriteSuperblock(dev, sb)
	require.NoError(t, err, "write reconciled superblock")
}

// freshFS creates a freshly formatted filesystem backed by a temp-dir disk image.
func freshFS(t *testing.T) *fs.Filesystem {
	t.Helper()
	dev, err := disk.NewBlockDevice(t.TempDir()+"/disk.img", 64*1024*1024)
	require.NoError(t, err)
	t.Cleanup(func() { dev.Close() })

	filesystem, err := fs.NewFormat(dev)
	require.NoError(t, err)
	t.Cleanup(func() { filesystem.Unmount() })
	return filesystem
}

// TestSmokeWriteAndRead formats a fresh disk, creates a file, writes 1MB of
// data, reads it back, and verifies the content is identical.
func TestSmokeWriteAndRead(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	// Create /home/alice directory.
	require.NoError(t, ops.Mkdir(filesystem, root, "/home/alice", 0755))

	// Create /home/alice/test.txt.
	inum, err := ops.Create(filesystem, root, "/home/alice/test.txt", 0644)
	require.NoError(t, err)

	// Write 1 MB of data.
	const size = 1 * 1024 * 1024
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	n, err := ops.Write(filesystem, root, inum, 0, data)
	require.NoError(t, err)
	assert.Equal(t, size, n)

	// Read back and verify.
	readBuf := make([]byte, size)
	nr, err := ops.Read(filesystem, root, inum, 0, readBuf)
	require.NoError(t, err)
	assert.Equal(t, size, nr)
	assert.True(t, bytes.Equal(data, readBuf), "read data does not match written data")
}

// TestSmoke100Files creates 100 files in /tmp and verifies that ReadDir returns
// exactly 100 entries (not counting . and ..).
func TestSmoke100Files(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	const count = 100
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("/tmp/file%03d.txt", i)
		_, err := ops.Create(filesystem, root, name, 0644)
		require.NoError(t, err, "create %s", name)
	}

	entries, err := ops.ReadDir(filesystem, root, "/tmp")
	require.NoError(t, err)

	// Filter out . and ..
	var real []ops.DirEntry
	for _, e := range entries {
		if e.Name != "." && e.Name != ".." {
			real = append(real, e)
		}
	}
	assert.Equal(t, count, len(real), "expected %d files, got %d", count, len(real))
}

// TestSmokeHardLink creates a file, hard-links it, checks link count == 2,
// unlinks the original, verifies the link still reads the content, and checks
// link count == 1.
func TestSmokeHardLink(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	require.NoError(t, ops.Mkdir(filesystem, root, "/home/alice", 0755))

	content := []byte("hello hard link world")
	inum, err := ops.Create(filesystem, root, "/home/alice/original.txt", 0644)
	require.NoError(t, err)

	_, err = ops.Write(filesystem, root, inum, 0, content)
	require.NoError(t, err)

	// Create hard link.
	require.NoError(t, ops.Link(filesystem, root, "/home/alice/original.txt", "/home/alice/link.txt"))

	// Invalidate cache so ReadInode fetches from disk (Link writes to disk but
	// may not update the in-memory inode cache).
	filesystem.InvalidateInodeCache(inum)

	// Verify link count == 2.
	di, err := filesystem.ReadInode(inum)
	require.NoError(t, err)
	assert.Equal(t, uint16(2), di.Links, "expected link count 2 after hard link")

	// Unlink the original.
	require.NoError(t, ops.Unlink(filesystem, root, "/home/alice/original.txt"))

	// Invalidate cache to fetch updated inode state from disk.
	filesystem.InvalidateInodeCache(inum)

	// Verify link count == 1 (the link still holds the inode alive).
	di, err = filesystem.ReadInode(inum)
	require.NoError(t, err)
	assert.Equal(t, uint16(1), di.Links, "expected link count 1 after unlinking original")

	// Verify content is still readable via the link.
	linkInum, err := ops.Resolve(filesystem, root, "/home/alice/link.txt")
	require.NoError(t, err)

	readBuf := make([]byte, len(content))
	nr, err := ops.Read(filesystem, root, linkInum, 0, readBuf)
	require.NoError(t, err)
	assert.Equal(t, len(content), nr)
	assert.Equal(t, content, readBuf[:nr])
}

// TestSmokeWriteSurvivesCrashWithoutFSync verifies that a completed write is
// durable and recoverable even without an explicit fsync call.
func TestSmokeWriteSurvivesCrashWithoutFSync(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	content := []byte("write committed without explicit fsync")
	inum, err := ops.Create(filesystem, root, "/tmp/no-fsync.txt", 0644)
	require.NoError(t, err)
	_, err = ops.Write(filesystem, root, inum, 0, content)
	require.NoError(t, err)

	require.NoError(t, filesystem.SimulateCrash())

	resolvedInum, err := ops.Resolve(filesystem, root, "/tmp/no-fsync.txt")
	require.NoError(t, err)
	readBuf := make([]byte, len(content))
	nr, err := ops.Read(filesystem, root, resolvedInum, 0, readBuf)
	require.NoError(t, err)
	assert.Equal(t, len(content), nr)
	assert.Equal(t, content, readBuf[:nr])
}

// TestSmokeRenameSurvivesCrash verifies that a completed rename remains
// consistent after a simulated crash and recovery cycle.
func TestSmokeRenameSurvivesCrash(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	inum, err := ops.Create(filesystem, root, "/tmp/rename-before.txt", 0644)
	require.NoError(t, err)
	_, err = ops.Write(filesystem, root, inum, 0, []byte("rename durability"))
	require.NoError(t, err)

	require.NoError(t, ops.Rename(filesystem, root, "/tmp/rename-before.txt", "/tmp/rename-after.txt"))
	require.NoError(t, filesystem.SimulateCrash())

	_, err = ops.Resolve(filesystem, root, "/tmp/rename-before.txt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotFound), "expected old path to be gone, got: %v", err)

	renamedInum, err := ops.Resolve(filesystem, root, "/tmp/rename-after.txt")
	require.NoError(t, err)
	buf := make([]byte, len("rename durability"))
	n, err := ops.Read(filesystem, root, renamedInum, 0, buf)
	require.NoError(t, err)
	assert.Equal(t, "rename durability", string(buf[:n]))
}

func TestFaultInjectedRenameHomeWriteRecoversFromJournal(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	inum, err := ops.Create(filesystem, root, "/tmp/fault-rename-before.txt", 0644)
	require.NoError(t, err)
	_, err = ops.Write(filesystem, root, inum, 0, []byte("fault rename"))
	require.NoError(t, err)

	sawJournalSync := false
	filesystem.Dev().SetFaultInjector(func(op disk.Operation) error {
		if op.Kind == "sync" && !sawJournalSync {
			sawJournalSync = true
			return nil
		}
		if op.Kind == "write" && sawJournalSync {
			return fmt.Errorf("injected home write failure")
		}
		return nil
	})

	err = ops.Rename(filesystem, root, "/tmp/fault-rename-before.txt", "/tmp/fault-rename-after.txt")
	require.Error(t, err)
	filesystem.Dev().ClearFaultInjector()

	require.NoError(t, filesystem.SimulateCrash())

	_, err = ops.Resolve(filesystem, root, "/tmp/fault-rename-before.txt")
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotFound))

	_, err = ops.Resolve(filesystem, root, "/tmp/fault-rename-after.txt")
	require.NoError(t, err)
}

func TestFaultInjectedTruncateJournalSyncRollsBack(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	inum, err := ops.Create(filesystem, root, "/tmp/fault-truncate.txt", 0644)
	require.NoError(t, err)
	_, err = ops.Write(filesystem, root, inum, 0, []byte("truncate me but keep me safe"))
	require.NoError(t, err)

	filesystem.Dev().SetFaultInjector(failAfter(func(op disk.Operation) bool {
		return op.Kind == "sync"
	}, 1))
	err = ops.Truncate(filesystem, root, inum, 4)
	require.Error(t, err)
	filesystem.Dev().ClearFaultInjector()

	require.NoError(t, filesystem.SimulateCrash())

	resolvedInum, err := ops.Resolve(filesystem, root, "/tmp/fault-truncate.txt")
	require.NoError(t, err)
	buf := make([]byte, 64)
	n, err := ops.Read(filesystem, root, resolvedInum, 0, buf)
	require.NoError(t, err)
	assert.Equal(t, "truncate me but keep me safe", string(buf[:n]))
}

// TestSmokeSymlinkChain creates target.txt, then link2 → target, link1 → link2,
// and verifies that reading via link1 returns the target's content.
func TestSmokeSymlinkChain(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	require.NoError(t, ops.Mkdir(filesystem, root, "/home/alice", 0755))

	content := []byte("symlink chain content")

	// Create the actual target file.
	inum, err := ops.Create(filesystem, root, "/home/alice/target.txt", 0644)
	require.NoError(t, err)
	_, err = ops.Write(filesystem, root, inum, 0, content)
	require.NoError(t, err)

	// link2 → target
	require.NoError(t, ops.Symlink(filesystem, root, "/home/alice/target.txt", "/home/alice/link2.txt"))
	// link1 → link2
	require.NoError(t, ops.Symlink(filesystem, root, "/home/alice/link2.txt", "/home/alice/link1.txt"))

	// Resolve link1 — should follow link1 → link2 → target.
	resolvedInum, err := ops.Resolve(filesystem, root, "/home/alice/link1.txt")
	require.NoError(t, err)

	readBuf := make([]byte, len(content))
	nr, err := ops.Read(filesystem, root, resolvedInum, 0, readBuf)
	require.NoError(t, err)
	assert.Equal(t, len(content), nr)
	assert.Equal(t, content, readBuf[:nr])
}

// TestSmokeSymlinkLoop creates a circular symlink (/tmp/a → /tmp/b → /tmp/a)
// and verifies that resolving /tmp/a returns fs.ErrLoop.
func TestSmokeSymlinkLoop(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	// /tmp/a → /tmp/b
	require.NoError(t, ops.Symlink(filesystem, root, "/tmp/b", "/tmp/a"))
	// /tmp/b → /tmp/a
	require.NoError(t, ops.Symlink(filesystem, root, "/tmp/a", "/tmp/b"))

	_, err := ops.Resolve(filesystem, root, "/tmp/a")
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrLoop), "expected ErrLoop, got: %v", err)
}

// TestSmokeJournalRecovery writes a file, simulates a crash (marks dirty and
// recovers via journal replay), and verifies the file is still readable.
func TestSmokeJournalRecovery(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	content := []byte("recovery test data")
	inum, err := ops.Create(filesystem, root, "/tmp/recovery.txt", 0644)
	require.NoError(t, err)
	_, err = ops.Write(filesystem, root, inum, 0, content)
	require.NoError(t, err)

	// Flush to disk before simulating crash.
	require.NoError(t, ops.FSync(filesystem, inum))

	// Simulate crash + recovery.
	require.NoError(t, filesystem.SimulateCrash())

	// File should still be readable.
	resolvedInum, err := ops.Resolve(filesystem, root, "/tmp/recovery.txt")
	require.NoError(t, err)

	readBuf := make([]byte, len(content))
	nr, err := ops.Read(filesystem, root, resolvedInum, 0, readBuf)
	require.NoError(t, err)
	assert.Equal(t, len(content), nr)
	assert.Equal(t, content, readBuf[:nr])
}

// TestSmokePermissionDenied creates a file with mode 0600 owned by root (uid=0),
// then tries to read it as uid=1000 and expects ErrPermission.
func TestSmokePermissionDenied(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	require.NoError(t, ops.Mkdir(filesystem, root, "/home/alice", 0755))

	// Create secret.txt as root with mode 0600.
	inum, err := ops.Create(filesystem, root, "/home/alice/secret.txt", 0600)
	require.NoError(t, err)

	_, err = ops.Write(filesystem, root, inum, 0, []byte("secret"))
	require.NoError(t, err)

	// Attempt to read as uid=1000.
	userCred := ops.Credential{UID: 1000, GID: 1000}
	readBuf := make([]byte, 16)
	_, err = ops.Read(filesystem, userCred, inum, 0, readBuf)
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrPermission), "expected ErrPermission, got: %v", err)
}

// TestSmokeDiskFull writes 4MB chunks until ENOSPC and verifies ErrNoSpace.
func TestSmokeDiskFull(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	const chunkSize = 4 * 1024 * 1024
	chunk := make([]byte, chunkSize)
	for i := range chunk {
		chunk[i] = byte(i % 256)
	}

	var lastErr error
	for fileIdx := 0; fileIdx < 100; fileIdx++ {
		name := fmt.Sprintf("/tmp/bigfile%03d.bin", fileIdx)
		inum, err := ops.Create(filesystem, root, name, 0644)
		if err != nil {
			lastErr = err
			break
		}
		_, err = ops.Write(filesystem, root, inum, 0, chunk)
		if err != nil {
			lastErr = err
			break
		}
	}

	require.Error(t, lastErr, "expected disk-full error before all files were written")
	assert.True(t, errors.Is(lastErr, fs.ErrNoSpace), "expected ErrNoSpace, got: %v", lastErr)
}

// TestSmokeLargeFile5MB writes a 5MB file and verifies the allocated block count
// is at least 1250 (5MB / 4KB = 1280 blocks expected).
func TestSmokeLargeFile5MB(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	const size = 5 * 1024 * 1024
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 199)
	}

	inum, err := ops.Create(filesystem, root, "/tmp/large.bin", 0644)
	require.NoError(t, err)

	n, err := ops.Write(filesystem, root, inum, 0, data)
	require.NoError(t, err)
	assert.Equal(t, size, n)

	di, err := filesystem.ReadInode(inum)
	require.NoError(t, err)

	// Blocks512 is in 512-byte units; divide by 8 to get 4KB blocks.
	blocks4k := di.Blocks512 / 8
	assert.GreaterOrEqual(t, blocks4k, uint32(1250),
		"expected >= 1250 4KB blocks for 5MB file, got %d (Blocks512=%d)", blocks4k, di.Blocks512)
}

// TestSmokeFsckClean seeds the filesystem with files, runs FsckFull(), and
// verifies the filesystem is clean with no issues.
func TestSmokeFsckClean(t *testing.T) {
	filesystem := freshFS(t)
	root := ops.RootCredential

	// Seed with a few files and directories.
	require.NoError(t, ops.Mkdir(filesystem, root, "/home/alice", 0755))

	inum, err := ops.Create(filesystem, root, "/home/alice/notes.txt", 0644)
	require.NoError(t, err)
	_, err = ops.Write(filesystem, root, inum, 0, []byte("fsck test content"))
	require.NoError(t, err)

	_, err = ops.Create(filesystem, root, "/tmp/scratch.txt", 0644)
	require.NoError(t, err)

	result := filesystem.FsckFull()
	require.NotNil(t, result)
	assert.True(t, result.Clean, "expected clean filesystem, issues: %v", result.Issues)
	assert.Empty(t, result.Issues, "expected no issues, got: %v", result.Issues)
}
