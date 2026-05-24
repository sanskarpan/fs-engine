package fs

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/yourname/fs-engine/internal/disk"
)

// Format creates a fresh filesystem on dev.
func Format(dev *disk.BlockDevice) (*DiskSuperblock, error) {
	now := uint32(time.Now().Unix())

	// Generate UUID
	var uuid [16]byte
	for i := range uuid {
		uuid[i] = byte(rand.Intn(256))
	}
	// Set version bits (version 4 UUID)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	// Build superblock
	sb := &DiskSuperblock{
		Magic:            SuperblockMagic,
		MagicV2:          SuperblockMagicV2,
		InodeCount:       InodeCount,
		BlockCount:       TotalBlocks,
		FreeInodes:       InodeCount - 1, // inode 1 reserved for root
		FreeBlocks:       TotalBlocks - DataStart,
		FirstDataBlock:   DataStart,
		BlockSize:        BlockSize,
		InodeSize:        InodeSize,
		BlocksPerGroup:   TotalBlocks,
		State:            StateClean,
		MountCount:       0,
		MaxMountCount:    20,
		LastMount:        now,
		LastWrite:        now,
		LastCheck:        now,
		InodeBitmapBlock: InodeBitmapAddr,
		BlockBitmapBlock: BlockBitmapAddr,
		InodeTableBlock:  InodeTableStart,
		JournalStart:     JournalStart,
		JournalLength:    JournalBlocks,
		Version:          1,
		UUID:             uuid,
	}
	copy(sb.VolumeName[:], "myfs")

	// Write zeroed blocks for all metadata areas
	zero := make([]byte, BlockSize)

	// Zero boot block
	if err := dev.WriteBlock(BootBlock, zero); err != nil {
		return nil, fmt.Errorf("mkfs: zero boot block: %w", err)
	}

	// Write superblock
	if err := WriteSuperblock(dev, sb); err != nil {
		return nil, fmt.Errorf("mkfs: write superblock: %w", err)
	}

	// Initialize bitmaps
	inodeBM := NewBitmap(InodeCount)
	blockBM := NewBitmap(TotalBlocks)

	// Mark system blocks as used in block bitmap
	// Blocks 0 to DataStart-1 are all metadata
	for i := uint32(0); i < DataStart; i++ {
		blockBM.Set(i)
	}

	// Reserve inode 1 for root
	inodeBM.Set(0) // inode 1 = bitmap index 0

	if err := WriteInodeBitmap(dev, inodeBM); err != nil {
		return nil, fmt.Errorf("mkfs: write inode bitmap: %w", err)
	}
	if err := WriteBlockBitmap(dev, blockBM); err != nil {
		return nil, fmt.Errorf("mkfs: write block bitmap: %w", err)
	}

	// Zero inode table
	for i := uint32(0); i < InodeTableBlocks; i++ {
		if err := dev.WriteBlock(InodeTableStart+i, zero); err != nil {
			return nil, fmt.Errorf("mkfs: zero inode table block %d: %w", i, err)
		}
	}

	// Zero journal
	for i := uint32(0); i < JournalBlocks; i++ {
		if err := dev.WriteBlock(JournalStart+i, zero); err != nil {
			return nil, fmt.Errorf("mkfs: zero journal block %d: %w", i, err)
		}
	}

	// Create root directory (inode 1)
	alloc := NewAllocator(dev, sb, blockBM, inodeBM, nil)

	// Allocate first block for root directory
	rootBlk, err := alloc.AllocBlock()
	if err != nil {
		return nil, fmt.Errorf("mkfs: alloc root block: %w", err)
	}

	rootInode := &DiskInode{
		Mode:      S_IFDIR | 0755,
		UID:       0,
		GID:       0,
		Links:     2, // "." and parent ref
		Blocks512: BlockSize / 512,
		ATime:     now,
		CTime:     now,
		MTime:     now,
	}
	rootInode.Direct[0] = rootBlk
	rootInode.SetSize(BlockSize)

	if err := WriteInode(dev, 1, rootInode); err != nil {
		return nil, fmt.Errorf("mkfs: write root inode: %w", err)
	}

	// Initialize root directory block with "." and ".."
	if err := InitDirBlock(dev, rootInode, 1, 1); err != nil {
		return nil, fmt.Errorf("mkfs: init root dir block: %w", err)
	}

	// Write final superblock
	if err := WriteSuperblock(dev, sb); err != nil {
		return nil, fmt.Errorf("mkfs: write final superblock: %w", err)
	}

	// Seed default directory tree
	if err := seedFS(dev, alloc, sb); err != nil {
		return nil, fmt.Errorf("mkfs: seed: %w", err)
	}

	return sb, nil
}

