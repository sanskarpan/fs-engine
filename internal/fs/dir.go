package fs

import (
	"encoding/binary"
	"fmt"

	"github.com/yourname/fs-engine/internal/disk"
)

const (
	// DirEntryMinSize is the minimum directory entry size (ino+reclen+namelen+filetype).
	DirEntryMinSize = 8
	// MaxNameLen is the maximum file name length.
	MaxNameLen = 255
)

// DirEntry represents a parsed directory entry.
type DirEntry struct {
	Inode    uint32
	RecLen   uint16
	NameLen  uint8
	FileType uint8
	Name     string
}

// align4 rounds up to the nearest multiple of 4.
func align4(n int) int { return (n + 3) &^ 3 }

// DirEntrySize returns the minimum actual size of a directory entry with the given name length.
func DirEntrySize(nameLen int) int {
	return align4(DirEntryMinSize + nameLen)
}

// ParseDirEntry parses a single directory entry from buf.
// Returns the entry and the number of bytes consumed (== RecLen).
func ParseDirEntry(buf []byte) (DirEntry, int, error) {
	if len(buf) < DirEntryMinSize {
		return DirEntry{}, 0, fmt.Errorf("dir: buffer too short for entry header")
	}
	le := binary.LittleEndian
	ino := le.Uint32(buf[0:])
	recLen := le.Uint16(buf[4:])
	nameLen := buf[6]
	fileType := buf[7]

	if int(recLen) < DirEntryMinSize || int(recLen) > len(buf) {
		return DirEntry{}, 0, fmt.Errorf("dir: invalid recLen %d (buf=%d)", recLen, len(buf))
	}
	if int(nameLen) > int(recLen)-DirEntryMinSize {
		return DirEntry{}, 0, fmt.Errorf("dir: nameLen %d exceeds recLen %d", nameLen, recLen)
	}

	name := string(buf[DirEntryMinSize : DirEntryMinSize+int(nameLen)])
	return DirEntry{
		Inode:    ino,
		RecLen:   recLen,
		NameLen:  nameLen,
		FileType: fileType,
		Name:     name,
	}, int(recLen), nil
}

// encodeDirEntry writes a directory entry into buf.
func encodeDirEntry(de DirEntry, buf []byte) {
	le := binary.LittleEndian
	le.PutUint32(buf[0:], de.Inode)
	le.PutUint16(buf[4:], de.RecLen)
	buf[6] = de.NameLen
	buf[7] = de.FileType
	copy(buf[DirEntryMinSize:], []byte(de.Name))
}

// IterDirEntries iterates over all directory entries in a directory inode,
// calling fn for each non-deleted entry. If fn returns false, iteration stops.
func IterDirEntries(dev *disk.BlockDevice, di *DiskInode, fn func(de DirEntry, blockIdx uint32, offset int) bool) error {
	size := di.Size()
	if size == 0 {
		return nil
	}

	numBlocks := (size + BlockSize - 1) / BlockSize
	for b := uint32(0); b < uint32(numBlocks); b++ {
		physBlk, err := GetBlockForOffset(dev, di, b)
		if err != nil {
			return fmt.Errorf("dir: get block for offset %d: %w", b, err)
		}
		if physBlk == 0 {
			continue
		}
		blk, err := dev.ReadBlock(physBlk)
		if err != nil {
			return fmt.Errorf("dir: read block %d: %w", physBlk, err)
		}

		off := 0
		for off+DirEntryMinSize <= BlockSize {
			de, consumed, err := ParseDirEntry(blk[off:])
			if err != nil {
				break
			}
			if consumed == 0 {
				break
			}
			if de.Inode != 0 {
				if !fn(de, b, off) {
					return nil
				}
			}
			off += consumed
		}
	}
	return nil
}

// LookupEntry searches a directory for an entry with the given name.
// Returns the inode number and file type, or ErrNotFound.
func LookupEntry(dev *disk.BlockDevice, di *DiskInode, name string) (uint32, uint8, error) {
	if !di.IsDir() {
		return 0, 0, ErrNotDir
	}
	var found uint32
	var foundFT uint8
	var foundErr error
	err := IterDirEntries(dev, di, func(de DirEntry, _ uint32, _ int) bool {
		if de.Name == name {
			found = de.Inode
			foundFT = de.FileType
			return false
		}
		return true
	})
	if err != nil {
		return 0, 0, err
	}
	if found == 0 {
		foundErr = ErrNotFound
	}
	return found, foundFT, foundErr
}

