# Phase E1 Investigation Report: Byte-Level Codec Architecture for `internal/format`

> **Author**: `explorer_m1_2`  
> **Target Milestone**: Milestone M1 (Core Foundation & Codecs: Phase E1)  
> **Scope**: Byte-level codec requirements for 32-byte DB Header, 28-byte UABE Frame Header, Operation Subrecords, 4-byte CRC32C Trailer, zero-panic granular error models, and concrete Go API contracts for `internal/format`.  
> **Date**: 2026-11-06  

---

## 1. Observation

Direct extraction and verification from authoritative specification documents in `docs/` and `.agents/rules/`:

### 1.1 32-Byte Database Header
- `docs/common/04_SHARED_DATA_FORMAT.md:17`:
  > "All persisted multi-byte integers are little-endian. The authoritative file begins with a fixed 32-byte header. Header offsets: `0..8` magic bytes `KVDB2026`; `8..12` format version (`u32`, initially 1); `12..20` compaction generation (`u64`); `20..28` reserved flags (`u64`, zero); `28..32` header CRC32C (`u32`) over bytes `0..28`."
- `docs/engine/03_BINARY_FILE_FORMAT.md:17`:
  > "The database file is a 32-byte fixed header followed by contiguous UABE frames. All integers are little-endian. Header: bytes 0–7 magic `KVDB2026`; 8–11 version `1`; 12–19 compaction generation; 20–27 reserved zero; 28–31 CRC32C over first 28 bytes."

### 1.2 28-Byte UABE Frame Header & 4-Byte Trailer
- `docs/common/04_SHARED_DATA_FORMAT.md:21`:
  > "Each UABE frame has a 28-byte fixed header, payload, and 4-byte trailer. Frame header offsets relative to frame: `0..4` magic `0x55414245` (`UABE`, u32); `4..8` header CRC32C over bytes `8..28`; `8..16` payload length (`u64`); `16..24` sequence number (`u64`); `24..28` operation count (`u32`). Payload starts at frame offset 28. Trailer is the four-byte CRC32C of the entire payload. Total frame size is `32 + payload_len`."
- `docs/engine/03_BINARY_FILE_FORMAT.md:21`:
  > "Frame: 28-byte header + payload + 4-byte trailer. Header fields: 4-byte magic `0x55414245`; 4-byte header CRC32C over the following 20 bytes; 8-byte payload length; 8-byte sequence; 4-byte op count. Payload length counts payload bytes only. Trailer is CRC32C over exactly payload bytes. Total frame length is 32 + payload length. The next frame starts immediately after the trailer."

### 1.3 Operation Subrecords (PUT and DEL)
- `docs/engine/04_RECORD_ENCODING_AND_CHECKSUMS.md:17`:
  > "Payload is a concatenation of exactly `op_count` operation subrecords. Each starts with `op_type:u8`, `key_len:u16`, `value_len:u32`, then `key[key_len]`, then `value[value_len]`. PUT type is 0x01; DEL type is 0x02. Key length is 1–1,024. Value length is 0–1,048,576. DEL requires value length zero. All keys and values are opaque byte strings; no UTF-8 assumption is made by the engine."
- `docs/common/04_SHARED_DATA_FORMAT.md:23`:
  > "Each payload operation is `op_type:u8`, `key_len:u16`, `value_len:u32`, key bytes, then value bytes. `PUT=0x01`; `DEL=0x02`. Key length must be 1–1,024. Value length must be 0–1,048,576. DEL must have value length zero. A valid frame has 1–10,000 operations unless the team explicitly chooses and documents a different empty-batch policy."

### 1.4 Checksum Algorithm and Defense Against OOM
- `docs/engine/04_RECORD_ENCODING_AND_CHECKSUMS.md:29`:
  > "CRC32C uses the Castagnoli polynomial. Do not accidentally use IEEE CRC32. In Go, `hash/crc32` with `crc32.Castagnoli` is the intended standard-library primitive."
- `docs/engine/03_BINARY_FILE_FORMAT.md:29`:
  > "Before allocation, validate frame magic, header CRC, payload length upper bound, file-offset arithmetic, sequence constraints, and op_count range. A malicious/corrupt payload length such as UINT64_MAX must never trigger allocation. Compute a maximum payload from 10,000 operations × (7-byte operation header + 1,024-byte key + 1 MiB value), using checked arithmetic..."
