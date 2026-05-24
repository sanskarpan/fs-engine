package journal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"

	"github.com/yourname/fs-engine/internal/disk"
)

// Recover scans the journal area and replays any committed transactions
// that have not yet been checkpointed.
func Recover(dev *disk.BlockDevice, start, length uint32) error {
	return RecoverWithHook(dev, start, length, nil)
}

// RecoverWithHook scans the journal and reports replay activity to hook.
func RecoverWithHook(dev *disk.BlockDevice, start, length uint32, hook func(eventType string, fields map[string]interface{})) error {
	if length == 0 {
		return nil
	}

	off := uint32(0)
	for off < length {
		blk, err := dev.ReadBlock(start + off)
		if err != nil {
			return fmt.Errorf("journal: recover read block %d: %w", start+off, err)
		}

		hdr := DecodeHeader(blk)
		if hdr.Magic != JBD2_MAGIC || hdr.BlockType != JBD2_DESCRIPTOR {
			off++
			continue
		}

		seq := hdr.Sequence
		totalAddrs := int(binary.BigEndian.Uint32(blk[firstDescriptorCountOffset:]))
		if totalAddrs <= 0 {
			off++
			continue
		}

		descBlocks := descriptorBlocksFor(totalAddrs)
		addrs := make([]uint32, 0, totalAddrs)
		validChain := true
		for descIdx := 0; descIdx < descBlocks; descIdx++ {
			var desc []byte
			if descIdx == 0 {
				desc = blk
			} else {
				desc, err = dev.ReadBlock(start + off + uint32(descIdx))
				if err != nil {
					return fmt.Errorf("journal: recover read descriptor %d: %w", descIdx, err)
				}
				descHdr := DecodeHeader(desc)
				if descHdr.Magic != JBD2_MAGIC || descHdr.BlockType != JBD2_DESCRIPTOR || descHdr.Sequence != seq {
					validChain = false
					break
				}
			}

			pos := otherDescriptorTagsOffset
			if descIdx == 0 {
				pos = firstDescriptorTagsOffset
			}
			for pos+4 <= BlockSize && len(addrs) < totalAddrs {
				addr := binary.BigEndian.Uint32(desc[pos:])
				addrs = append(addrs, addr)
				pos += 4
			}
		}

		if !validChain || len(addrs) != totalAddrs {
			off++
			continue
		}

		needed := uint32(descBlocks + len(addrs) + 1)
		if off+needed > length {
			break
		}

		// Read data blocks and compute checksum
		dataBlocks := make([][]byte, len(addrs))
		crc := crc32.NewIEEE()
		for i := range addrs {
			db, err := dev.ReadBlock(start + off + uint32(descBlocks) + uint32(i))
			if err != nil {
				return fmt.Errorf("journal: recover read data block: %w", err)
			}
			dataBlocks[i] = db
			crc.Write(db)
		}

		// Read commit block
		commitBlk, err := dev.ReadBlock(start + off + uint32(descBlocks) + uint32(len(addrs)))
		if err != nil {
			return fmt.Errorf("journal: recover read commit: %w", err)
		}
		commitHdr := DecodeHeader(commitBlk)
		if commitHdr.Magic != JBD2_MAGIC || commitHdr.BlockType != JBD2_COMMIT || commitHdr.Sequence != seq {
			// Not a valid commit - skip
			off++
			continue
		}

		storedCRC := binary.BigEndian.Uint32(commitBlk[12:])
		if storedCRC != crc.Sum32() {
			// Checksum mismatch - skip
			off++
			continue
		}

		// Valid committed transaction - replay
		for i, addr := range addrs {
			if err := dev.WriteBlock(addr, dataBlocks[i]); err != nil {
				return fmt.Errorf("journal: recover write block %d: %w", addr, err)
			}
		}
		if hook != nil {
			hook("journal_recover", map[string]interface{}{
				"txnId":      seq,
				"blockCount": len(addrs),
				"blocks":     addrs,
			})
		}

		off += needed
	}

	return nil
}
