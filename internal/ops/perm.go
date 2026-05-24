package ops

import (
	fstype "github.com/yourname/fs-engine/internal/fs"
)

// checkPerm wraps CheckPermission and increments the PermDenied counter on failure.
func checkPerm(fs *fstype.Filesystem, di *fstype.DiskInode, cred Credential, read, write, exec bool) error {
	if err := CheckPermission(di, cred, read, write, exec); err != nil {
		fs.GetMetrics().PermDenied.Add(1)
		return err
	}
	return nil
}

// Credential represents the identity of the calling process.
type Credential struct {
	UID uint32
	GID uint32
	CWD uint32 // current working directory inode number
}

// RootCredential is the root credential.
var RootCredential = Credential{UID: 0, GID: 0, CWD: 1}

// CheckPermission verifies that cred has the required permission bits on inode di.
// requiredBits should be a combination of fstype.S_IRUSR, S_IWUSR, S_IXUSR patterns
// (we use the owner bits as the canonical form, and shift appropriately).
func CheckPermission(di *fstype.DiskInode, cred Credential, read, write, exec bool) error {
	// Root bypasses permission checks
	if cred.UID == 0 {
		return nil
	}

	mode := di.Mode

	// Determine which permission triplet to use
	var shift uint
	if cred.UID == uint32(di.UID) {
		shift = 6 // owner
	} else if cred.GID == uint32(di.GID) {
		shift = 3 // group
	} else {
		shift = 0 // other
	}

	perms := (mode >> shift) & 0x7

	if read && perms&4 == 0 {
		return fstype.ErrPermission
	}
	if write && perms&2 == 0 {
		return fstype.ErrPermission
	}
	if exec && perms&1 == 0 {
		return fstype.ErrPermission
	}

	return nil
}

// CheckDirAccess verifies that cred can traverse (execute) the directory.
func CheckDirAccess(di *fstype.DiskInode, cred Credential) error {
	if !di.IsDir() {
		return fstype.ErrNotDir
	}
	return CheckPermission(di, cred, false, false, true)
}

// CheckStickyDelete checks the sticky bit: only owner or directory owner can delete.
// parentDI is the directory containing the entry; fileDI is the entry being deleted.
func CheckStickyDelete(parentDI, fileDI *fstype.DiskInode, cred Credential) error {
	// Root can always delete
	if cred.UID == 0 {
		return nil
	}

	// Check sticky bit on directory
	if parentDI.Mode&fstype.S_ISVTX == 0 {
		return nil // No sticky bit, normal permission check applies
	}

	// With sticky bit: must own the file or own the directory
	if cred.UID == uint32(fileDI.UID) {
		return nil
	}
	if cred.UID == uint32(parentDI.UID) {
		return nil
	}

	return fstype.ErrPermission
}