// AddEntry adds a directory entry to a directory inode.
// Strategy (ext2-compatible):
// 1. Search existing blocks for a slot where recLen can be split to fit the new entry.
// 2. Allocate a new block if no space found.
// Directory size is always a multiple of BlockSize.
func AddEntry(dev *disk.BlockDevice, alloc *Allocator, di *DiskInode, name string, inum uint32, fileType uint8) error {
	if len(name) > MaxNameLen {
		return ErrNameTooLong
	}
	if !di.IsDir() {
		return ErrNotDir
	}

	needed := DirEntrySize(len(name))

	// Scan existing blocks for a slot that can be split
	size := di.Size()
	numBlocks := (size + BlockSize - 1) / BlockSize

	for b := uint32(0); b < uint32(numBlocks); b++ {
		physBlk, err := GetBlockForOffset(dev, di, b)
		if err != nil {
			return err
		}
		if physBlk == 0 {
			continue
		}
		blk, err := dev.ReadBlock(physBlk)
		if err != nil {
			return err
		}

		off := 0
		for off+DirEntryMinSize <= BlockSize {
			de, _, err := ParseDirEntry(blk[off:])
			if err != nil {
				break
			}
			if de.RecLen == 0 {
				break
			}

			// Can we fit here?
			if de.Inode == 0 {
				// Deleted slot - reuse directly if it fits
				if int(de.RecLen) >= needed {
					newDE := DirEntry{
						Inode:    inum,
						RecLen:   de.RecLen,
						NameLen:  uint8(len(name)),
						FileType: fileType,
						Name:     name,
					}
					encodeDirEntry(newDE, blk[off:])
					return dev.WriteBlock(physBlk, blk)
				}
			} else {
				// Active entry - check if we can split its trailing space
				minSize := DirEntrySize(int(de.NameLen))
				spare := int(de.RecLen) - minSize
				if spare >= needed {
					// Shrink this entry
					binary.LittleEndian.PutUint16(blk[off+4:], uint16(minSize))
					// Write new entry in the spare space
					newOff := off + minSize
					newDE := DirEntry{
						Inode:    inum,
						RecLen:   uint16(spare),
						NameLen:  uint8(len(name)),
						FileType: fileType,
						Name:     name,
					}
					encodeDirEntry(newDE, blk[newOff:])
					return dev.WriteBlock(physBlk, blk)
				}
			}
			off += int(de.RecLen)
		}
	}

	// No space found - allocate a new block
	newPhys, err := alloc.AllocBlock()
	if err != nil {
		return err
	}

	blk := make([]byte, BlockSize)
	newDE := DirEntry{
		Inode:    inum,
		RecLen:   uint16(BlockSize), // spans the entire new block
		NameLen:  uint8(len(name)),
		FileType: fileType,
		Name:     name,
	}
	encodeDirEntry(newDE, blk)

	if err := dev.WriteBlock(newPhys, blk); err != nil {
		return err
	}

	// Map this block into the inode
	logicalBlock := uint32(numBlocks)
	if err := setBlockForOffset(dev, alloc, di, logicalBlock, newPhys); err != nil {
		return err
	}

	// Update directory size
	di.SetSize(int64(logicalBlock+1) * BlockSize)
	return nil
}

// setBlockForOffset sets an already-allocated block as the mapping for the logical block.
func setBlockForOffset(dev *disk.BlockDevice, alloc *Allocator, di *DiskInode, logicalBlock uint32, physBlock uint32) error {
	const direct = 12
	const ptrs = PointersPerBlock

	if logicalBlock < direct {
		di.Direct[logicalBlock] = physBlock
		di.Blocks512 += BlockSize / 512
		return nil
	}
	lb := logicalBlock - direct

	if lb < ptrs {
		if di.Indirect1 == 0 {
			ind, err := alloc.AllocBlock()
			if err != nil {
				return err
			}
			di.Indirect1 = ind
			di.Blocks512 += BlockSize / 512
		}
		blk, err := dev.ReadBlock(di.Indirect1)
		if err != nil {
			return err
		}
		writePointer(blk, int(lb), physBlock)
		di.Blocks512 += BlockSize / 512
		return dev.WriteBlock(di.Indirect1, blk)
	}
	return ErrTooBig
}

