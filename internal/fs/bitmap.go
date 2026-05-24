package fs

import (
	"fmt"
	"math/bits"

	"github.com/yourname/fs-engine/internal/disk"
)

// Bitmap is an in-memory bit array.
type Bitmap struct {
	bits []byte
	size uint32 // total number of bits
}

// NewBitmap creates a new bitmap for n bits.
func NewBitmap(n uint32) *Bitmap {
	return &Bitmap{
		bits: make([]byte, (n+7)/8),
		size: n,
	}
}

// NewBitmapFromBytes wraps existing byte data.
func NewBitmapFromBytes(data []byte, n uint32) *Bitmap {
	b := &Bitmap{
		bits: make([]byte, len(data)),
		size: n,
	}
	copy(b.bits, data)
	return b
}

// IsSet returns true if bit n is set.
func (b *Bitmap) IsSet(n uint32) bool {
	if n >= b.size {
		return false
	}
	return b.bits[n/8]&(1<<(n%8)) != 0
}

// Set sets bit n.
func (b *Bitmap) Set(n uint32) {
	if n >= b.size {
		return
	}
	b.bits[n/8] |= 1 << (n % 8)
}

// Clear clears bit n.
func (b *Bitmap) Clear(n uint32) {
	if n >= b.size {
		return
	}
	b.bits[n/8] &^= 1 << (n % 8)
}

// FindFirst returns the index of the first clear bit, or (0, false) if none.
func (b *Bitmap) FindFirst() (uint32, bool) {
	return b.FindNext(0)
}

// FindNext returns the index of the first clear bit at or after start.
// Uses bits.TrailingZeros64 for fast 64-bit word scanning.
func (b *Bitmap) FindNext(start uint32) (uint32, bool) {
	wordIdx := start / 64
	bitOff := start % 64

	nWords := (uint32(len(b.bits)) + 7) / 8
	for w := wordIdx; w <= nWords; w++ {
		// Build uint64 from up to 8 bytes
		var word uint64
		byteBase := int(w) * 8
		for i := 0; i < 8 && byteBase+i < len(b.bits); i++ {
			word |= uint64(b.bits[byteBase+i]) << (uint(i) * 8)
		}

		// Invert: set bits now mean FREE
		inv := ^word
		// For the first word, zero bits below our start position
		if w == wordIdx && bitOff > 0 {
			inv &= ^uint64((1 << bitOff) - 1)
		}
		if inv == 0 {
			continue
		}
		tz := uint32(bits.TrailingZeros64(inv))
		idx := w*64 + tz
		if idx < b.size {
			return idx, true
		}
	}
	return 0, false
}

// Count returns the number of set bits using bits.OnesCount64 for speed.
func (b *Bitmap) Count() uint32 {
	var count uint32
	for i := 0; i+8 <= len(b.bits); i += 8 {
		word := uint64(b.bits[i]) |
			uint64(b.bits[i+1])<<8 |
			uint64(b.bits[i+2])<<16 |
			uint64(b.bits[i+3])<<24 |
			uint64(b.bits[i+4])<<32 |
			uint64(b.bits[i+5])<<40 |
			uint64(b.bits[i+6])<<48 |
			uint64(b.bits[i+7])<<56
		count += uint32(bits.OnesCount64(word))
	}
	// Remaining bytes
	for i := (len(b.bits) / 8) * 8; i < len(b.bits); i++ {
		count += uint32(bits.OnesCount8(b.bits[i]))
	}
	return count
}

// Bytes returns the raw bytes backing the bitmap.
func (b *Bitmap) Bytes() []byte {
	return b.bits
}

// ReadBlockBitmap reads the block bitmap from disk.
func ReadBlockBitmap(dev *disk.BlockDevice) (*Bitmap, error) {
	blk, err := dev.ReadBlock(BlockBitmapAddr)
	if err != nil {
		return nil, fmt.Errorf("bitmap: read block bitmap: %w", err)
	}
	return NewBitmapFromBytes(blk, TotalBlocks), nil
}

// WriteBlockBitmap writes the block bitmap to disk.
func WriteBlockBitmap(dev *disk.BlockDevice, bm *Bitmap) error {
	buf := make([]byte, BlockSize)
	copy(buf, bm.Bytes())
	return dev.WriteBlock(BlockBitmapAddr, buf)
}

// ReadInodeBitmap reads the inode bitmap from disk.
func ReadInodeBitmap(dev *disk.BlockDevice) (*Bitmap, error) {
	blk, err := dev.ReadBlock(InodeBitmapAddr)
	if err != nil {
		return nil, fmt.Errorf("bitmap: read inode bitmap: %w", err)
	}
	return NewBitmapFromBytes(blk, InodeCount), nil
}

// WriteInodeBitmap writes the inode bitmap to disk.
func WriteInodeBitmap(dev *disk.BlockDevice, bm *Bitmap) error {
	buf := make([]byte, BlockSize)
	copy(buf, bm.Bytes())
	return dev.WriteBlock(InodeBitmapAddr, buf)
}
