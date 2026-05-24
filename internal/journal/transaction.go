package journal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"time"
)

const (
	firstDescriptorCountOffset = 12
	firstDescriptorTagsOffset  = 16
	otherDescriptorTagsOffset  = 12
)

// txBlock holds a block to be journaled.
type txBlock struct {
	addr uint32
	data []byte
}

// JournalTransaction is an in-progress journal transaction.
type JournalTransaction struct {
	journal  *Journal
	sequence uint32
	blocks   []txBlock
}

// LogBlock adds a block to the transaction.
// data must be exactly BlockSize bytes; a copy is made.
func (t *JournalTransaction) LogBlock(addr uint32, data []byte) error {
	if len(data) != BlockSize {
		return fmt.Errorf("journal: LogBlock data must be %d bytes", BlockSize)
	}
	cp := make([]byte, BlockSize)
	copy(cp, data)
	t.blocks = append(t.blocks, txBlock{addr: addr, data: cp})
	return nil
}

// Commit writes the transaction to the journal and then writes the real blocks.
// Steps (ordered journaling):
//  1. Write descriptor block
//  2. Write each data block to journal space
//  3. Write commit block with CRC32
//  4. Write actual data blocks to their real locations
//  5. Sync
func (t *JournalTransaction) Commit() error {
	if len(t.blocks) == 0 {
		return nil
	}

	j := t.journal
	j.mu.Lock()

	descBlocks := descriptorBlocksFor(len(t.blocks))
	needed := uint32(descBlocks + len(t.blocks) + 1)
	off, err := j.allocBlocks(needed)
	if err != nil {
		j.mu.Unlock()
		return err
	}

	remaining := len(t.blocks)
	index := 0
	for descIdx := 0; descIdx < descBlocks; descIdx++ {
		descBuf := make([]byte, BlockSize)
		EncodeHeader(JournalHeader{
			Magic:     JBD2_MAGIC,
			BlockType: JBD2_DESCRIPTOR,
			Sequence:  t.sequence,
		}, descBuf)

		var pos, capacity int
		if descIdx == 0 {
			binary.BigEndian.PutUint32(descBuf[firstDescriptorCountOffset:], uint32(len(t.blocks)))
			pos = firstDescriptorTagsOffset
			capacity = firstDescriptorCapacity()
		} else {
			pos = otherDescriptorTagsOffset
			capacity = otherDescriptorCapacity()
		}

		n := remaining
		if n > capacity {
			n = capacity
		}
		for i := 0; i < n; i++ {
			binary.BigEndian.PutUint32(descBuf[pos:], t.blocks[index].addr)
			pos += 4
			index++
		}
		remaining -= n

		if err := j.writeJournalBlock(off+uint32(descIdx), descBuf); err != nil {
			j.mu.Unlock()
			return fmt.Errorf("journal: write descriptor %d: %w", descIdx, err)
		}
	}

	// 2. Write data blocks to journal
	crc := crc32.NewIEEE()
	for i, tb := range t.blocks {
		if err := j.writeJournalBlock(off+uint32(descBlocks)+uint32(i), tb.data); err != nil {
			j.mu.Unlock()
			return fmt.Errorf("journal: write data block %d: %w", i, err)
		}
		crc.Write(tb.data)
	}

	// 3. Write commit block
	commitBuf := make([]byte, BlockSize)
	EncodeHeader(JournalHeader{
		Magic:     JBD2_MAGIC,
		BlockType: JBD2_COMMIT,
		Sequence:  t.sequence,
	}, commitBuf)
	binary.BigEndian.PutUint32(commitBuf[12:], crc.Sum32())

	commitOff := off + uint32(descBlocks) + uint32(len(t.blocks))
	if err := j.writeJournalBlock(commitOff, commitBuf); err != nil {
		j.mu.Unlock()
		return fmt.Errorf("journal: write commit: %w", err)
	}

	j.mu.Unlock()

	// Persist the journal before applying home-block writes.
	if err := j.dev.Sync(); err != nil {
		return fmt.Errorf("journal: sync journal: %w", err)
	}

	// 4. Write actual data blocks to their real locations
	for _, tb := range t.blocks {
		if err := j.dev.WriteBlock(tb.addr, tb.data); err != nil {
			return fmt.Errorf("journal: write real block %d: %w", tb.addr, err)
		}
	}

	// 5. Sync
	if err := j.dev.Sync(); err != nil {
		return fmt.Errorf("journal: sync home blocks: %w", err)
	}

	// Record committed transaction for inspection.
	addrs := make([]uint32, len(t.blocks))
	for i, tb := range t.blocks {
		addrs[i] = tb.addr
	}
	j.recordTx(TxRecord{
		ID:        t.sequence,
		Status:    "committed",
		Blocks:    addrs,
		Timestamp: time.Now(),
	})
	if j.metrics != nil {
		j.metrics.JournalCommits.Add(1)
	}
	if j.emit != nil {
		j.emit("journal_commit", map[string]interface{}{
			"txnId":      t.sequence,
			"blockCount": len(t.blocks),
			"blocks":     addrs,
		})
	}
	return nil
}

func firstDescriptorCapacity() int {
	return (BlockSize - firstDescriptorTagsOffset) / 4
}

func otherDescriptorCapacity() int {
	return (BlockSize - otherDescriptorTagsOffset) / 4
}

func descriptorBlocksFor(blocks int) int {
	if blocks <= firstDescriptorCapacity() {
		return 1
	}
	remaining := blocks - firstDescriptorCapacity()
	per := otherDescriptorCapacity()
	return 1 + (remaining+per-1)/per
}