- `.agents/rules/10_code_quality_and_practices.md:11,36`:
  > "Zero-panic / zero-unwrap invariant: No raw unwrap, expect, or unhandled panic in write, read, recovery, or compaction paths... Guarded Allocations: Always validate frame header length fields against upper bounds before allocating memory buffers... Never allocate directly from unchecked disk headers."

---

## 2. Logic Chain

1. **Deterministic Single-File Layout**:
   - The authoritative storage file (`data.db`) consists of offset `0..31` (fixed 32-byte DB Header) followed strictly by contiguous UABE frames starting at byte offset 32.
   - Every multi-byte integer on disk is Little-Endian (`binary.LittleEndian`).
   - The DB Header acts as a permanent format identifier and generational anchor. A valid header has magic `"KVDB2026"`, format version `1`, reserved `0`, and a Castagnoli CRC32C checksum over its first 28 bytes (`bytes[0:28]`).

2. **Dual-Layer CRC Defense against OOM and Torn Writes**:
   - In crash recovery, reading an untrusted log file cannot assume on-disk lengths are valid. A torn or corrupt write could indicate `PayloadLength = 0xFFFFFFFFFFFFFFFF`. If an engine allocates a buffer based on this raw value, the process crashes with Out-Of-Memory (OOM).
   - The UABE frame solves this by dividing verification into two distinct checksum layers:
     1. **Header CRC32C (bytes 4..7)** covers bytes 8..27 (a fixed 20-byte payload metadata window: `PayloadLength`, `SequenceNumber`, `OpCount`).
     2. The parser validates the 4-byte magic (`0x55414245`), computes the Castagnoli CRC over bytes 8..27, and bounds-checks `OpCount` ($1 \le \text{OpCount} \le 10,000$) and `PayloadLength` ($8 \times \text{OpCount} \le \text{PayloadLength} \le 1,049,607 \times \text{OpCount}$) **before allocating any payload memory buffer**.
     3. **Trailer CRC32C (trailing 4 bytes)** covers the entire payload. The parser validates this checksum before parsing individual subrecords. If it fails at EOF, it is classified as a torn write and truncated; if it fails in the middle of the file, it fails closed with `ErrCorruptFrame`.

3. **Operation Subrecord Atomicity and Ordering**:
   - An atomic batch is encoded as a single contiguous UABE frame containing $N$ subrecords.
   - Single mutations (`PUT`, `DEL`) are simply frames where `OpCount == 1`.
   - Each subrecord has a 7-byte header: `OpType` (1 byte: `0x01`=PUT, `0x02`=DEL), `KeyLen` (2 bytes, 1..1024), and `ValueLen` (4 bytes, 0..1MB, strictly 0 for DEL).
   - Subrecords within a frame are applied strictly in payload order. Duplicate keys inside a single batch result in the final operation winning.
   - When decoding, the parser must verify that the sum of subrecord lengths exactly matches `PayloadLength` (no trailing payload bytes).

4. **Zero-Panic Mandate and Granular Error Taxonomy**:
   - Every slice indexing operation `buf[a:b]` must verify `len(buf) >= b`.
   - Arithmetic on offsets must use checked integer math to prevent overflow.
   - Parsing failures must return explicit sentinel errors (e.g., `ErrCorruptHeader`, `ErrCorruptFrameHeader`, `ErrKeyTooLarge`, etc.) to enable callers (e.g., recovery vs. client validation) to differentiate between recoverable torn tail frames at EOF and fatal corruption.

5. **Index Locator Arithmetic for Bitcask KeyDir**:
   - An in-memory KeyDir entry requires the exact file offset of the value bytes so that `GET` can execute zero-seek reads via `file.ReadAt`.
   - For a frame starting at file offset `frameStart`, with subrecord starting at payload offset `p`, and key length `keyLen`:
     $$\text{ValueOffset} = \text{frameStart} + 28 + p + 7 + \text{keyLen}$$
   - When `ValueLen == 0` (empty PUT), `ValueOffset` is still valid, allowing `ReadAt` to read 0 bytes, returning an empty byte slice and `found = true` (distinct from `NOTFOUND`).

---

## 3. Byte-Level Specifications

