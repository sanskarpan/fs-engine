package journal

import "encoding/binary"

const (
	JBD2_MAGIC         = 0xC03B3998
	JBD2_DESCRIPTOR    = 1
	JBD2_COMMIT        = 2
	JBD2_SUPERBLOCK_V1 = 3
	JBD2_SUPERBLOCK_V2 = 4
	JBD2_REVOKE        = 5
)

// JournalHeader is the common block header for all journal blocks.
type JournalHeader struct {
	Magic    uint32
	BlockType uint32
	Sequence uint32
}

// JournalDescriptorTag describes a single data block in a descriptor block.
type JournalDescriptorTag struct {
	BlockNr  uint32
	Flags    uint32
	Checksum uint32
}

// JournalCommitBlock is written at the end of a transaction.
type JournalCommitBlock struct {
	Header   JournalHeader
	Checksum uint32
	_        [BlockSize - 16]byte
}

const BlockSize = 4096

// EncodeHeader encodes a JournalHeader into the first 12 bytes of buf.
func EncodeHeader(h JournalHeader, buf []byte) {
	binary.BigEndian.PutUint32(buf[0:], h.Magic)
	binary.BigEndian.PutUint32(buf[4:], h.BlockType)
	binary.BigEndian.PutUint32(buf[8:], h.Sequence)
}

// DecodeHeader decodes a JournalHeader from the first 12 bytes of buf.
func DecodeHeader(buf []byte) JournalHeader {
	return JournalHeader{
		Magic:    binary.BigEndian.Uint32(buf[0:]),
		BlockType: binary.BigEndian.Uint32(buf[4:]),
		Sequence: binary.BigEndian.Uint32(buf[8:]),
	}
}
