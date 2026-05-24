package ops

import (
	"time"

	fstype "github.com/yourname/fs-engine/internal/fs"
)

// StatInfo holds the result of a Stat call.
type StatInfo struct {
	Inode  uint32
	Mode   uint16
	UID    uint16
	GID    uint16
	Size   int64
	Blocks uint32
	ATime  time.Time
	MTime  time.Time
	CTime  time.Time
	Links  uint16
	IsDir  bool
	IsLink bool
	IsFile bool
}

func inodeToStat(inum uint32, di *fstype.DiskInode) StatInfo {
	return StatInfo{
		Inode:  inum,
		Mode:   di.Mode,
		UID:    di.UID,
		GID:    di.GID,
		Size:   di.Size(),
		Blocks: di.Blocks512,
		ATime:  time.Unix(int64(di.ATime), 0),
		MTime:  time.Unix(int64(di.MTime), 0),
		CTime:  time.Unix(int64(di.CTime), 0),
		Links:  di.Links,
		IsDir:  di.IsDir(),
		IsLink: di.IsSymlink(),
		IsFile: di.IsRegular(),
	}
}

// Stat returns inode metadata for path, following symlinks.
func Stat(fs *fstype.Filesystem, cred Credential, path string) (StatInfo, error) {
	fs.GetMetrics().StatOps.Add(1)

	inum, err := Resolve(fs, cred, path)
	if err != nil {
		return StatInfo{}, err
	}

	di, err := fs.ReadInode(inum)
	if err != nil {
		return StatInfo{}, err
	}

	return inodeToStat(inum, di), nil
}

// Lstat returns inode metadata for path, without following the last symlink.
func Lstat(fs *fstype.Filesystem, cred Credential, path string) (StatInfo, error) {
	fs.GetMetrics().StatOps.Add(1)

	inum, err := ResolveLstat(fs, cred, path)
	if err != nil {
		return StatInfo{}, err
	}

	di, err := fs.ReadInode(inum)
	if err != nil {
		return StatInfo{}, err
	}

	return inodeToStat(inum, di), nil
}

// Chmod changes the permission bits of the file at path.
func Chmod(fs *fstype.Filesystem, cred Credential, path string, mode uint32) error {
	fs.GetMetrics().ChmodOps.Add(1)

	return fs.RunMutation(func() error {
		inum, err := Resolve(fs, cred, path)
		if err != nil {
			return err
		}
		di, err := fs.ReadInode(inum)
		if err != nil {
			return err
		}
		if cred.UID != 0 && cred.UID != uint32(di.UID) {
			return fstype.ErrPermission
		}
		di.Mode = (di.Mode & fstype.S_IFMT) | uint16(mode&0xFFF)
		di.CTime = uint32(time.Now().Unix())
		return fs.WriteInodeFS(inum, di)
	})
}

// Chown changes the owner and group of the file at path.
func Chown(fs *fstype.Filesystem, cred Credential, path string, uid, gid uint32) error {
	fs.GetMetrics().ChownOps.Add(1)

	return fs.RunMutation(func() error {
		inum, err := Resolve(fs, cred, path)
		if err != nil {
			return err
		}
		di, err := fs.ReadInode(inum)
		if err != nil {
			return err
		}
		if cred.UID != 0 {
			return fstype.ErrPermission
		}
		if uid != ^uint32(0) {
			di.UID = uint16(uid)
		}
		if gid != ^uint32(0) {
			di.GID = uint16(gid)
		}
		di.Mode &^= uint16(fstype.S_ISUID | fstype.S_ISGID)
		di.CTime = uint32(time.Now().Unix())
		return fs.WriteInodeFS(inum, di)
	})
}

// Utimes updates the access and modification times of the file at path.
func Utimes(fs *fstype.Filesystem, cred Credential, path string, atime, mtime time.Time) error {
	return fs.RunMutation(func() error {
		inum, err := Resolve(fs, cred, path)
		if err != nil {
			return err
		}
		di, err := fs.ReadInode(inum)
		if err != nil {
			return err
		}
		if cred.UID != 0 && cred.UID != uint32(di.UID) {
			if err := CheckPermission(di, cred, false, true, false); err != nil {
				return fstype.ErrPermission
			}
		}

		now := time.Now()
		if atime.IsZero() {
			atime = now
		}
		if mtime.IsZero() {
			mtime = now
		}

		di.ATime = uint32(atime.Unix())
		di.MTime = uint32(mtime.Unix())
		di.CTime = uint32(now.Unix())
		return fs.WriteInodeFS(inum, di)
	})
}
