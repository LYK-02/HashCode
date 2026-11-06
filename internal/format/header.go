package format

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
)

const (
	HeaderSize       = 32
	HeaderPreCRCSize = 28
	HeaderMagicStr   = "KVDB2026"
	FormatVersion1   = uint32(1)
	HeaderVersion    = FormatVersion1
)

var (
	HeaderMagic  = [8]byte{'K', 'V', 'D', 'B', '2', '0', '2', '6'}
	crcTableCast = crc32.MakeTable(crc32.Castagnoli)
)

// ChecksumCastagnoli computes the 32-bit CRC using the Castagnoli polynomial (0x82F63B78).
func ChecksumCastagnoli(data []byte) uint32 {
	return crc32.Checksum(data, crcTableCast)
}

// Header represents the 32-byte database file header.
type Header struct {
	Magic         [8]byte
	Version       uint32
	CompactionGen uint64
	Generation    uint64 // Alias for CompactionGen
	Reserved      uint64
	CRC32C        uint32
	CRC           uint32 // Alias for CRC32C
}

// DBHeader is an alias for Header for backwards and cross-spec compatibility.
type DBHeader = Header

// NewHeader initializes a standard Header with version 1 and the specified compaction generation.
func NewHeader(compactionGen uint64) Header {
	return Header{
		Magic:         HeaderMagic,
		Version:       FormatVersion1,
		CompactionGen: compactionGen,
		Generation:    compactionGen,
		Reserved:      0,
	}
}

// EncodeHeader serializes the header into a 32-byte slice with Castagnoli CRC32C.
func EncodeHeader(h *Header) ([]byte, error) {
	buf := make([]byte, HeaderSize)
	if err := EncodeHeaderTo(buf, h); err != nil {
		return nil, err
	}
	return buf, nil
}

// EncodeHeaderTo encodes the header directly into an existing byte slice of at least 32 bytes.
func EncodeHeaderTo(buf []byte, h *Header) error {
	if len(buf) < HeaderSize {
		return ErrHeaderTooShort
	}
	magic := h.Magic
	if magic == [8]byte{} {
		magic = HeaderMagic
	}
	copy(buf[0:8], magic[:])

	version := h.Version
	if version == 0 {
		version = FormatVersion1
	}
	binary.LittleEndian.PutUint32(buf[8:12], version)

	gen := h.CompactionGen
	if gen == 0 && h.Generation != 0 {
		gen = h.Generation
	}
	binary.LittleEndian.PutUint64(buf[12:20], gen)
	binary.LittleEndian.PutUint64(buf[20:28], h.Reserved)

	crc := ChecksumCastagnoli(buf[0:HeaderPreCRCSize])
	binary.LittleEndian.PutUint32(buf[28:32], crc)

	h.CRC32C = crc
	h.CRC = crc
	return nil
}

// DecodeHeader deserializes and validates a 32-byte DB header.
func DecodeHeader(b []byte) (*Header, error) {
	if len(b) < HeaderSize {
		return nil, ErrHeaderTooShort
	}
	if !bytes.Equal(b[0:8], HeaderMagic[:]) {
		return nil, ErrInvalidHeaderMagic
	}
	version := binary.LittleEndian.Uint32(b[8:12])
	if version != FormatVersion1 {
		return nil, ErrUnsupportedVersion
	}
	compactionGen := binary.LittleEndian.Uint64(b[12:20])
	reserved := binary.LittleEndian.Uint64(b[20:28])
	if reserved != 0 {
		return nil, ErrReservedNonZero
	}
	storedCRC := binary.LittleEndian.Uint32(b[28:32])
	computedCRC := ChecksumCastagnoli(b[0:HeaderPreCRCSize])
	if storedCRC != computedCRC {
		return nil, ErrCorruptHeader
	}

	return &Header{
		Magic:         HeaderMagic,
		Version:       version,
		CompactionGen: compactionGen,
		Generation:    compactionGen,
		Reserved:      reserved,
		CRC32C:        storedCRC,
		CRC:           storedCRC,
	}, nil
}
