package fs

import (
	"encoding/binary"
	"fmt"

	"github.com/yourname/fs-engine/internal/disk"
)

// GetBlockForOffset returns the physical block address for the logical block
// at offset (0-indexed) in the file, without allocating.
// Returns 0 if the block is a hole (sparse file).
func GetBlockForOffset(dev *disk.BlockDevice, di *DiskInode, logicalBlock uint32) (uint32, error) {
	const direct = 12
	const ptrs = PointersPerBlock

	if logicalBlock < direct {
		return di.Direct[logicalBlock], nil
	}

	logicalBlock -= direct

	// Single indirect
	if logicalBlock < ptrs {
		if di.Indirect1 == 0 {
			return 0, nil
		}
		blk, err := dev.ReadBlock(di.Indirect1)
		if err != nil {
			return 0, fmt.Errorf("extent: read indirect1 block: %w", err)
		}
		return readPointer(blk, int(logicalBlock)), nil
	}
	logicalBlock -= ptrs

	// Double indirect
	if logicalBlock < ptrs*ptrs {
		if di.Indirect2 == 0 {
			return 0, nil
		}
		blk, err := dev.ReadBlock(di.Indirect2)
		if err != nil {
			return 0, fmt.Errorf("extent: read indirect2 block: %w", err)
		}
		l2 := readPointer(blk, int(logicalBlock/ptrs))
		if l2 == 0 {
			return 0, nil
		}
		blk2, err := dev.ReadBlock(l2)
		if err != nil {
			return 0, fmt.Errorf("extent: read indirect2 leaf: %w", err)
		}
		return readPointer(blk2, int(logicalBlock%ptrs)), nil
	}
	logicalBlock -= ptrs * ptrs

	// Triple indirect
	if logicalBlock < ptrs*ptrs*ptrs {
		if di.Indirect3 == 0 {
			return 0, nil
		}
		blk, err := dev.ReadBlock(di.Indirect3)
		if err != nil {
			return 0, fmt.Errorf("extent: read indirect3 block: %w", err)
		}
		l2idx := logicalBlock / (ptrs * ptrs)
		l2 := readPointer(blk, int(l2idx))
		if l2 == 0 {
			return 0, nil
		}
		blk2, err := dev.ReadBlock(l2)
		if err != nil {
			return 0, fmt.Errorf("extent: read indirect3 l2: %w", err)
		}
		l3idx := (logicalBlock / ptrs) % ptrs
		l3 := readPointer(blk2, int(l3idx))
		if l3 == 0 {
			return 0, nil
		}
		blk3, err := dev.ReadBlock(l3)
		if err != nil {
			return 0, fmt.Errorf("extent: read indirect3 leaf: %w", err)
		}
		return readPointer(blk3, int(logicalBlock%ptrs)), nil
	}

	return 0, ErrTooBig
}