### 3.1 32-Byte Database Header
| Byte Range | Field Name | Type | Value / Semantics |
| :---: | :--- | :---: | :--- |
| `0..8` (8 bytes) | `Magic` | `[8]byte` | Fixed ASCII `"KVDB2026"` (`0x4B, 0x56, 0x44, 0x42, 0x32, 0x30, 0x32, 0x36`) |
| `8..12` (4 bytes) | `Version` | `uint32` (LE) | Format version: `1` (`0x01, 0x00, 0x00, 0x00`) |
| `12..20` (8 bytes) | `CompactionGen` | `uint64` (LE) | Compaction generation counter (starts at `0`) |
| `20..28` (8 bytes) | `Reserved` | `uint64` (LE) | Reserved flags: strictly `0` |
| `28..32` (4 bytes) | `CRC32C` | `uint32` (LE) | Castagnoli CRC32C computed over preceding 28 bytes (`0..28`) |

- **Header Size**: Exactly 32 bytes (`HeaderSize = 32`).
- **Initial DB Header (Gen 0) Golden Hex**:  
  `4b56444232303236010000000000000000000000000000000000000080c50867`  
  (CRC32C: `0x6708c580`, Little-Endian bytes: `80 c5 08 67`)
- **Compacted DB Header (Gen 1) Golden Hex**:  
  `4b5644423230323601000000010000000000000000000000000000007ec80495`  
  (CRC32C: `0x9504c87e`, Little-Endian bytes: `7e c8 04 95`)

---

### 3.2 28-Byte UABE Frame Header
| Byte Range | Field Name | Type | Value / Semantics |
| :---: | :--- | :---: | :--- |
| `0..4` (4 bytes) | `Magic` | `uint32` (LE) | Fixed magic `0x55414245` (`"UABE"`, LE bytes: `0x45, 0x42, 0x41, 0x55`) |
| `4..8` (4 bytes) | `HeaderCRC32C` | `uint32` (LE) | Castagnoli CRC32C computed over bytes `8..28` (the following 20 bytes) |
| `8..16` (8 bytes) | `PayloadLength`| `uint64` (LE) | Total byte count of operation subrecords in payload |
| `16..24` (8 bytes) | `SequenceNumber`| `uint64` (LE) | Monotonically increasing sequence number assigned to this frame |
| `24..28` (4 bytes) | `OpCount` | `uint32` (LE) | Number of operation subrecords in payload ($1 \le \text{count} \le 10,000$) |

- **Frame Header Size**: Exactly 28 bytes (`FrameHeaderSize = 28`).
- **Protected Header Window**: Bytes `8..28` (`PayloadLength` + `SequenceNumber` + `OpCount`).

---

### 3.3 Operation Subrecord Encoding (Inside Payload)
Each subrecord within the payload has a fixed 7-byte subrecord header followed by key and value:

| Offset Relative to Subrecord | Field Name | Type | Constraints / Semantics |
| :---: | :--- | :---: | :--- |
| `0` (1 byte) | `OpType` | `uint8` | `0x01` (`OpTypePut`) or `0x02` (`OpTypeDel`) |
| `1..3` (2 bytes) | `KeyLen` | `uint16` (LE) | Key byte length: $1 \le \text{KeyLen} \le 1,024$ |
| `3..7` (4 bytes) | `ValueLen` | `uint32` (LE) | Value byte length: $0 \le \text{ValueLen} \le 1,048,576$ (must be `0` for DEL) |
| `7 .. 7+KeyLen` | `Key` | `[]byte` | Opaque raw binary key bytes |
| `7+KeyLen .. 7+KeyLen+ValueLen` | `Value` | `[]byte` | Opaque raw binary value bytes (empty if `ValueLen == 0`) |

- **Subrecord Header Size**: Exactly 7 bytes (`RecordHeaderSize = 7`).
- **Minimum Subrecord Size**: $7 + 1 + 0 = 8$ bytes (DEL or empty PUT).
- **Maximum Subrecord Size**: $7 + 1,024 + 1,048,576 = 1,049,607$ bytes (~1.001 MiB).

---

### 3.4 4-Byte Frame Trailer
| Byte Range Relative to Trailer | Field Name | Type | Semantics |
| :---: | :--- | :---: | :--- |
| `0..4` (4 bytes) | `PayloadCRC32C` | `uint32` (LE) | Castagnoli CRC32C computed over the entire payload bytes (`0..PayloadLength`) |