// RemoveEntry removes a directory entry by name (marks its inode as 0).
func RemoveEntry(dev *disk.BlockDevice, di *DiskInode, name string) error {
	if !di.IsDir() {
		return ErrNotDir
	}

	size := di.Size()
	numBlocks := (size + BlockSize - 1) / BlockSize

	for b := uint32(0); b < uint32(numBlocks); b++ {
		physBlk, err := GetBlockForOffset(dev, di, b)
		if err != nil {
			return err
		}
		if physBlk == 0 {
			continue
		}
		blk, err := dev.ReadBlock(physBlk)
		if err != nil {
			return err
		}

		off := 0
		for off+DirEntryMinSize <= BlockSize {
			de, _, err := ParseDirEntry(blk[off:])
			if err != nil {
				break
			}
			if de.RecLen == 0 {
				break
			}
			if de.Inode != 0 && de.Name == name {
				// Mark entry as deleted by zeroing the inode number
				binary.LittleEndian.PutUint32(blk[off:], 0)
				return dev.WriteBlock(physBlk, blk)
			}
			off += int(de.RecLen)
		}
	}

	return ErrNotFound
}

// IsEmptyDir returns true if the directory contains only "." and ".." entries.
func IsEmptyDir(dev *disk.BlockDevice, di *DiskInode) (bool, error) {
	count := 0
	err := IterDirEntries(dev, di, func(de DirEntry, _ uint32, _ int) bool {
		if de.Name != "." && de.Name != ".." {
			count++
			return false
		}
		return true
	})
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// ReadAllEntries returns all non-deleted directory entries.
func ReadAllEntries(dev *disk.BlockDevice, di *DiskInode) ([]DirEntry, error) {
	var entries []DirEntry
	err := IterDirEntries(dev, di, func(de DirEntry, _ uint32, _ int) bool {
		entries = append(entries, de)
		return true
	})
	return entries, err
}

// InitDirBlock initialises the first block of a new directory with "." and ".." entries.
func InitDirBlock(dev *disk.BlockDevice, di *DiskInode, dirInum, parentInum uint32) error {
	if di.Direct[0] == 0 {
		return fmt.Errorf("dir: no first block allocated")
	}

	blk := make([]byte, BlockSize)
	offset := 0

	// "." entry - minimum size
	dotSize := DirEntrySize(1)
	dot := DirEntry{
		Inode:    dirInum,
		RecLen:   uint16(dotSize),
		NameLen:  1,
		FileType: FT_DIR,
		Name:     ".",
	}
	encodeDirEntry(dot, blk[offset:])
	offset += dotSize

	// ".." entry - takes remaining space in the block
	dotdot := DirEntry{
		Inode:    parentInum,
		RecLen:   uint16(BlockSize - offset),
		NameLen:  2,
		FileType: FT_DIR,
		Name:     "..",
	}
	encodeDirEntry(dotdot, blk[offset:])

	return dev.WriteBlock(di.Direct[0], blk)
}

// ParseDirBlock parses all directory entries from a raw block of data.
// Used for block inspection.
func ParseDirBlock(data []byte) ([]map[string]interface{}, error) {
	var entries []map[string]interface{}
	offset := 0
	for offset < len(data) && offset+DirEntryMinSize <= len(data) {
		de, consumed, err := ParseDirEntry(data[offset:])
		if err != nil || consumed <= 0 {
			break
		}
		entry := map[string]interface{}{
			"inode":     de.Inode,
			"rec_len":   de.RecLen,
			"name_len":  de.NameLen,
			"file_type": de.FileType,
			"name":      de.Name,
		}
		entries = append(entries, entry)
		offset += consumed
	}
	return entries, nil
}