// AllocBlockForOffset allocates a block for the logical position in the inode,
// allocating any necessary indirect blocks along the way.
func AllocBlockForOffset(dev *disk.BlockDevice, alloc *Allocator, di *DiskInode, logicalBlock uint32) (uint32, error) {
	const direct = 12
	const ptrs = PointersPerBlock

	// Check existing mapping first
	existing, err := GetBlockForOffset(dev, di, logicalBlock)
	if err != nil {
		return 0, err
	}
	if existing != 0 {
		return existing, nil
	}

	if logicalBlock < direct {
		newBlk, err := alloc.AllocBlock()
		if err != nil {
			return 0, err
		}
		di.Direct[logicalBlock] = newBlk
		di.Blocks512 += BlockSize / 512
		return newBlk, nil
	}

	logicalBlock -= direct

	// Single indirect
	if logicalBlock < ptrs {
		if di.Indirect1 == 0 {
			ind, err := alloc.AllocBlock()
			if err != nil {
				return 0, err
			}
			di.Indirect1 = ind
			di.Blocks512 += BlockSize / 512
		}
		blk, err := dev.ReadBlock(di.Indirect1)
		if err != nil {
			return 0, err
		}
		newBlk, err := alloc.AllocBlock()
		if err != nil {
			return 0, err
		}
		writePointer(blk, int(logicalBlock), newBlk)
		if err := dev.WriteBlock(di.Indirect1, blk); err != nil {
			return 0, err
		}
		di.Blocks512 += BlockSize / 512
		return newBlk, nil
	}
	logicalBlock -= ptrs

	// Double indirect
	if logicalBlock < ptrs*ptrs {
		if di.Indirect2 == 0 {
			ind, err := alloc.AllocBlock()
			if err != nil {
				return 0, err
			}
			di.Indirect2 = ind
			di.Blocks512 += BlockSize / 512
		}
		blk, err := dev.ReadBlock(di.Indirect2)
		if err != nil {
			return 0, err
		}
		l2idx := int(logicalBlock / ptrs)
		l2 := readPointer(blk, l2idx)
		if l2 == 0 {
			l2, err = alloc.AllocBlock()
			if err != nil {
				return 0, err
			}
			writePointer(blk, l2idx, l2)
			if err := dev.WriteBlock(di.Indirect2, blk); err != nil {
				return 0, err
			}
			di.Blocks512 += BlockSize / 512
		}
		blk2, err := dev.ReadBlock(l2)
		if err != nil {
			return 0, err
		}
		leafIdx := int(logicalBlock % ptrs)
		newBlk, err := alloc.AllocBlock()
		if err != nil {
			return 0, err
		}
		writePointer(blk2, leafIdx, newBlk)
		if err := dev.WriteBlock(l2, blk2); err != nil {
			return 0, err
		}
		di.Blocks512 += BlockSize / 512
		return newBlk, nil
	}
	logicalBlock -= ptrs * ptrs

	// Triple indirect
	if logicalBlock < ptrs*ptrs*ptrs {
		if di.Indirect3 == 0 {
			ind, err := alloc.AllocBlock()
			if err != nil {
				return 0, err
			}
			di.Indirect3 = ind
			di.Blocks512 += BlockSize / 512
		}
		blk, err := dev.ReadBlock(di.Indirect3)
		if err != nil {
			return 0, err
		}
		l2idx := int(logicalBlock / (ptrs * ptrs))
		l2 := readPointer(blk, l2idx)
		if l2 == 0 {
			l2, err = alloc.AllocBlock()
			if err != nil {
				return 0, err
			}
			writePointer(blk, l2idx, l2)
			if err := dev.WriteBlock(di.Indirect3, blk); err != nil {
				return 0, err
			}
			di.Blocks512 += BlockSize / 512
		}
		blk2, err := dev.ReadBlock(l2)
		if err != nil {
			return 0, err
		}
		l3idx := int((logicalBlock / ptrs) % ptrs)
		l3 := readPointer(blk2, l3idx)
		if l3 == 0 {
			l3, err = alloc.AllocBlock()
			if err != nil {
				return 0, err
			}
			writePointer(blk2, l3idx, l3)
			if err := dev.WriteBlock(l2, blk2); err != nil {
				return 0, err
			}
			di.Blocks512 += BlockSize / 512
		}
		blk3, err := dev.ReadBlock(l3)
		if err != nil {
			return 0, err
		}
		leafIdx := int(logicalBlock % ptrs)
		newBlk, err := alloc.AllocBlock()
		if err != nil {
			return 0, err
		}
		writePointer(blk3, leafIdx, newBlk)
		if err := dev.WriteBlock(l3, blk3); err != nil {
			return 0, err
		}
		di.Blocks512 += BlockSize / 512
		return newBlk, nil
	}

	return 0, ErrTooBig
}

