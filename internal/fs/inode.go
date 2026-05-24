package fs

import (
	"encoding/binary"
	"fmt"
	"time"
	"unsafe"

	"github.com/yourname/fs-engine/internal/disk"
)

// DiskInode is the on-disk inode structure, exactly 128 bytes.
type DiskInode struct {
	Mode       uint16
	UID        uint16
	SizeLo     uint32
	ATime      uint32
	CTime      uint32
	MTime      uint32
	DTime      uint32
	GID        uint16
	Links      uint16
	Blocks512  uint32
	Flags      uint32
	_          [4]byte    // OS-specific field 1
	Direct     [12]uint32 // direct block pointers
	Indirect1  uint32
	Indirect2  uint32
	Indirect3  uint32
	_          [4]byte
	Generation uint32
	_          [8]byte // file ACL, dir ACL
	SizeHi     uint32
	_          [8]byte // padding to 128 bytes
}

// compile-time size assertion: Sizeof(DiskInode) must equal 128
var _ = [1]struct{}{}[unsafe.Sizeof(DiskInode{})-128]

// FileType returns the file type bits from Mode.
func (di *DiskInode) FileType() uint16 {
	return di.Mode & S_IFMT
}

// IsDir returns true if the inode represents a directory.
func (di *DiskInode) IsDir() bool {
	return di.FileType() == S_IFDIR
}

// IsRegular returns true if the inode represents a regular file.
func (di *DiskInode) IsRegular() bool {
	return di.FileType() == S_IFREG
}

// IsSymlink returns true if the inode represents a symbolic link.
func (di *DiskInode) IsSymlink() bool {
	return di.FileType() == S_IFLNK
}

// Size returns the full 64-bit file size.
func (di *DiskInode) Size() int64 {
	return int64(di.SizeHi)<<32 | int64(di.SizeLo)
}

// SetSize sets the full 64-bit file size.
func (di *DiskInode) SetSize(size int64) {
	di.SizeLo = uint32(size & 0xFFFFFFFF)
	di.SizeHi = uint32(size >> 32)
}

// ModeString returns a human-readable permission string like "drwxr-xr-x".
func (di *DiskInode) ModeString() string {
	m := di.Mode
	buf := make([]byte, 10)

	switch m & S_IFMT {
	case S_IFDIR:
		buf[0] = 'd'
	case S_IFLNK:
		buf[0] = 'l'
	case S_IFREG:
		buf[0] = '-'
	case S_IFBLK:
		buf[0] = 'b'
	case S_IFCHR:
		buf[0] = 'c'
	case S_IFIFO:
		buf[0] = 'p'
	case S_IFSOCK:
		buf[0] = 's'
	default:
		buf[0] = '?'
	}

	perms := []struct {
		bit  uint16
		char byte
	}{
		{S_IRUSR, 'r'}, {S_IWUSR, 'w'}, {S_IXUSR, 'x'},
		{S_IRGRP, 'r'}, {S_IWGRP, 'w'}, {S_IXGRP, 'x'},
		{S_IROTH, 'r'}, {S_IWOTH, 'w'}, {S_IXOTH, 'x'},
	}
	for i, p := range perms {
		if m&p.bit != 0 {
			buf[i+1] = p.char
		} else {
			buf[i+1] = '-'
		}
	}

	// SUID/SGID/sticky
	if m&S_ISUID != 0 {
		if buf[3] == 'x' {
			buf[3] = 's'
		} else {
			buf[3] = 'S'
		}
	}
	if m&S_ISGID != 0 {
		if buf[6] == 'x' {
			buf[6] = 's'
		} else {
			buf[6] = 'S'
		}
	}
	if m&S_ISVTX != 0 {
		if buf[9] == 'x' {
			buf[9] = 't'
		} else {
			buf[9] = 'T'
		}
	}

	return string(buf)
}

// SetTimes updates access and modification times.
func (di *DiskInode) SetTimes(atime, mtime time.Time) {
	if !atime.IsZero() {
		di.ATime = uint32(atime.Unix())
	}
	if !mtime.IsZero() {
		di.MTime = uint32(mtime.Unix())
	}
}

