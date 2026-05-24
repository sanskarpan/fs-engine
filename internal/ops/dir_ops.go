package ops

import (
	"time"

	fstype "github.com/yourname/fs-engine/internal/fs"
)

// DirEntry is the user-facing directory entry type.
type DirEntry struct {
	Inode    uint32
	Name     string
	FileType uint8
}

// Mkdir creates a new directory at path with the given mode.
func Mkdir(fs *fstype.Filesystem, cred Credential, path string, mode uint32) error {
	fs.GetMetrics().MkdirOps.Add(1)

	if err := fs.RunMutation(func() error {
		parentInum, name, err := ResolveSplit(fs, cred, path)
		if err != nil {
			return err
		}
		if name == "" {
			return fstype.ErrInvalidArg
		}
		if len(name) > fstype.MaxNameLen {
			return fstype.ErrNameTooLong
		}

		parentDI, err := fs.ReadInode(parentInum)
		if err != nil {
			return err
		}
		if !parentDI.IsDir() {
			return fstype.ErrNotDir
		}
		if err := checkPerm(fs, parentDI, cred, false, true, true); err != nil {
			return err
		}

		if _, _, err := fstype.LookupEntry(fs.Dev(), parentDI, name); err == nil {
			return fstype.ErrExists
		}

		newInum, err := fs.Alloc().AllocInode()
		if err != nil {
			return err
		}
		dirBlk, err := fs.Alloc().AllocBlock()
		if err != nil {
			return err
		}

		now := uint32(time.Now().Unix())
		newDI := &fstype.DiskInode{
			Mode:      uint16(fstype.S_IFDIR | (mode & 0777)),
			UID:       uint16(cred.UID),
			GID:       uint16(cred.GID),
			Links:     2,
			Blocks512: fstype.BlockSize / 512,
			ATime:     now,
			CTime:     now,
			MTime:     now,
		}
		newDI.Direct[0] = dirBlk
		newDI.SetSize(fstype.BlockSize)

		if err := fs.WriteInodeFS(newInum, newDI); err != nil {
			return err
		}
		if err := fstype.InitDirBlock(fs.Dev(), newDI, newInum, parentInum); err != nil {
			return err
		}

		parentDI.Links++
		if err := fstype.AddEntry(fs.Dev(), fs.Alloc(), parentDI, name, newInum, fstype.FT_DIR); err != nil {
			return err
		}
		parentDI.MTime = now
		parentDI.CTime = now
		return fs.WriteInodeFS(parentInum, parentDI)
	}); err != nil {
		return err
	}

	fs.EmitEvent(fstype.EventMkdir, path, nil)
	return nil
}

// Rmdir removes an empty directory at path.
func Rmdir(fs *fstype.Filesystem, cred Credential, path string) error {
	fs.GetMetrics().RmdirOps.Add(1)

	var targetInum uint32
	if err := fs.RunMutation(func() error {
		parentInum, name, err := ResolveSplit(fs, cred, path)
		if err != nil {
			return err
		}
		if name == "" || name == "." || name == ".." {
			return fstype.ErrInvalidArg
		}

		parentDI, err := fs.ReadInode(parentInum)
		if err != nil {
			return err
		}
		if !parentDI.IsDir() {
			return fstype.ErrNotDir
		}
		if err := checkPerm(fs, parentDI, cred, false, true, true); err != nil {
			return err
		}

		targetInum, _, err = fstype.LookupEntry(fs.Dev(), parentDI, name)
		if err != nil {
			return err
		}

		targetDI, err := fs.ReadInode(targetInum)
		if err != nil {
			return err
		}
		if !targetDI.IsDir() {
			return fstype.ErrNotDir
		}

		if err := CheckStickyDelete(parentDI, targetDI, cred); err != nil {
			return err
		}
		empty, err := fstype.IsEmptyDir(fs.Dev(), targetDI)
		if err != nil {
			return err
		}
		if !empty {
			return fstype.ErrNotEmpty
		}

		if err := fstype.RemoveEntry(fs.Dev(), parentDI, name); err != nil {
			return err
		}
		if err := fs.Alloc().FreeInodeBlocks(targetDI); err != nil {
			return err
		}
		if err := fs.Alloc().FreeInode(targetInum); err != nil {
			return err
		}

		if parentDI.Links > 0 {
			parentDI.Links--
		}
		now := uint32(time.Now().Unix())
		parentDI.MTime = now
		parentDI.CTime = now
		return fs.WriteInodeFS(parentInum, parentDI)
	}); err != nil {
		return err
	}

	fs.InvalidateInodeCache(targetInum)
	fs.EmitEvent(fstype.EventRmdir, path, nil)
	return nil
}

// ReadDir reads all directory entries from path.
func ReadDir(fs *fstype.Filesystem, cred Credential, path string) ([]DirEntry, error) {
	var inum uint32
	var err error
	if path == "/" {
		inum = 1
	} else {
		inum, err = Resolve(fs, cred, path)
		if err != nil {
			return nil, err
		}
	}

	di, err := fs.ReadInode(inum)
	if err != nil {
		return nil, err
	}
	if !di.IsDir() {
		return nil, fstype.ErrNotDir
	}
	if err := checkPerm(fs, di, cred, true, false, false); err != nil {
		return nil, err
	}

	rawEntries, err := fstype.ReadAllEntries(fs.Dev(), di)
	if err != nil {
		return nil, err
	}

	result := make([]DirEntry, 0, len(rawEntries))
	for _, e := range rawEntries {
		result = append(result, DirEntry{
			Inode:    e.Inode,
			Name:     e.Name,
			FileType: e.FileType,
		})
	}
	return result, nil
}

// GetCwd returns the path of the given directory inode.
// This is a simplified implementation that walks up to root.
func GetCwd(fs *fstype.Filesystem, cred Credential, dirInum uint32) (string, error) {
	if dirInum == 1 {
		return "/", nil
	}

	var parts []string
	cur := dirInum
	for cur != 1 {
		di, err := fs.ReadInode(cur)
		if err != nil {
			return "", err
		}
		if !di.IsDir() {
			return "", fstype.ErrNotDir
		}

		// Find ".." entry
		parentInum, _, err := fstype.LookupEntry(fs.Dev(), di, "..")
		if err != nil {
			return "", err
		}

		// Find our name in parent
		parentDI, err := fs.ReadInode(parentInum)
		if err != nil {
			return "", err
		}

		var myName string
		err = fstype.IterDirEntries(fs.Dev(), parentDI, func(de fstype.DirEntry, _ uint32, _ int) bool {
			if de.Inode == cur && de.Name != "." && de.Name != ".." {
				myName = de.Name
				return false
			}
			return true
		})
		if err != nil {
			return "", err
		}
		if myName == "" {
			return "", fstype.ErrNotFound
		}

		parts = append([]string{myName}, parts...)
		cur = parentInum
	}

	return "/" + joinPath(parts), nil
}

func joinPath(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "/"
		}
		result += p
	}
	return result
}