// FreeBlocksFrom frees all blocks at or beyond logical block start.
func FreeBlocksFrom(dev *disk.BlockDevice, alloc *Allocator, di *DiskInode, startLogical uint32) error {
	totalBlocks := blocksForSize(di.Size())
	if startLogical >= totalBlocks {
		return nil
	}

	for lb := startLogical; lb < totalBlocks; lb++ {
		phys, err := GetBlockForOffset(dev, di, lb)
		if err != nil {
			return err
		}
		if phys != 0 {
			if err := alloc.FreeBlock(phys); err != nil {
				return err
			}
			if err := clearPointerAt(dev, alloc, di, lb); err != nil {
				return err
			}
		}
	}

	// Free now-empty indirect blocks
	return pruneIndirectBlocks(dev, alloc, di, startLogical)
}

// BlockList returns all physical block addresses for the file.
func BlockList(dev *disk.BlockDevice, di *DiskInode) ([]uint32, error) {
	total := blocksForSize(di.Size())
	result := make([]uint32, 0, total)
	for lb := uint32(0); lb < total; lb++ {
		phys, err := GetBlockForOffset(dev, di, lb)
		if err != nil {
			return nil, err
		}
		result = append(result, phys)
	}
	return result, nil
}

func blocksForSize(size int64) uint32 {
	if size == 0 {
		return 0
	}
	return uint32((size + BlockSize - 1) / BlockSize)
}

func readPointer(blk []byte, idx int) uint32 {
	return binary.LittleEndian.Uint32(blk[idx*4:])
}

func writePointer(blk []byte, idx int, val uint32) {
	binary.LittleEndian.PutUint32(blk[idx*4:], val)
}

func clearPointerAt(dev *disk.BlockDevice, alloc *Allocator, di *DiskInode, lb uint32) error {
	const direct = 12
	const ptrs = PointersPerBlock

	if lb < direct {
		di.Direct[lb] = 0
		return nil
	}
	lb -= direct

	if lb < ptrs {
		if di.Indirect1 == 0 {
			return nil
		}
		blk, err := dev.ReadBlock(di.Indirect1)
		if err != nil {
			return err
		}
		writePointer(blk, int(lb), 0)
		return dev.WriteBlock(di.Indirect1, blk)
	}
	lb -= ptrs

	if lb < ptrs*ptrs {
		if di.Indirect2 == 0 {
			return nil
		}
		blk, err := dev.ReadBlock(di.Indirect2)
		if err != nil {
			return err
		}
		l2 := readPointer(blk, int(lb/ptrs))
		if l2 == 0 {
			return nil
		}
		blk2, err := dev.ReadBlock(l2)
		if err != nil {
			return err
		}
		writePointer(blk2, int(lb%ptrs), 0)
		return dev.WriteBlock(l2, blk2)
	}
	lb -= ptrs * ptrs

	if lb < ptrs*ptrs*ptrs {
		if di.Indirect3 == 0 {
			return nil
		}
		blk, err := dev.ReadBlock(di.Indirect3)
		if err != nil {
			return err
		}
		l2 := readPointer(blk, int(lb/(ptrs*ptrs)))
		if l2 == 0 {
			return nil
		}
		blk2, err := dev.ReadBlock(l2)
		if err != nil {
			return err
		}
		l3 := readPointer(blk2, int((lb/ptrs)%ptrs))
		if l3 == 0 {
			return nil
		}
		blk3, err := dev.ReadBlock(l3)
		if err != nil {
			return err
		}
		writePointer(blk3, int(lb%ptrs), 0)
		return dev.WriteBlock(l3, blk3)
	}
	return nil
}

func pruneIndirectBlocks(dev *disk.BlockDevice, alloc *Allocator, di *DiskInode, startLogical uint32) error {
	// Simple approach: check if indirect blocks are now fully empty and free them
	if di.Indirect1 != 0 {
		blk, err := dev.ReadBlock(di.Indirect1)
		if err != nil {
			return err
		}
		empty := true
		for i := 0; i < PointersPerBlock; i++ {
			if readPointer(blk, i) != 0 {
				empty = false
				break
			}
		}
		if empty && startLogical <= 12 {
			if err := alloc.FreeBlock(di.Indirect1); err != nil {
				return err
			}
			di.Indirect1 = 0
		}
	}
	return nil
}