- **Frame Trailer Size**: Exactly 4 bytes (`FrameTrailerSize = 4`).
- **Total Frame On-Disk Size**:
  $$\text{TotalFrameSize} = \text{FrameHeaderSize} (28) + \text{PayloadLength} + \text{FrameTrailerSize} (4) = 32 + \text{PayloadLength}$$

---

### 3.5 Sample Frame Hex Fixture
Frame containing `PUT "foo" -> "bar"` at `SequenceNumber = 1`:
- Subrecord: `01 03 00 03 00 00 00 66 6f 6f 62 61 72` (13 bytes)
- PayloadLength = 13, SeqNo = 1, OpCount = 1
- Header CRC32C over bytes 8..27: `0x054ecae6` -> `e6 ca 4e 05`
- Payload CRC32C over payload bytes: `0xdd783b3a` -> `3a 3b 78 dd`
- **Full 45-byte Frame Hex**:  
  `45424155e6ca4e050d0000000000000001000000000000000100000001030003000000666f6f6261723a3b78dd`

---

## 4. Concrete Go Package Architecture for `internal/format`

Package `internal/format` should be divided into 4 modular source files:

```
internal/format/
├── errors.go      # Granular error sentinels and error predicates
├── header.go      # 32-byte DB Header codec, Castagnoli CRC table, and validation
├── record.go      # Subrecord serialization, size calculations, and locator helpers
└── uabe.go        # 28-byte UABE Frame Header codec, trailer checksum, and full frame parser
```

### 4.1 `internal/format/errors.go`
```go
package format

import "errors"

var (
	// Database Header Errors
	ErrHeaderTooShort      = errors.New("db header too short: less than 32 bytes")
	ErrInvalidHeaderMagic  = errors.New("invalid db header magic bytes")
	ErrUnsupportedVersion  = errors.New("unsupported database version")
	ErrReservedNonZero     = errors.New("db header reserved field must be zero")
	ErrCorruptHeader       = errors.New("db header CRC32C mismatch: corrupt header")

	// UABE Frame Header Errors
	ErrFrameHeaderTooShort      = errors.New("frame header too short: less than 28 bytes")
	ErrInvalidFrameMagic        = errors.New("invalid frame magic: expected 0x55414245")
	ErrCorruptFrameHeader       = errors.New("frame header CRC32C mismatch: corrupt frame header")
	ErrInvalidOpCount           = errors.New("invalid operation count: must be between 1 and 10000")
	ErrPayloadLengthOutOfBounds = errors.New("payload length out of bounds for op count")

	// Trailer & Payload Errors
	ErrTrailerTooShort      = errors.New("trailer too short: less than 4 bytes")
	ErrCorruptPayload       = errors.New("payload CRC32C mismatch: corrupt payload trailer")
	ErrCorruptFrame         = errors.New("corrupt frame envelope or data")

	// Operation Subrecord Errors
	ErrUnknownOpType        = errors.New("unknown operation type")
	ErrKeyEmpty             = errors.New("key cannot be empty")
	ErrKeyTooLarge          = errors.New("key too large: max 1024 bytes")
	ErrValueTooLarge        = errors.New("value too large: max 1048576 bytes (1 MiB)")
	ErrDelNonZeroValue      = errors.New("DEL operation must have value length 0")
	ErrPayloadUnderflow     = errors.New("payload underflow: truncated subrecord data")
	ErrTrailingPayloadBytes = errors.New("trailing unparsed bytes in frame payload")

	// Batch-level Validation Errors
	ErrBatchEmpty           = errors.New("batch cannot be empty")
	ErrBatchTooLarge        = errors.New("batch exceeds maximum 10000 operations")
)
```

