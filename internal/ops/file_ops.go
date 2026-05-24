package ops

import (
	"time"

	fstype "github.com/yourname/fs-engine/internal/fs"
)

// Read reads up to len(buf) bytes from file inum starting at offset.
// Returns the number of bytes read.
func Read(fs *fstype.Filesystem, cred Credential, inum uint32, offset int64, buf []byte) (int, error) {
	fs.GetMetrics().Reads.Add(1)

	di, err := fs.ReadInode(inum)
	if err != nil {
		return 0, err
	}

	if di.IsDir() {
		return 0, fstype.ErrIsDir
	}

	if err := checkPerm(fs, di, cred, true, false, false); err != nil {
		return 0, err
	}

	size := di.Size()
	if offset >= size {
		return 0, nil
	}
	if int64(len(buf))+offset > size {
		buf = buf[:size-offset]
	}
	if len(buf) == 0 {
		return 0, nil
	}

	totalRead := 0
	for len(buf) > 0 {
		logicalBlock := uint32(offset / fstype.BlockSize)
		blockOff := int(offset % fstype.BlockSize)

		physBlk, err := fstype.GetBlockForOffset(fs.Dev(), di, logicalBlock)
		if err != nil {
			return totalRead, err
		}

		var blkData []byte
		if physBlk == 0 {
			// Sparse hole - return zeros
			blkData = make([]byte, fstype.BlockSize)
		} else {
			blkData, err = fs.BufCache().GetBlock(physBlk)
			if err != nil {
				return totalRead, err
			}
		}

		canRead := fstype.BlockSize - blockOff
		if canRead > len(buf) {
			canRead = len(buf)
		}

		copy(buf[:canRead], blkData[blockOff:blockOff+canRead])
		if physBlk != 0 {
			// Unpin (PutBlock with same data = no-op for data, just decrements pin)
			_ = fs.BufCache().PutBlock(physBlk, blkData)
		}
		buf = buf[canRead:]
		offset += int64(canRead)
		totalRead += canRead
	}

	// Update atime
	di.ATime = uint32(time.Now().Unix())
	fs.UpdateInodeCache(inum, di)

	fs.GetMetrics().ReadBytes.Add(int64(totalRead))
	return totalRead, nil
}

// Write writes buf to file inum starting at offset.
// Allocates blocks as needed. Returns bytes written.
func Write(fs *fstype.Filesystem, cred Credential, inum uint32, offset int64, buf []byte) (int, error) {
	fs.GetMetrics().Writes.Add(1)

	var totalWritten int
	err := fs.RunMutation(func() error {
		di, err := fs.ReadInode(inum)
		if err != nil {
			return err
		}

		if di.IsDir() {
			return fstype.ErrIsDir
		}

		if err := checkPerm(fs, di, cred, false, true, false); err != nil {
			return err
		}

		if len(buf) == 0 {
			return nil
		}

		writeBuf := append([]byte(nil), buf...)
		curOffset := offset
		for len(writeBuf) > 0 {
			logicalBlock := uint32(curOffset / fstype.BlockSize)
			blockOff := int(curOffset % fstype.BlockSize)

			physBlk, err := fstype.AllocBlockForOffset(fs.Dev(), fs.Alloc(), di, logicalBlock)
			if err != nil {
				return err
			}

			blkData, err := fs.BufCache().GetBlock(physBlk)
			if err != nil {
				return err
			}

			canWrite := fstype.BlockSize - blockOff
			if canWrite > len(writeBuf) {
				canWrite = len(writeBuf)
			}

			copy(blkData[blockOff:blockOff+canWrite], writeBuf[:canWrite])
			if err := fs.BufCache().PutBlock(physBlk, blkData); err != nil {
				return err
			}
			fs.BufCache().MarkDirty(physBlk)

			writeBuf = writeBuf[canWrite:]
			curOffset += int64(canWrite)
			totalWritten += canWrite
		}

		now := uint32(time.Now().Unix())
		if curOffset > di.Size() {
			di.SetSize(curOffset)
		}
		di.MTime = now
		di.CTime = now

		return fs.WriteInodeFS(inum, di)
	})
	if err != nil {
		return totalWritten, err
	}

	fs.GetMetrics().WriteBytes.Add(int64(totalWritten))
	fs.EmitEvent(fstype.EventWrite, "", nil)
	return totalWritten, nil
}

// Truncate truncates or extends a file to the given size.
func Truncate(fs *fstype.Filesystem, cred Credential, inum uint32, size int64) error {
	if size < 0 {
		return fstype.ErrInvalidArg
	}

	return fs.RunMutation(func() error {
		di, err := fs.ReadInode(inum)
		if err != nil {
			return err
		}

		if di.IsDir() {
			return fstype.ErrIsDir
		}

		if err := checkPerm(fs, di, cred, false, true, false); err != nil {
			return err
		}

		curSize := di.Size()
		if size == curSize {
			return nil
		}

		if size < curSize {
			startLogical := uint32((size + fstype.BlockSize - 1) / fstype.BlockSize)
			if err := fstype.FreeBlocksFrom(fs.Dev(), fs.Alloc(), di, startLogical); err != nil {
				return err
			}
		}

		di.SetSize(size)
		now := uint32(time.Now().Unix())
		di.MTime = now
		di.CTime = now
		return fs.WriteInodeFS(inum, di)
	})
}

// FSync flushes all dirty data for an inode to disk.
func FSync(fs *fstype.Filesystem, inum uint32) error {
	fs.GetMetrics().FsyncCalls.Add(1)
	if err := fs.FlushInodeCache(inum); err != nil {
		return err
	}
	if err := fs.BufCache().FlushAll(); err != nil {
		return err
	}
	return fs.Dev().Sync()
}

// encodeInodeInBlock encodes a DiskInode into a 128-byte buffer.
// This is a local copy to avoid circular import; mirrors inode.go's encodeInode.
func encodeInodeInBlock(di *fstype.DiskInode, buf []byte) {
	// We call through the exported WriteInode path for the actual encoding
	// but we need a local helper here. Just use the raw byte layout.
	// Since DiskInode uses explicit encoding in inode.go, we reproduce it:
	putU16 := func(b []byte, v uint16) {
		b[0] = byte(v)
		b[1] = byte(v >> 8)
	}
	putU32 := func(b []byte, v uint32) {
		b[0] = byte(v)
		b[1] = byte(v >> 8)
		b[2] = byte(v >> 16)
		b[3] = byte(v >> 24)
	}
	putU16(buf[0:], di.Mode)
	putU16(buf[2:], di.UID)
	putU32(buf[4:], di.SizeLo)
	putU32(buf[8:], di.ATime)
	putU32(buf[12:], di.CTime)
	putU32(buf[16:], di.MTime)
	putU32(buf[20:], di.DTime)
	putU16(buf[24:], di.GID)
	putU16(buf[26:], di.Links)
	putU32(buf[28:], di.Blocks512)
	putU32(buf[32:], di.Flags)
	for i, ptr := range di.Direct {
		putU32(buf[40+i*4:], ptr)
	}
	putU32(buf[88:], di.Indirect1)
	putU32(buf[92:], di.Indirect2)
	putU32(buf[96:], di.Indirect3)
	putU32(buf[104:], di.Generation)
	putU32(buf[116:], di.SizeHi)
}