// seedFS creates the default directory structure.
func seedFS(dev *disk.BlockDevice, alloc *Allocator, sb *DiskSuperblock) error {
	now := uint32(time.Now().Unix())

	// Directory tree to create under root
	dirs := []string{
		"bin", "etc", "home", "tmp", "usr", "var",
		"lib", "dev", "proc", "sys", "mnt", "opt",
		"usr/bin", "usr/lib", "usr/local",
		"home/user",
		"var/log", "var/tmp",
		"etc/init.d",
	}

	for _, path := range dirs {
		if err := mkdirPath(dev, alloc, sb, path, now); err != nil {
			return fmt.Errorf("seed mkdir %s: %w", path, err)
		}
	}

	// Create some sample files
	files := []struct {
		path    string
		content string
	}{
		{"etc/hostname", "fsengine\n"},
		{"etc/version", "fs-engine v1.0\n"},
		{"home/user/readme.txt", "Welcome to fs-engine!\n"},
	}

	for _, f := range files {
		if err := createFile(dev, alloc, sb, f.path, f.content, now); err != nil {
			return fmt.Errorf("seed file %s: %w", f.path, err)
		}
	}

	// Persist allocator-updated free counts after seeding files and directories.
	return WriteSuperblock(dev, sb)
}

// mkdirPath creates a directory (and all missing parents) at path relative to root (inode 1).
// path must not contain leading slash.
func mkdirPath(dev *disk.BlockDevice, alloc *Allocator, sb *DiskSuperblock, path string, now uint32) error {
	// Split path into components
	parts := splitPath(path)
	if len(parts) == 0 {
		return nil
	}

	parentInum := uint32(1)
	for _, part := range parts {
		parentInode, err := ReadInode(dev, parentInum)
		if err != nil {
			return err
		}

		// Check if already exists
		childInum, _, err := LookupEntry(dev, parentInode, part)
		if err == nil {
			parentInum = childInum
			continue
		}
		if err != ErrNotFound {
			return err
		}

		// Create this component
		newInum, err := alloc.AllocInode()
		if err != nil {
			return err
		}

		dirBlk, err := alloc.AllocBlock()
		if err != nil {
			return err
		}

		newInode := &DiskInode{
			Mode:      S_IFDIR | 0755,
			UID:       0,
			GID:       0,
			Links:     2,
			Blocks512: BlockSize / 512,
			ATime:     now,
			CTime:     now,
			MTime:     now,
		}
		newInode.Direct[0] = dirBlk
		newInode.SetSize(BlockSize)

		if err := WriteInode(dev, newInum, newInode); err != nil {
			return err
		}
		if err := InitDirBlock(dev, newInode, newInum, parentInum); err != nil {
			return err
		}

		// Add entry to parent
		parentInode.Links++
		if err := AddEntry(dev, alloc, parentInode, part, newInum, FT_DIR); err != nil {
			return err
		}
		if err := WriteInode(dev, parentInum, parentInode); err != nil {
			return err
		}

		parentInum = newInum
	}

	return WriteSuperblock(dev, sb)
}

// createFile creates a regular file at path relative to root with given content.
// Intermediate directories are created if they don't exist.
func createFile(dev *disk.BlockDevice, alloc *Allocator, sb *DiskSuperblock, path string, content string, now uint32) error {
	parts := splitPath(path)
	if len(parts) == 0 {
		return ErrInvalidArg
	}

	// Ensure parent directories exist
	if len(parts) > 1 {
		parentPath := ""
		for _, p := range parts[:len(parts)-1] {
			if parentPath == "" {
				parentPath = p
			} else {
				parentPath = parentPath + "/" + p
			}
		}
		if err := mkdirPath(dev, alloc, sb, parentPath, now); err != nil {
			return err
		}
	}

	// Find parent directory
	parentInum := uint32(1)
	for _, part := range parts[:len(parts)-1] {
		parentInode, err := ReadInode(dev, parentInum)
		if err != nil {
			return err
		}
		childInum, _, err := LookupEntry(dev, parentInode, part)
		if err != nil {
			return fmt.Errorf("createFile: parent %s: %w", part, err)
		}
		parentInum = childInum
	}

	name := parts[len(parts)-1]
	parentInode, err := ReadInode(dev, parentInum)
	if err != nil {
		return err
	}

	// Allocate inode
	newInum, err := alloc.AllocInode()
	if err != nil {
		return err
	}

	newInode := &DiskInode{
		Mode:  S_IFREG | 0644,
		UID:   0,
		GID:   0,
		Links: 1,
		ATime: now,
		CTime: now,
		MTime: now,
	}

	// Write content
	data := []byte(content)
	if len(data) > 0 {
		blkAddr, err := alloc.AllocBlock()
		if err != nil {
			return err
		}
		blk := make([]byte, BlockSize)
		copy(blk, data)
		if err := dev.WriteBlock(blkAddr, blk); err != nil {
			return err
		}
		newInode.Direct[0] = blkAddr
		newInode.Blocks512 = BlockSize / 512
		newInode.SetSize(int64(len(data)))
	}

	if err := WriteInode(dev, newInum, newInode); err != nil {
		return err
	}

	// Add entry to parent
	if err := AddEntry(dev, alloc, parentInode, name, newInum, FT_REG_FILE); err != nil {
		return err
	}
	return WriteInode(dev, parentInum, parentInode)
}

// splitPath splits a slash-separated path into components, filtering empty parts.
func splitPath(path string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if i > start {
				parts = append(parts, path[start:i])
			}
			start = i + 1
		}
	}
	return parts
}