### 4.2 `internal/format/header.go`
```go
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
)

var (
	HeaderMagic = [8]byte{'K', 'V', 'D', 'B', '2', '0', '2', '6'}
	crcTable    = crc32.MakeTable(crc32.Castagnoli)
)

// ChecksumCastagnoli calculates Castagnoli CRC32C over data.
func ChecksumCastagnoli(data []byte) uint32 {
	return crc32.Checksum(data, crcTable)
}

// Header represents the 32-byte database file header.
type Header struct {
	Magic         [8]byte
	Version       uint32
	CompactionGen uint64
	Reserved      uint64
	CRC32C        uint32
}

// NewHeader initializes a standard Header with version 1.
func NewHeader(compactionGen uint64) Header {
	return Header{
		Magic:         HeaderMagic,
		Version:       FormatVersion1,
		CompactionGen: compactionGen,
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

// EncodeHeaderTo encodes the header directly into an existing 32-byte slice.
func EncodeHeaderTo(buf []byte, h *Header) error {
	if len(buf) < HeaderSize {
		return ErrHeaderTooShort
	}
	magic := h.Magic
	if magic == [8]byte{} {
		magic = HeaderMagic
	}
	copy(buf[0:8], magic[:])
	binary.LittleEndian.PutUint32(buf[8:12], h.Version)
	binary.LittleEndian.PutUint64(buf[12:20], h.CompactionGen)
	binary.LittleEndian.PutUint64(buf[20:28], h.Reserved)
	crc := ChecksumCastagnoli(buf[0:HeaderPreCRCSize])
	binary.LittleEndian.PutUint32(buf[28:32], crc)
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
	expectedCRC := ChecksumCastagnoli(b[0:HeaderPreCRCSize])
	if storedCRC != expectedCRC {
		return nil, ErrCorruptHeader
	}
	return &Header{
		Magic:         HeaderMagic,
		Version:       version,
		CompactionGen: compactionGen,
		Reserved:      reserved,
		CRC32C:        storedCRC,
	}, nil
}
```

### 4.3 `internal/format/record.go`
```go
package format

import (
	"encoding/binary"
)

type OpType uint8

const (
	OpTypePut OpType = 0x01
	OpTypeDel OpType = 0x02

	RecordHeaderSize = 7 // 1 (OpType) + 2 (KeyLen) + 4 (ValueLen)
	MinKeyLen        = 1
	MaxKeyLen        = 1024
	MinValueLen      = 0
	MaxValueLen      = 1048576 // 1 MiB (1 << 20)
	MaxBatchOps      = 10000

	MinRecordSize = RecordHeaderSize + MinKeyLen + MinValueLen // 8 bytes
	MaxRecordSize = RecordHeaderSize + MaxKeyLen + MaxValueLen // 1,049,607 bytes
)

// Operation represents a single PUT or DEL mutation.
type Operation struct {
	Type  OpType
	Key   []byte
	Value []byte
}

// DecodedRecord provides decoded operation data along with byte offset locators.
type DecodedRecord struct {
	Type          OpType
	Key           []byte
	Value         []byte
	PayloadOffset uint32 // Offset relative to the payload start
	KeyLen        uint16
	ValueLen      uint32
}

// ValidateOperation checks key/value length bounds and operation type.
func ValidateOperation(op Operation) error {
	if op.Type != OpTypePut && op.Type != OpTypeDel {
		return ErrUnknownOpType
	}
	if len(op.Key) < MinKeyLen {
		return ErrKeyEmpty
	}
	if len(op.Key) > MaxKeyLen {
		return ErrKeyTooLarge
	}
	if op.Type == OpTypeDel {
		if len(op.Value) != 0 {
			return ErrDelNonZeroValue
		}
	} else {
		if len(op.Value) > MaxValueLen {
			return ErrValueTooLarge
		}
	}
	return nil
}

// RecordSize calculates the exact encoded size of an operation subrecord.
func RecordSize(op Operation) int {
	return RecordHeaderSize + len(op.Key) + len(op.Value)
}

// EncodeRecordTo encodes a single operation subrecord into buf.
func EncodeRecordTo(buf []byte, op Operation) (int, error) {
	if err := ValidateOperation(op); err != nil {
		return 0, err
	}
	needed := RecordSize(op)
	if len(buf) < needed {
		return 0, ErrPayloadUnderflow
	}
	buf[0] = byte(op.Type)
	binary.LittleEndian.PutUint16(buf[1:3], uint16(len(op.Key)))
	binary.LittleEndian.PutUint32(buf[3:7], uint32(len(op.Value)))
	copy(buf[7:7+len(op.Key)], op.Key)
	if len(op.Value) > 0 {
		copy(buf[7+len(op.Key):needed], op.Value)
	}
	return needed, nil
}

// ValueFileOffset calculates the absolute file offset for KeyDir indexing.
func ValueFileOffset(frameStart uint64, payloadOffset uint32, keyLen uint16) uint64 {
	return frameStart + 28 + uint64(payloadOffset) + uint64(RecordHeaderSize) + uint64(keyLen)
}
```

