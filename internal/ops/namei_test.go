package ops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/fs-engine/internal/disk"
	fstype "github.com/yourname/fs-engine/internal/fs"
)

func newTestFS(t *testing.T) *fstype.Filesystem {
	t.Helper()
	path := filepath.Join(t.TempDir(), "namei_test.img")
	dev, err := disk.NewBlockDevice(path, fstype.DiskSize)
	require.NoError(t, err)
	t.Cleanup(func() {
		os.Remove(path)
	})
	fs, err := fstype.NewFormat(dev)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = fs.Unmount()
		_ = dev.Close()
	})
	return fs
}

func TestResolve_Root(t *testing.T) {
	fs := newTestFS(t)
	cred := RootCredential

	inum, err := Resolve(fs, cred, "/")
	require.NoError(t, err)
	assert.Equal(t, uint32(1), inum)
}

func TestResolve_Dir(t *testing.T) {
	fs := newTestFS(t)
	cred := RootCredential
	cred.CWD = 1

	// "bin" should exist from seeding
	inum, err := Resolve(fs, cred, "/bin")
	require.NoError(t, err)
	assert.Greater(t, inum, uint32(1))
}

func TestResolve_NotFound(t *testing.T) {
	fs := newTestFS(t)
	cred := RootCredential
	cred.CWD = 1

	_, err := Resolve(fs, cred, "/nonexistent/path")
	assert.Error(t, err)
}

func TestResolveSplit_Simple(t *testing.T) {
	fs := newTestFS(t)
	cred := RootCredential
	cred.CWD = 1

	parentInum, name, err := ResolveSplit(fs, cred, "/bin/ls")
	require.NoError(t, err)
	assert.Equal(t, "ls", name)
	assert.Greater(t, parentInum, uint32(0))
}

func TestResolveSplit_RootChild(t *testing.T) {
	fs := newTestFS(t)
	cred := RootCredential
	cred.CWD = 1

	parentInum, name, err := ResolveSplit(fs, cred, "/newdir")
	require.NoError(t, err)
	assert.Equal(t, uint32(1), parentInum)
	assert.Equal(t, "newdir", name)
}

func TestResolve_Symlink(t *testing.T) {
	fs := newTestFS(t)
	cred := RootCredential
	cred.CWD = 1

	// Create a file
	inum, err := Create(fs, cred, "/etc/testfile", 0644)
	require.NoError(t, err)

	// Create a symlink pointing to it
	err = Symlink(fs, cred, "/etc/testfile", "/etc/testlink")
	require.NoError(t, err)

	// Resolve should follow symlink
	resolvedInum, err := Resolve(fs, cred, "/etc/testlink")
	require.NoError(t, err)
	assert.Equal(t, inum, resolvedInum)
}

func TestResolveLstat_NoFollow(t *testing.T) {
	fs := newTestFS(t)
	cred := RootCredential
	cred.CWD = 1

	// Create symlink
	require.NoError(t, Symlink(fs, cred, "/etc/hostname", "/etc/hnlink"))

	// Lstat should return the symlink inode, not the target
	linkInum, err := ResolveLstat(fs, cred, "/etc/hnlink")
	require.NoError(t, err)

	di, err := fs.ReadInode(linkInum)
	require.NoError(t, err)
	assert.True(t, di.IsSymlink(), "Lstat should return symlink inode")
}

func TestResolve_TooManySymlinks(t *testing.T) {
	fs := newTestFS(t)
	cred := RootCredential
	cred.CWD = 1

	// Create circular symlinks
	require.NoError(t, Symlink(fs, cred, "/tmp/b", "/tmp/a"))
	require.NoError(t, Symlink(fs, cred, "/tmp/a", "/tmp/b"))

	_, err := Resolve(fs, cred, "/tmp/a")
	assert.ErrorIs(t, err, fstype.ErrLoop)
}
