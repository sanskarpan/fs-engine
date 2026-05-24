package ops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	fstype "github.com/yourname/fs-engine/internal/fs"
)

func TestCheckPermission_Root(t *testing.T) {
	di := &fstype.DiskInode{
		Mode: fstype.S_IFREG | 0000, // no permissions
		UID:  1000,
		GID:  1000,
	}
	// Root should bypass all permission checks
	root := Credential{UID: 0, GID: 0}
	assert.NoError(t, CheckPermission(di, root, true, true, true))
}

func TestCheckPermission_Owner(t *testing.T) {
	di := &fstype.DiskInode{
		Mode: fstype.S_IFREG | 0600, // owner read+write
		UID:  1000,
		GID:  1000,
	}
	owner := Credential{UID: 1000, GID: 1000}
	assert.NoError(t, CheckPermission(di, owner, true, true, false))
	assert.Error(t, CheckPermission(di, owner, false, false, true)) // no exec

	other := Credential{UID: 2000, GID: 2000}
	assert.Error(t, CheckPermission(di, other, true, false, false)) // other has no read
}

func TestCheckPermission_Group(t *testing.T) {
	di := &fstype.DiskInode{
		Mode: fstype.S_IFREG | 0040, // group read only
		UID:  1000,
		GID:  500,
	}
	groupMember := Credential{UID: 2000, GID: 500}
	assert.NoError(t, CheckPermission(di, groupMember, true, false, false))
	assert.Error(t, CheckPermission(di, groupMember, false, true, false))
}

func TestCheckPermission_Other(t *testing.T) {
	di := &fstype.DiskInode{
		Mode: fstype.S_IFREG | 0004, // other read
		UID:  1000,
		GID:  1000,
	}
	other := Credential{UID: 2000, GID: 2000}
	assert.NoError(t, CheckPermission(di, other, true, false, false))
	assert.Error(t, CheckPermission(di, other, false, true, false))
}

func TestCheckDirAccess(t *testing.T) {
	dir := &fstype.DiskInode{
		Mode: fstype.S_IFDIR | 0755,
		UID:  0,
		GID:  0,
	}
	user := Credential{UID: 1000, GID: 1000}
	assert.NoError(t, CheckDirAccess(dir, user))

	noExec := &fstype.DiskInode{
		Mode: fstype.S_IFDIR | 0644,
		UID:  0,
		GID:  0,
	}
	assert.Error(t, CheckDirAccess(noExec, user))
}

func TestCheckDirAccess_NotDir(t *testing.T) {
	file := &fstype.DiskInode{Mode: fstype.S_IFREG | 0755}
	assert.Error(t, CheckDirAccess(file, Credential{}))
}

func TestCheckStickyDelete(t *testing.T) {
	// Directory with sticky bit
	parentDir := &fstype.DiskInode{
		Mode: fstype.S_IFDIR | fstype.S_ISVTX | 0777,
		UID:  0,
	}
	// File owned by user 1000
	fileDI := &fstype.DiskInode{
		Mode: fstype.S_IFREG | 0644,
		UID:  1000,
	}

	// Owner of file can delete
	owner := Credential{UID: 1000, GID: 1000}
	assert.NoError(t, CheckStickyDelete(parentDir, fileDI, owner))

	// Root can always delete
	root := Credential{UID: 0}
	assert.NoError(t, CheckStickyDelete(parentDir, fileDI, root))

	// Other user cannot delete
	other := Credential{UID: 2000}
	assert.Error(t, CheckStickyDelete(parentDir, fileDI, other))
}

func TestCheckStickyDelete_NoSticky(t *testing.T) {
	// Without sticky bit, normal permission check applies (not enforced here)
	parentDir := &fstype.DiskInode{
		Mode: fstype.S_IFDIR | 0777, // no sticky
		UID:  0,
	}
	fileDI := &fstype.DiskInode{Mode: fstype.S_IFREG | 0644, UID: 1000}

	other := Credential{UID: 2000}
	// Without sticky, CheckStickyDelete should succeed (parent perms handle the rest)
	assert.NoError(t, CheckStickyDelete(parentDir, fileDI, other))
}