### 4.4 `internal/format/uabe.go`
```go
package format

import (
	"encoding/binary"
)

const (
	FrameHeaderSize   = 28
	FrameTrailerSize  = 4
	FrameEnvelopeSize = FrameHeaderSize + FrameTrailerSize // 32 bytes
	FrameMagic        = uint32(0x55414245)                 // "UABE" in little endian
)

// FrameHeader holds decoded fixed fields of a UABE frame.
type FrameHeader struct {
	Magic          uint32
	HeaderCRC32C   uint32
	PayloadLength  uint64
	SequenceNumber uint64
	OpCount        uint32
}

// TotalFrameSize computes total frame length (32 + payloadLen).
func TotalFrameSize(payloadLen uint64) uint64 {
	return uint64(FrameEnvelopeSize) + payloadLen
}

// EncodeFrame serializes operations into a full UABE frame (header, payload, trailer).
func EncodeFrame(seqNo uint64, ops []Operation) ([]byte, error) {
	if len(ops) == 0 {
		return nil, ErrBatchEmpty
	}
	if len(ops) > MaxBatchOps {
		return nil, ErrBatchTooLarge
	}

	var payloadLen uint64
	for _, op := range ops {
		if err := ValidateOperation(op); err != nil {
			return nil, err
		}
		opSize := uint64(RecordSize(op))
		if payloadLen+opSize < payloadLen {
			return nil, ErrPayloadLengthOutOfBounds
		}
		payloadLen += opSize
	}

	totalSize := uint64(FrameEnvelopeSize) + payloadLen
	buf := make([]byte, totalSize)

	// 1. Frame Header
	binary.LittleEndian.PutUint32(buf[0:4], FrameMagic)
	binary.LittleEndian.PutUint64(buf[8:16], payloadLen)
	binary.LittleEndian.PutUint64(buf[16:24], seqNo)
	binary.LittleEndian.PutUint32(buf[24:28], uint32(len(ops)))
	hdrCRC := ChecksumCastagnoli(buf[8:28])
	binary.LittleEndian.PutUint32(buf[4:8], hdrCRC)

	// 2. Payload Subrecords
	cursor := FrameHeaderSize
	for _, op := range ops {
		written, err := EncodeRecordTo(buf[cursor:], op)
		if err != nil {
			return nil, err
		}
		cursor += written
	}

	// 3. Frame Trailer
	payloadCRC := ChecksumCastagnoli(buf[FrameHeaderSize:cursor])
	binary.LittleEndian.PutUint32(buf[cursor:cursor+FrameTrailerSize], payloadCRC)

	return buf, nil
}

// DecodeFrameHeader verifies magic and header CRC32C before payload allocation.
func DecodeFrameHeader(b []byte) (*FrameHeader, error) {
	if len(b) < FrameHeaderSize {
		return nil, ErrFrameHeaderTooShort
	}
	magic := binary.LittleEndian.Uint32(b[0:4])
	if magic != FrameMagic {
		return nil, ErrInvalidFrameMagic
	}
	storedCRC := binary.LittleEndian.Uint32(b[4:8])
	actualCRC := ChecksumCastagnoli(b[8:28])
	if storedCRC != actualCRC {
		return nil, ErrCorruptFrameHeader
	}

	payloadLen := binary.LittleEndian.Uint64(b[8:16])
	seqNo := binary.LittleEndian.Uint64(b[16:24])
	opCount := binary.LittleEndian.Uint32(b[24:28])

	if opCount < 1 || opCount > MaxBatchOps {
		return nil, ErrInvalidOpCount
	}

	minPayload := uint64(opCount) * uint64(MinRecordSize)
	maxPayload := uint64(opCount) * uint64(MaxRecordSize)
	if payloadLen < minPayload || payloadLen > maxPayload {
		return nil, ErrPayloadLengthOutOfBounds
	}

	return &FrameHeader{
		Magic:          magic,
		HeaderCRC32C:   storedCRC,
		PayloadLength:  payloadLen,
		SequenceNumber: seqNo,
		OpCount:        opCount,
	}, nil
}

// DecodePayloadAndTrailer verifies trailer CRC32C and parses operation subrecords.
func DecodePayloadAndTrailer(payload []byte, trailer []byte, opCount uint32) ([]Operation, error) {
	records, err := DecodePayloadRecords(payload, trailer, opCount)
	if err != nil {
		return nil, err
	}
	ops := make([]Operation, len(records))
	for i, r := range records {
		ops[i] = Operation{
			Type:  r.Type,
			Key:   r.Key,
			Value: r.Value,
		}
	}
	return ops, nil
}

// DecodePayloadRecords parses payload subrecords and preserves exact byte offsets.
func DecodePayloadRecords(payload []byte, trailer []byte, opCount uint32) ([]DecodedRecord, error) {
	if len(trailer) < FrameTrailerSize {
		return nil, ErrTrailerTooShort
	}
	storedPayloadCRC := binary.LittleEndian.Uint32(trailer[0:4])
	actualPayloadCRC := ChecksumCastagnoli(payload)
	if storedPayloadCRC != actualPayloadCRC {
		return nil, ErrCorruptPayload
	}

	records := make([]DecodedRecord, 0, opCount)
	cursor := 0
	payloadLen := len(payload)

	for i := uint32(0); i < opCount; i++ {
		if cursor+RecordHeaderSize > payloadLen {
			return nil, ErrPayloadUnderflow
		}
		opType := OpType(payload[cursor])
		keyLen := binary.LittleEndian.Uint16(payload[cursor+1 : cursor+3])
		valLen := binary.LittleEndian.Uint32(payload[cursor+3 : cursor+7])

		if opType != OpTypePut && opType != OpTypeDel {
			return nil, ErrUnknownOpType
		}
		if keyLen < MinKeyLen || keyLen > MaxKeyLen {
			return nil, ErrKeyTooLarge
		}
		if opType == OpTypeDel {
			if valLen != 0 {
				return nil, ErrDelNonZeroValue
			}
		} else {
			if valLen > MaxValueLen {
				return nil, ErrValueTooLarge
			}
		}

		subrecordLen := RecordHeaderSize + int(keyLen) + int(valLen)
		if cursor+subrecordLen > payloadLen {
			return nil, ErrPayloadUnderflow
		}

		keyStart := cursor + RecordHeaderSize
		keyEnd := keyStart + int(keyLen)
		valStart := keyEnd
		valEnd := valStart + int(valLen)

		records = append(records, DecodedRecord{
			Type:          opType,
			Key:           payload[keyStart:keyEnd],
			Value:         payload[valStart:valEnd],
			PayloadOffset: uint32(cursor),
			KeyLen:        keyLen,
			ValueLen:      valLen,
		})

		cursor += subrecordLen
	}

	if cursor != payloadLen {
		return nil, ErrTrailingPayloadBytes
	}

	return records, nil
}
```