// InodeNumToBlock converts a 1-indexed inode number to the block address and
// byte offset within that block where the inode resides.
func InodeNumToBlock(num uint32) (blockAddr uint32, byteOffset int) {
	idx := (num - 1) * InodeSize
	blockAddr = InodeTableStart + uint32(idx/BlockSize)
	byteOffset = int(idx % BlockSize)
	return
}

// ReadInode reads a DiskInode from the inode table.
// Inode numbers are 1-indexed; inode 0 is invalid.
func ReadInode(dev *disk.BlockDevice, num uint32) (*DiskInode, error) {
	if num == 0 || num > InodeCount {
		return nil, fmt.Errorf("inode: invalid inode number %d", num)
	}
	blockAddr, offset := InodeNumToBlock(num)
	blk, err := dev.ReadBlock(blockAddr)
	if err != nil {
		return nil, fmt.Errorf("inode: read block %d: %w", blockAddr, err)
	}
	di := &DiskInode{}
	decodeInode(blk[offset:offset+InodeSize], di)
	return di, nil
}

// WriteInode writes a DiskInode to the inode table.
func WriteInode(dev *disk.BlockDevice, num uint32, di *DiskInode) error {
	if num == 0 || num > InodeCount {
		return fmt.Errorf("inode: invalid inode number %d", num)
	}
	blockAddr, offset := InodeNumToBlock(num)
	blk, err := dev.ReadBlock(blockAddr)
	if err != nil {
		return fmt.Errorf("inode: read block %d: %w", blockAddr, err)
	}
	encodeInode(di, blk[offset:offset+InodeSize])
	return dev.WriteBlock(blockAddr, blk)
}

func encodeInode(di *DiskInode, buf []byte) {
	le := binary.LittleEndian
	le.PutUint16(buf[0:], di.Mode)
	le.PutUint16(buf[2:], di.UID)
	le.PutUint32(buf[4:], di.SizeLo)
	le.PutUint32(buf[8:], di.ATime)
	le.PutUint32(buf[12:], di.CTime)
	le.PutUint32(buf[16:], di.MTime)
	le.PutUint32(buf[20:], di.DTime)
	le.PutUint16(buf[24:], di.GID)
	le.PutUint16(buf[26:], di.Links)
	le.PutUint32(buf[28:], di.Blocks512)
	le.PutUint32(buf[32:], di.Flags)
	// [4] OS-specific at offset 36 - skip (zeros)
	for i, ptr := range di.Direct {
		le.PutUint32(buf[40+i*4:], ptr)
	}
	le.PutUint32(buf[88:], di.Indirect1)
	le.PutUint32(buf[92:], di.Indirect2)
	le.PutUint32(buf[96:], di.Indirect3)
	// [4] at 100 - skip
	le.PutUint32(buf[104:], di.Generation)
	// [8] ACL at 108 - skip
	le.PutUint32(buf[116:], di.SizeHi)
	// [8] padding at 120 - already zero
}

func decodeInode(buf []byte, di *DiskInode) {
	le := binary.LittleEndian
	di.Mode = le.Uint16(buf[0:])
	di.UID = le.Uint16(buf[2:])
	di.SizeLo = le.Uint32(buf[4:])
	di.ATime = le.Uint32(buf[8:])
	di.CTime = le.Uint32(buf[12:])
	di.MTime = le.Uint32(buf[16:])
	di.DTime = le.Uint32(buf[20:])
	di.GID = le.Uint16(buf[24:])
	di.Links = le.Uint16(buf[26:])
	di.Blocks512 = le.Uint32(buf[28:])
	di.Flags = le.Uint32(buf[32:])
	for i := range di.Direct {
		di.Direct[i] = le.Uint32(buf[40+i*4:])
	}
	di.Indirect1 = le.Uint32(buf[88:])
	di.Indirect2 = le.Uint32(buf[92:])
	di.Indirect3 = le.Uint32(buf[96:])
	di.Generation = le.Uint32(buf[104:])
	di.SizeHi = le.Uint32(buf[116:])
}

// DecodeInodeBytes decodes a 128-byte inode buffer into a DiskInode.
// This is exported for block inspection use.
func DecodeInodeBytes(buf []byte) (*DiskInode, error) {
	if len(buf) < InodeSize {
		return nil, fmt.Errorf("inode: buffer too short")
	}
	di := &DiskInode{}
	decodeInode(buf[:InodeSize], di)
	return di, nil
}
