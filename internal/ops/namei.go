package ops

import (
	"strings"

	fstype "github.com/yourname/fs-engine/internal/fs"
)

const maxSymlinkDepth = 40

// Resolve resolves a path to an inode number.
// Follows symlinks up to maxSymlinkDepth levels.
func Resolve(fs *fstype.Filesystem, cred Credential, path string) (uint32, error) {
	inum, _, err := resolve(fs, cred, path, 0, false)
	return inum, err
}

// ResolveSplit resolves a path and returns the parent inode number and the
// final component name (without following the last component if it's a symlink).
func ResolveSplit(fs *fstype.Filesystem, cred Credential, path string) (parentInum uint32, name string, err error) {
	if path == "" {
		return 0, "", fstype.ErrInvalidArg
	}
	if path == "/" {
		return 1, "", nil
	}

	// Normalise
	path = strings.TrimRight(path, "/")

	lastSlash := strings.LastIndex(path, "/")
	var dir, base string
	if lastSlash < 0 {
		dir = ""
		base = path
	} else {
		dir = path[:lastSlash]
		base = path[lastSlash+1:]
	}

	if base == "" {
		return 0, "", fstype.ErrInvalidArg
	}
	if len(base) > fstype.MaxNameLen {
		return 0, "", fstype.ErrNameTooLong
	}

	var pInum uint32
	if dir == "" || dir == "/" {
		pInum = startingInode(fs, cred, path)
		if dir == "/" || path[0] == '/' {
			pInum = 1
		} else {
			pInum = cred.CWD
			if pInum == 0 {
				pInum = 1
			}
		}
	} else {
		pInum, _, err = resolve(fs, cred, dir, 0, false)
		if err != nil {
			return 0, "", err
		}
	}

	return pInum, base, nil
}

// ResolveLstat resolves a path without following the last symlink.
func ResolveLstat(fs *fstype.Filesystem, cred Credential, path string) (uint32, error) {
	inum, _, err := resolve(fs, cred, path, 0, true)
	return inum, err
}

func startingInode(fs *fstype.Filesystem, cred Credential, path string) uint32 {
	if len(path) > 0 && path[0] == '/' {
		return 1
	}
	if cred.CWD != 0 {
		return cred.CWD
	}
	return 1
}

func resolve(fs *fstype.Filesystem, cred Credential, path string, depth int, lstat bool) (uint32, *fstype.DiskInode, error) {
	if depth > maxSymlinkDepth {
		return 0, nil, fstype.ErrLoop
	}

	if path == "" {
		inum := cred.CWD
		if inum == 0 {
			inum = 1
		}
		di, err := fs.ReadInode(inum)
		if err != nil {
			return 0, nil, err
		}
		return inum, di, nil
	}

	var curInum uint32
	if path[0] == '/' {
		curInum = 1
		path = strings.TrimLeft(path, "/")
	} else {
		curInum = cred.CWD
		if curInum == 0 {
			curInum = 1
		}
	}

	if path == "" {
		// Was just "/"
		di, err := fs.ReadInode(curInum)
		if err != nil {
			return 0, nil, err
		}
		return curInum, di, nil
	}

	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}

		curDI, err := fs.ReadInode(curInum)
		if err != nil {
			return 0, nil, err
		}

		if !curDI.IsDir() {
			return 0, nil, fstype.ErrNotDir
		}

		// Check execute permission on directory
		if err := CheckDirAccess(curDI, cred); err != nil {
			return 0, nil, err
		}

		childInum, _, err := fstype.LookupEntry(fs.Dev(), curDI, part)
		if err != nil {
			return 0, nil, err
		}

		childDI, err := fs.ReadInode(childInum)
		if err != nil {
			return 0, nil, err
		}

		// Follow symlink unless it's the last component and lstat is true
		isLast := (i == len(parts)-1)
		if childDI.IsSymlink() && !(lstat && isLast) {
			target, err := readSymlinkTarget(fs, childInum, childDI)
			if err != nil {
				return 0, nil, err
			}

			// Build full remaining path
			remaining := target
			if !isLast {
				rest := strings.Join(parts[i+1:], "/")
				if rest != "" {
					remaining = target + "/" + rest
				}
			}

			// Resolve from current dir or root depending on absolute/relative
			var symlinkCred Credential
			if len(target) > 0 && target[0] == '/' {
				symlinkCred = cred
			} else {
				// Relative symlink resolved from the directory containing it
				symlinkCred = Credential{UID: cred.UID, GID: cred.GID, CWD: curInum}
			}

			return resolve(fs, symlinkCred, remaining, depth+1, lstat && isLast)
		}

		curInum = childInum
	}

	di, err := fs.ReadInode(curInum)
	if err != nil {
		return 0, nil, err
	}
	return curInum, di, nil
}

// readSymlinkTarget reads the target of a symlink.
func readSymlinkTarget(fs *fstype.Filesystem, inum uint32, di *fstype.DiskInode) (string, error) {
	size := di.Size()
	if size < 0 || size > 4096 {
		return "", fstype.ErrBadFS
	}

	// Fast symlink: stored inline in inode block pointers
	if di.Blocks512 == 0 && size < 60 {
		// Target is stored as raw bytes in the direct/indirect pointer fields
		// We need to read the raw inode block bytes
		blockAddr, offset := fstype.InodeNumToBlock(inum)
		blk, err := fs.Dev().ReadBlock(blockAddr)
		if err != nil {
			return "", err
		}
		// Direct blocks start at offset 40 in the encoded inode
		inodeData := blk[offset : offset+fstype.InodeSize]
		target := make([]byte, size)
		copy(target, inodeData[40:40+int(size)])
		return string(target), nil
	}

	// Regular symlink
	if di.Direct[0] == 0 {
		return "", fstype.ErrBadFS
	}
	blk, err := fs.Dev().ReadBlock(di.Direct[0])
	if err != nil {
		return "", err
	}
	if size > int64(len(blk)) {
		size = int64(len(blk))
	}
	return string(blk[:size]), nil
}