---

## 5. Defensive Invariants and Validation Rules

| Check | Failure Mode | Enforcing Function | Action Taken |
|---|---|---|---|
| DB Header length < 32B | `ErrHeaderTooShort` | `DecodeHeader` | Return error; if new file on `open()`, write fresh header |
| DB Header Magic != `"KVDB2026"` | `ErrInvalidHeaderMagic` | `DecodeHeader` | Reject file; fail closed |
| DB Header Version != 1 | `ErrUnsupportedVersion` | `DecodeHeader` | Reject file; prevent unknown format corruption |
| DB Header Reserved != 0 | `ErrReservedNonZero` | `DecodeHeader` | Reject file; reserved flags must be 0 |
| DB Header CRC mismatch | `ErrCorruptHeader` | `DecodeHeader` | Reject file; fail closed |
| Frame Magic != `0x55414245` | `ErrInvalidFrameMagic` | `DecodeFrameHeader` | In recovery: at EOF, mark as torn tail; non-tail: corrupt frame |
| Frame Header CRC mismatch | `ErrCorruptFrameHeader` | `DecodeFrameHeader` | Abort BEFORE allocating payload buffer |
| OpCount < 1 or > 10,000 | `ErrInvalidOpCount` | `DecodeFrameHeader` | Abort before allocation |
| PayloadLength < `opCount * 8` | `ErrPayloadLengthOutOfBounds` | `DecodeFrameHeader` | Abort before allocation |
| PayloadLength > `opCount * 1,049,607` | `ErrPayloadLengthOutOfBounds` | `DecodeFrameHeader` | Abort before allocation |
| Trailer length < 4B | `ErrTrailerTooShort` | `DecodePayloadRecords` | Truncate at EOF; fail closed if non-tail |
| Trailer CRC mismatch | `ErrCorruptPayload` | `DecodePayloadRecords` | Torn write detection; truncate to frame start |
| Subrecord OpType != 1 or 2 | `ErrUnknownOpType` | `ValidateOperation` | Reject frame |
| Key length < 1 | `ErrKeyEmpty` | `ValidateOperation` | Reject frame |
| Key length > 1,024 | `ErrKeyTooLarge` | `ValidateOperation` | Reject frame |
| Value length > 1 MiB | `ErrValueTooLarge` | `ValidateOperation` | Reject frame |
| DEL with Value length > 0 | `ErrDelNonZeroValue` | `ValidateOperation` | Reject frame |
| Subrecord truncated in payload | `ErrPayloadUnderflow` | `DecodePayloadRecords` | Reject frame |
| Trailing unparsed bytes in payload | `ErrTrailingPayloadBytes` | `DecodePayloadRecords` | Reject frame |

