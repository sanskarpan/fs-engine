package ops

import (
	"time"

	fstype "github.com/yourname/fs-engine/internal/fs"
)

// Create creates a new regular file at path. Returns the new inode number.
func Create(fs *fstype.Filesystem, cred Credential, path string, mode uint32) (uint32, error) {
	fs.GetMetrics().Creates.Add(1)

	var createdInum uint32
	if err := fs.RunMutation(func() error {
		parentInum, name, err := ResolveSplit(fs, cred, path)
		if err != nil {
			return err
		}
		if name == "" {
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

		if existInum, _, err := fstype.LookupEntry(fs.Dev(), parentDI, name); err == nil {
			existDI, err := fs.ReadInode(existInum)
			if err != nil {
				return err
			}
			if existDI.IsDir() {
				return fstype.ErrIsDir
			}
			createdInum = existInum
			return nil
		}

		now := uint32(time.Now().Unix())
		newInum, err := fs.Alloc().AllocInode()
		if err != nil {
			return err
		}

		newDI := &fstype.DiskInode{
			Mode:  uint16(fstype.S_IFREG | (mode & 0777)),
			UID:   uint16(cred.UID),
			GID:   uint16(cred.GID),
			Links: 1,
			ATime: now,
			CTime: now,
			MTime: now,
		}
		if err := fs.WriteInodeFS(newInum, newDI); err != nil {
			return err
		}
		if err := fstype.AddEntry(fs.Dev(), fs.Alloc(), parentDI, name, newInum, fstype.FT_REG_FILE); err != nil {
			return err
		}

		parentDI.MTime = now
		parentDI.CTime = now
		if err := fs.WriteInodeFS(parentInum, parentDI); err != nil {
			return err
		}
		createdInum = newInum
		return nil
	}); err != nil {
		return 0, err
	}

	fs.EmitEvent(fstype.EventCreate, path, nil)
	return createdInum, nil
}

// Link creates a hard link at newpath pointing to the inode at oldpath.
func Link(fs *fstype.Filesystem, cred Credential, oldpath, newpath string) error {
	fs.GetMetrics().LinkOps.Add(1)

	if err := fs.RunMutation(func() error {
		targetInum, err := Resolve(fs, cred, oldpath)
		if err != nil {
			return err
		}

		targetDI, err := fs.ReadInode(targetInum)
		if err != nil {
			return err
		}
		if targetDI.IsDir() {
			return fstype.ErrIsDir
		}

		parentInum, name, err := ResolveSplit(fs, cred, newpath)
		if err != nil {
			return err
		}
		if name == "" {
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

		if _, _, err := fstype.LookupEntry(fs.Dev(), parentDI, name); err == nil {
			return fstype.ErrExists
		}

		ft := inodeFileType(targetDI)
		if err := fstype.AddEntry(fs.Dev(), fs.Alloc(), parentDI, name, targetInum, ft); err != nil {
			return err
		}

		now := uint32(time.Now().Unix())
		parentDI.MTime = now
		parentDI.CTime = now
		if err := fs.WriteInodeFS(parentInum, parentDI); err != nil {
			return err
		}

		targetDI.Links++
		targetDI.CTime = now
		return fs.WriteInodeFS(targetInum, targetDI)
	}); err != nil {
		return err
	}

	fs.EmitEvent(fstype.EventLink, newpath, nil)
	return nil
}

// Symlink creates a symbolic link at path pointing to target.
func Symlink(fs *fstype.Filesystem, cred Credential, target, path string) error {
	fs.GetMetrics().SymlinkOps.Add(1)

	if len(target) == 0 {
		return fstype.ErrInvalidArg
	}

	if err := fs.RunMutation(func() error {
		parentInum, name, err := ResolveSplit(fs, cred, path)
		if err != nil {
			return err
		}
		if name == "" {
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

		if _, _, err := fstype.LookupEntry(fs.Dev(), parentDI, name); err == nil {
			return fstype.ErrExists
		}

		now := uint32(time.Now().Unix())
		newInum, err := fs.Alloc().AllocInode()
		if err != nil {
			return err
		}

		newDI := &fstype.DiskInode{
			Mode:  fstype.S_IFLNK | 0777,
			UID:   uint16(cred.UID),
			GID:   uint16(cred.GID),
			Links: 1,
			ATime: now,
			CTime: now,
			MTime: now,
		}
		newDI.SetSize(int64(len(target)))

		if len(target) < 60 {
			newDI.Blocks512 = 0
			if err := fs.WriteInodeFS(newInum, newDI); err != nil {
				return err
			}
			blockAddr, offset := fstype.InodeNumToBlock(newInum)
			blk, err := fs.Dev().ReadBlock(blockAddr)
			if err != nil {
				return err
			}
			copy(blk[offset+40:offset+40+len(target)], []byte(target))
			if err := fs.Dev().WriteBlock(blockAddr, blk); err != nil {
				return err
			}
			fs.UpdateInodeCacheRaw(newInum, blk[offset:offset+fstype.InodeSize])
		} else {
			blkAddr, err := fs.Alloc().AllocBlock()
			if err != nil {
				return err
			}
			blk := make([]byte, fstype.BlockSize)
			copy(blk, []byte(target))
			if err := fs.Dev().WriteBlock(blkAddr, blk); err != nil {
				return err
			}
			newDI.Direct[0] = blkAddr
			newDI.Blocks512 = fstype.BlockSize / 512
			if err := fs.WriteInodeFS(newInum, newDI); err != nil {
				return err
			}
		}

		if err := fstype.AddEntry(fs.Dev(), fs.Alloc(), parentDI, name, newInum, fstype.FT_SYMLINK); err != nil {
			return err
		}

		parentDI.MTime = now
		parentDI.CTime = now
		return fs.WriteInodeFS(parentInum, parentDI)
	}); err != nil {
		return err
	}

	fs.EmitEvent(fstype.EventLink, path, nil)
	return nil
}

// ReadLink returns the target of a symbolic link.
func ReadLink(fs *fstype.Filesystem, cred Credential, path string) (string, error) {
	inum, err := ResolveLstat(fs, cred, path)
	if err != nil {
		return "", err
	}

	di, err := fs.ReadInode(inum)
	if err != nil {
		return "", err
	}
	if !di.IsSymlink() {
		return "", fstype.ErrInvalidArg
	}

	return readSymlinkTarget(fs, inum, di)
}

// Unlink removes a directory entry and decrements the link count.
// If link count reaches zero, the inode and its blocks are freed.
func Unlink(fs *fstype.Filesystem, cred Credential, path string) error {
	fs.GetMetrics().UnlinkOps.Add(1)

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
		if targetDI.IsDir() {
			return fstype.ErrIsDir
		}

		if err := CheckStickyDelete(parentDI, targetDI, cred); err != nil {
			return err
		}
		if err := fstype.RemoveEntry(fs.Dev(), parentDI, name); err != nil {
			return err
		}

		now := uint32(time.Now().Unix())
		parentDI.MTime = now
		parentDI.CTime = now
		if err := fs.WriteInodeFS(parentInum, parentDI); err != nil {
			return err
		}

		if targetDI.Links > 0 {
			targetDI.Links--
		}
		targetDI.CTime = now

		if targetDI.Links == 0 {
			targetDI.DTime = now
			if err := fs.Alloc().FreeInodeBlocks(targetDI); err != nil {
				return err
			}
			if err := fs.Alloc().FreeInode(targetInum); err != nil {
				return err
			}
			return nil
		}

		if err := fs.WriteInodeFS(targetInum, targetDI); err != nil {
			return err
		}
		fs.UpdateInodeCache(targetInum, targetDI)
		return nil
	}); err != nil {
		return err
	}

	fs.InvalidateInodeCache(targetInum)
	fs.EmitEvent(fstype.EventDelete, path, nil)
	return nil
}

// Rename moves/renames oldpath to newpath.
func Rename(fs *fstype.Filesystem, cred Credential, oldpath, newpath string) error {
	fs.GetMetrics().RenameOps.Add(1)

	var invalidatedDst uint32
	if err := fs.RunMutation(func() error {
		oldParentInum, oldName, err := ResolveSplit(fs, cred, oldpath)
		if err != nil {
			return err
		}
		newParentInum, newName, err := ResolveSplit(fs, cred, newpath)
		if err != nil {
			return err
		}
		if oldName == "" || newName == "" {
			return fstype.ErrInvalidArg
		}

		oldParentDI, err := fs.ReadInode(oldParentInum)
		if err != nil {
			return err
		}
		if err := checkPerm(fs, oldParentDI, cred, false, true, true); err != nil {
			return err
		}

		newParentDI, err := fs.ReadInode(newParentInum)
		if err != nil {
			return err
		}
		if err := checkPerm(fs, newParentDI, cred, false, true, true); err != nil {
			return err
		}

		srcInum, srcFT, err := fstype.LookupEntry(fs.Dev(), oldParentDI, oldName)
		if err != nil {
			return err
		}
		srcDI, err := fs.ReadInode(srcInum)
		if err != nil {
			return err
		}
		if err := CheckStickyDelete(oldParentDI, srcDI, cred); err != nil {
			return err
		}

		dstInum, _, dstErr := fstype.LookupEntry(fs.Dev(), newParentDI, newName)
		if dstErr == nil {
			dstDI, err := fs.ReadInode(dstInum)
			if err != nil {
				return err
			}
			if srcDI.IsDir() && !dstDI.IsDir() {
				return fstype.ErrNotDir
			}
			if !srcDI.IsDir() && dstDI.IsDir() {
				return fstype.ErrIsDir
			}
			if dstDI.IsDir() {
				empty, err := fstype.IsEmptyDir(fs.Dev(), dstDI)
				if err != nil {
					return err
				}
				if !empty {
					return fstype.ErrNotEmpty
				}
			}

			if err := fstype.RemoveEntry(fs.Dev(), newParentDI, newName); err != nil {
				return err
			}
			dstDI.Links--
			if dstDI.Links == 0 {
				if err := fs.Alloc().FreeInodeBlocks(dstDI); err != nil {
					return err
				}
				if err := fs.Alloc().FreeInode(dstInum); err != nil {
					return err
				}
				invalidatedDst = dstInum
			}
		}

		if err := fstype.AddEntry(fs.Dev(), fs.Alloc(), newParentDI, newName, srcInum, srcFT); err != nil {
			return err
		}
		if err := fstype.RemoveEntry(fs.Dev(), oldParentDI, oldName); err != nil {
			return err
		}

		now := uint32(time.Now().Unix())
		oldParentDI.MTime = now
		oldParentDI.CTime = now
		if err := fs.WriteInodeFS(oldParentInum, oldParentDI); err != nil {
			return err
		}
		if newParentInum != oldParentInum {
			newParentDI.MTime = now
			newParentDI.CTime = now
			if err := fs.WriteInodeFS(newParentInum, newParentDI); err != nil {
				return err
			}
		}

		if srcDI.IsDir() {
			blk, err := fs.Dev().ReadBlock(srcDI.Direct[0])
			if err == nil {
				off := 0
				de, consumed, err2 := fstype.ParseDirEntry(blk[off:])
				if err2 == nil && de.Name == "." {
					off += consumed
					if off+8 <= fstype.BlockSize {
						blk[off] = byte(newParentInum)
						blk[off+1] = byte(newParentInum >> 8)
						blk[off+2] = byte(newParentInum >> 16)
						blk[off+3] = byte(newParentInum >> 24)
						if err := fs.Dev().WriteBlock(srcDI.Direct[0], blk); err != nil {
							return err
						}
					}
				}
			}
		}

		srcDI.CTime = now
		return fs.WriteInodeFS(srcInum, srcDI)
	}); err != nil {
		return err
	}

	if invalidatedDst != 0 {
		fs.InvalidateInodeCache(invalidatedDst)
	}
	fs.EmitEvent(fstype.EventRename, newpath, nil)
	return nil
}

// inodeFileType returns the directory entry file type for an inode.
func inodeFileType(di *fstype.DiskInode) uint8 {
	switch di.FileType() {
	case fstype.S_IFREG:
		return fstype.FT_REG_FILE
	case fstype.S_IFDIR:
		return fstype.FT_DIR
	case fstype.S_IFLNK:
		return fstype.FT_SYMLINK
	case fstype.S_IFCHR:
		return fstype.FT_CHRDEV
	case fstype.S_IFBLK:
		return fstype.FT_BLKDEV
	case fstype.S_IFIFO:
		return fstype.FT_FIFO
	case fstype.S_IFSOCK:
		return fstype.FT_SOCK
	default:
		return fstype.FT_UNKNOWN
	}
}
