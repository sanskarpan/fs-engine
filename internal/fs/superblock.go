package fs

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/yourname/fs-engine/internal/disk"
)

// DiskSuperblock is the on-disk superblock structure, exactly 4096 bytes.
type DiskSuperblock struct {
	Magic            uint32
	InodeCount       uint32
	BlockCount       uint32
	FreeInodes       uint32
	FreeBlocks       uint32
	FirstDataBlock   uint32
	BlockSize        uint32
	InodeSize        uint16
	_                [2]byte
	BlocksPerGroup   uint32
	MagicV2          uint32
	State            uint16
	MountCount       uint16
	MaxMountCount    uint16
	_                [2]byte
	LastMount        uint32
	LastWrite        uint32
	LastCheck        uint32
	InodeBitmapBlock uint32
	BlockBitmapBlock uint32
	InodeTableBlock  uint32
	JournalStart     uint32
	JournalLength    uint32
	Version          uint32
	UUID             [16]byte
	VolumeName       [16]byte
	_                [3980]byte
}

// compile-time size assertion
var _ = [1]struct{}{}[unsafe.Sizeof(DiskSuperblock{})-4096]

// ReadSuperblock reads the superblock from block SuperblockAddr.
func ReadSuperblock(dev *disk.BlockDevice) (*DiskSuperblock, error) {
	blk, err := dev.ReadBlock(SuperblockAddr)
	if err != nil {
		return nil, fmt.Errorf("superblock: read block: %w", err)
	}

	sb := &DiskSuperblock{}
	if err := decodeSuperblock(blk, sb); err != nil {
		return nil, err
	}
	return sb, nil
}

// WriteSuperblock writes the superblock to block SuperblockAddr.
func WriteSuperblock(dev *disk.BlockDevice, sb *DiskSuperblock) error {
	blk := make([]byte, BlockSize)
	encodeSuperblock(sb, blk)
	return dev.WriteBlock(SuperblockAddr, blk)
}

// Validate checks the superblock magic numbers and basic sanity.
func (sb *DiskSuperblock) Validate() error {
	if sb.Magic != SuperblockMagic {
		return fmt.Errorf("superblock: bad magic 0x%X (want 0x%X): %w", sb.Magic, SuperblockMagic, ErrBadFS)
	}
	if sb.MagicV2 != SuperblockMagicV2 {
		return fmt.Errorf("superblock: bad magic v2 0x%X (want 0x%X): %w", sb.MagicV2, SuperblockMagicV2, ErrBadFS)
	}
	if sb.BlockSize != BlockSize {
		return fmt.Errorf("superblock: unexpected block size %d: %w", sb.BlockSize, ErrBadFS)
	}
	if sb.InodeSize != InodeSize {
		return fmt.Errorf("superblock: unexpected inode size %d: %w", sb.InodeSize, ErrBadFS)
	}
	return nil
}

// encodeSuperblock serialises the struct into buf using little-endian byte order.
// We use explicit field encoding so padding bytes stay zero.
func encodeSuperblock(sb *DiskSuperblock, buf []byte) {
	le := binary.LittleEndian
	le.PutUint32(buf[0:], sb.Magic)
	le.PutUint32(buf[4:], sb.InodeCount)
	le.PutUint32(buf[8:], sb.BlockCount)
	le.PutUint32(buf[12:], sb.FreeInodes)
	le.PutUint32(buf[16:], sb.FreeBlocks)
	le.PutUint32(buf[20:], sb.FirstDataBlock)
	le.PutUint32(buf[24:], sb.BlockSize)
	le.PutUint16(buf[28:], sb.InodeSize)
	// [2] padding at 30
	le.PutUint32(buf[32:], sb.BlocksPerGroup)
	le.PutUint32(buf[36:], sb.MagicV2)
	le.PutUint16(buf[40:], sb.State)
	le.PutUint16(buf[42:], sb.MountCount)
	le.PutUint16(buf[44:], sb.MaxMountCount)
	// [2] padding at 46
	le.PutUint32(buf[48:], sb.LastMount)
	le.PutUint32(buf[52:], sb.LastWrite)
	le.PutUint32(buf[56:], sb.LastCheck)
	le.PutUint32(buf[60:], sb.InodeBitmapBlock)
	le.PutUint32(buf[64:], sb.BlockBitmapBlock)
	le.PutUint32(buf[68:], sb.InodeTableBlock)
	le.PutUint32(buf[72:], sb.JournalStart)
	le.PutUint32(buf[76:], sb.JournalLength)
	le.PutUint32(buf[80:], sb.Version)
	copy(buf[84:100], sb.UUID[:])
	copy(buf[100:116], sb.VolumeName[:])
	// rest is zero padding
}

func decodeSuperblock(buf []byte, sb *DiskSuperblock) error {
	if len(buf) < BlockSize {
		return fmt.Errorf("superblock: short buffer %d", len(buf))
	}
	le := binary.LittleEndian
	sb.Magic = le.Uint32(buf[0:])
	sb.InodeCount = le.Uint32(buf[4:])
	sb.BlockCount = le.Uint32(buf[8:])
	sb.FreeInodes = le.Uint32(buf[12:])
	sb.FreeBlocks = le.Uint32(buf[16:])
	sb.FirstDataBlock = le.Uint32(buf[20:])
	sb.BlockSize = le.Uint32(buf[24:])
	sb.InodeSize = le.Uint16(buf[28:])
	sb.BlocksPerGroup = le.Uint32(buf[32:])
	sb.MagicV2 = le.Uint32(buf[36:])
	sb.State = le.Uint16(buf[40:])
	sb.MountCount = le.Uint16(buf[42:])
	sb.MaxMountCount = le.Uint16(buf[44:])
	sb.LastMount = le.Uint32(buf[48:])
	sb.LastWrite = le.Uint32(buf[52:])
	sb.LastCheck = le.Uint32(buf[56:])
	sb.InodeBitmapBlock = le.Uint32(buf[60:])
	sb.BlockBitmapBlock = le.Uint32(buf[64:])
	sb.InodeTableBlock = le.Uint32(buf[68:])
	sb.JournalStart = le.Uint32(buf[72:])
	sb.JournalLength = le.Uint32(buf[76:])
	sb.Version = le.Uint32(buf[80:])
	copy(sb.UUID[:], buf[84:100])
	copy(sb.VolumeName[:], buf[100:116])
	return nil
}