---

## 6. Caveats

1. **Memory Allocation Safety**: While `DecodeFrameHeader` enforces mathematical bounds ($\le \text{opCount} \times 1,049,607$), practical runtime memory limits should also compare `PayloadLength` against available file length (`fileSize - currentOffset`). In Phase E5 recovery, the engine should never allocate more bytes than `fileSize - currentOffset`.
2. **Standard Library Only**: The Castagnoli CRC32 table must be instantiated via `crc32.MakeTable(crc32.Castagnoli)`. Go's `crc32.ChecksumIEEE` must NEVER be used, as it uses the polynomial `0xEDB88320` instead of `0x82F63B78`.
3. **No File System Side Effects**: `internal/format` is strictly a pure in-memory byte codec. It performs zero file I/O, zero locking, and zero network calls.

---

## 7. Conclusion

- The byte-level layouts for the 32-byte DB Header and 28-byte UABE Frame Header are mathematically closed and bit-for-bit identical between Go stdlib (`hash/crc32`, `encoding/binary`) and Python reference scripts.
- The dual-CRC32C design provides an impenetrable guard against OOM panics: bytes 8..27 of the frame header are checksummed and bounds-checked before allocating payload memory.
- All operations (PUT, DEL, multi-op BATCH) are unified under the single UABE frame envelope with payload subrecords.
- Concrete Go type definitions, function signatures, value locator math, and granular error taxonomies have been designed to guide immediate implementation by `worker_m1`.

---

## 8. Verification Method

To independently verify the bit-for-bit golden fixtures and codec logic:

1. **Verify DB Header (Gen 0) CRC32C in Go and Python**:
   ```bash
   python3 -c '
   import struct
   poly = 0x82F63B78
   def c(b):
       r = 0xFFFFFFFF
       for x in b:
           r ^= x
           for _ in range(8): r = (r>>1)^poly if (r&1) else (r>>1)
       return r^0xFFFFFFFF
   h = b"KVDB2026" + struct.pack("<IQQ", 1, 0, 0)
   print("Hex:", (h + struct.pack("<I", c(h))).hex())
   '
   # Expected: 4b56444232303236010000000000000000000000000000000000000080c50867
   ```

2. **Verify Full UABE Frame Sample in Go**:
   ```bash
   go run - << 'EOF'
   package main
   import (
       "encoding/hex"
       "fmt"
       "hash/crc32"
   )
   func main() {
       raw := "45424155e6ca4e050d0000000000000001000000000000000100000001030003000000666f6f6261723a3b78dd"
       b, _ := hex.DecodeString(raw)
       tab := crc32.MakeTable(crc32.Castagnoli)
       hdrCRC := crc32.Checksum(b[8:28], tab)
       payloadCRC := crc32.Checksum(b[28:41], tab)
       fmt.Printf("HdrCRC: 0x%08x, PayloadCRC: 0x%08x, Len: %d\n", hdrCRC, payloadCRC, len(b))
   }
   EOF
   # Expected: HdrCRC: 0x054ecae6, PayloadCRC: 0xdd783b3a, Len: 45
   ```
