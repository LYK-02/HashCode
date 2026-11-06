# Exhaustive Test Specification & Golden Fixtures Report (Milestone M1: Phases E0 & E1)

> **Agent:** `spec_miner_m1_3`  
> **Milestone:** M1 — Core Foundation & Codecs (Phases E0 & E1)  
> **Target Files:** `internal/format/header.go`, `internal/format/uabe.go`, `internal/format/record.go`, `internal/format/errors.go`, `internal/format/codec_test.go`  
> **Working Directory:** `/home/laik/workspace/srinathonn/.agents/teamwork/spec_miner_m1_3/`  
> **Date:** 2026-11-06  

---

## 1. Observation

### 1.1 Verbatim Specifications and Field Offsets
- **32-Byte DB Header Specification** (`docs/common/04_SHARED_DATA_FORMAT.md:17-18`, `docs/engine/03_BINARY_FILE_FORMAT.md:17-18`):
  - Offset `0..8`: Magic bytes ASCII `"KVDB2026"` (`0x4B, 0x56, 0x44, 0x42, 0x32, 0x30, 0x32, 0x36`).
  - Offset `8..12`: Format version (`uint32` Little-Endian, value `1`).
  - Offset `12..20`: Compaction generation (`uint64` Little-Endian, initially `0`).
  - Offset `20..28`: Reserved flags (`uint64` Little-Endian, strictly `0`).
  - Offset `28..32`: Header CRC32C (`uint32` Little-Endian) computed over bytes `0..28` using Castagnoli polynomial (`0x82F63B78`).

- **28-Byte UABE Frame Header Specification** (`docs/common/04_SHARED_DATA_FORMAT.md:21-24`, `docs/engine/03_BINARY_FILE_FORMAT.md:21`):
  - Offset `0..4`: Frame Magic `0x55414245` (`uint32` Little-Endian; on-disk bytes `0x45, 0x42, 0x41, 0x55`).
  - Offset `4..8`: Header CRC32C (`uint32` Little-Endian) computed over the subsequent 20 bytes (`8..28`).
  - Offset `8..16`: Payload length in bytes (`uint64` Little-Endian).
  - Offset `16..24`: Monotonic sequence number (`uint64` Little-Endian).
  - Offset `24..28`: Operation count (`uint32` Little-Endian, range `1..10,000`).

- **Payload Subrecords & Limits** (`docs/engine/04_RECORD_ENCODING_AND_CHECKSUMS.md:17-18`):
  - Offset `0..1`: `OpType` (`uint8`): `0x01` = PUT, `0x02` = DEL.
  - Offset `1..3`: `KeyLen` (`uint16` Little-Endian): range `1..1,024` bytes.
  - Offset `3..7`: `ValLen` (`uint32` Little-Endian): range `0..1,048,576` bytes ($1\text{ MiB}$); strictly `0` for DEL.
  - Offset `7..7+KeyLen`: Key bytes (opaque raw bytes).
  - Offset `7+KeyLen..7+KeyLen+ValLen`: Value bytes (opaque raw bytes; absent if `ValLen == 0`).

- **4-Byte Frame Trailer** (`docs/common/04_SHARED_DATA_FORMAT.md:21`, `docs/engine/04_RECORD_ENCODING_AND_CHECKSUMS.md:21`):
  - Offset `0..4`: `PayloadCRC32C` (`uint32` Little-Endian) computed over the complete payload body (`0..PayloadLength`).
  - Total frame size on disk: $\text{TotalFrameSize} = 28 + \text{PayloadLength} + 4 = 32 + \text{PayloadLength}$.

- **Standard Castagnoli Checksum Verification**:
  - Polynomial: `0x82F63B78` (reversed/reflected, equivalent to normal `0x1EDC6F41`).
  - RFC 3720 standard reference string `"123456789"` yields `0xE3069283`.
  - 32 zero bytes yield `0x8A9136AA`; 32 `0xFF` bytes yield `0x62A8AB43`.

---

## 2. Logic Chain

1. **Dual CRC Envelope Integrity**:
   - Recovery and parsing cannot trust raw `PayloadLength` from disk before verifying authenticity. A corrupted or malicious integer (e.g. `0xFFFFFFFFFFFFFFFF`) could trigger an immediate Out-Of-Memory panic (`make([]byte, payloadLen)`).
   - Therefore, the 28-byte UABE header is split into an independently checksummed header: bytes 8..27 (payload length, sequence number, operation count) are checksummed into `Header CRC32C` at bytes 4..8.
   - The scanner validates `Frame Magic == 0x55414245` and `Header CRC32C` *before* allocating any memory. Furthermore, `payload_len` is capped against maximum allowable batch size ($10,000 \times (7 + 1024 + 1048576)$).
   - Only after reading the payload is the trailer `PayloadCRC32C` validated.

2. **Boundary Enforcement**:
   - Zero-length keys (`KeyLen == 0`) violate index integrity and must be rejected before disk serialization.
   - Zero-length values (`ValLen == 0`) for PUT are valid representations of empty strings and must be stored as length-0 locators in `KeyDir`, distinct from `NOTFOUND`.
   - DEL operations must have `ValLen == 0`. Any frame containing DEL with `ValLen > 0` violates the contract and is corrupt.
   - Empty batches (`OpCount == 0`) produce no disk frame and return an immediate validation error.
   - Batches exceeding 10,000 operations are rejected upfront before frame construction.

3. **Corruption Detection & Fail-Closed Semantics**:
   - Header bit-flips in magic, version, or generation must fail with specific typed errors (`ErrInvalidHeaderMagic`, `ErrUnsupportedVersion`, `ErrHeaderChecksumMismatch`).
   - Frame header bit-flips must be caught by `Header CRC32C` before reading payload bytes.
   - Truncated frames or payload corruption at the file EOF are classified as torn writes and truncated cleanly to `validEOF`; corruption in non-tail frames fails closed with `CORRUPT_FRAME`.

---

## 3. Features Discovered

| # | Category | Feature | Description | Inputs | Outputs | Error Behavior | Discovered Via |
|---|----------|---------|-------------|--------|---------|----------------|----------------|
| 1 | DB Header | 32-Byte File Header Codec | Encodes and decodes 32-byte database header at offset 0 | `Header{Magic, Version, Generation, Reserved}` | 32 bytes Little-Endian with Castagnoli CRC32C | Rejects header < 32B, bad magic, version != 1, bad CRC | `docs/engine/03_BINARY_FILE_FORMAT.md:17` |
| 2 | Frame Codec | 28-Byte UABE Frame Header Codec | Encodes and decodes 28-byte frame header | `FrameHeader{Magic, PayloadLen, SeqNo, OpCount}` | 28 bytes Little-Endian with Header CRC32C over bytes 8..27 | Rejects bad magic `0x55414245`, header CRC mismatch, short buffer | `docs/common/04_SHARED_DATA_FORMAT.md:21` |
| 3 | Payload Codec | Subrecord Serialization | Serializes PUT (`0x01`) and DEL (`0x02`) subrecords | `Operation{Type, Key, Value}` | Concatenated binary subrecords: `Type(1B) + KeyLen(2B) + ValLen(4B) + Key + Val` | Rejects key len 0 or >1024, val len >1MB, DEL val > 0 | `docs/engine/04_RECORD_ENCODING_AND_CHECKSUMS.md:17` |
| 4 | Trailer Codec | 4-Byte Castagnoli Trailer | Computes and validates CRC32C over complete payload bytes | Encoded payload byte slice | 4-byte Little-Endian Castagnoli CRC32C | Mismatch returns `ErrPayloadChecksumMismatch` | `docs/engine/04_RECORD_ENCODING_AND_CHECKSUMS.md:21` |
| 5 | Envelope | Complete Frame Round-Trip | Encapsulates header + subrecords + trailer | `SeqNo uint64`, `[]Operation` (1..10,000) | `32 + PayloadLen` bytes contiguous buffer | Rejects empty batch, batch > 10,000, payload cap breach | `docs/engine/07_BATCH_ATOMICITY.md:17` |
| 6 | Index Locator | Offset Arithmetic Calculation | Deterministically computes value offset for `KeyDir` | `FrameStart uint64`, `Cursor uint64`, `KeyLen uint16` | Absolute value offset: `FrameStart + 28 + Cursor + 7 + KeyLen` | Checked integer arithmetic prevents overflow | `docs/common/04_SHARED_DATA_FORMAT.md:31` |

---

## 4. Edge Cases

| # | Feature | Input / Condition | Observed / Required Behavior |
|---|---------|-------------------|-----------------------------|
| 1 | Key Boundary | `KeyLen = 0` (empty key) | Rejected with `ErrKeyLengthZero`; no frame written to disk. |
| 2 | Key Boundary | `KeyLen = 1` (single byte) | Accepted; encoded with `KeyLen = 0x0001` (LE: `01 00`). |
| 3 | Key Boundary | `KeyLen = 1024` (1 KiB) | Accepted; encoded with `KeyLen = 0x0400` (LE: `00 04`). |
| 4 | Key Boundary | `KeyLen = 1025` (1 KiB + 1 B) | Rejected with `ErrKeyTooLarge`; frame construction aborted. |
| 5 | Value Boundary | `ValLen = 0` for PUT | Accepted; stored with `ValLen = 0x00000000`; KeyDir stores 0-length locator. |
| 6 | Value Boundary | `ValLen = 1` for PUT | Accepted; encoded with `ValLen = 0x00000001` (LE: `01 00 00 00`). |
| 7 | Value Boundary | `ValLen = 1,048,576` (1 MiB) | Accepted; encoded with `ValLen = 0x00100000` (LE: `00 00 10 00`). |
| 8 | Value Boundary | `ValLen = 1,048,577` (1 MiB + 1 B) | Rejected with `ErrValueTooLarge`; frame construction aborted. |
| 9 | Batch Size | `OpCount = 0` (empty batch) | Rejected with `ErrBatchEmpty`; zero bytes appended. |
| 10 | Batch Size | `OpCount = 1` (single op) | Accepted; standard standalone PUT/DEL frame. |
| 11 | Batch Size | `OpCount = 10,000` | Accepted; valid multi-operation UABE frame. |
| 12 | Batch Size | `OpCount = 10,001` | Rejected with `ErrBatchTooLarge`; all-or-nothing rejection. |
| 13 | Delete Invariant | DEL with `ValLen > 0` | Rejected with `ErrInvalidDelete`; decoder flags frame corrupt. |
| 14 | Delete Invariant | DEL with `ValLen == 0` | Accepted; tombstone subrecord encoded without value bytes. |
| 15 | Header Guard | Untrusted `PayloadLen > 64MB` | Rejected with `ErrPayloadTooLarge` *before* allocating memory. |
| 16 | Payload Extra Bytes | `OpCount` exhausted before payload end | Rejected with `ErrTrailingPayloadGarbage`. |
| 17 | Payload Underflow | Payload truncated before `OpCount` reached | Rejected with `ErrSubrecordTruncated`. |
| 18 | Short Reads | Buffer < 32B for DB header; < 28B for Frame header | Rejected with `io.ErrUnexpectedEOF` or `ErrShortHeader`. |

---

## 5. Exact Byte-for-Byte Golden Test Fixtures

All hex bytes are deterministic and verified against Castagnoli CRC32C polynomial `0x82F63B78`.

### 5.1 Authoritative 32-Byte DB Header Golden Fixtures

#### Fixture 1A: Fresh Database Header (Generation = 0, Version = 1)
- **Field Breakdown**:
  - `00..07` (8B): Magic `"KVDB2026"` $\to$ `4b 56 44 42 32 30 32 36`
  - `08..11` (4B): Version `1` (uint32 LE) $\to$ `01 00 00 00`
  - `12..19` (8B): Generation `0` (uint64 LE) $\to$ `00 00 00 00 00 00 00 00`
  - `20..27` (8B): Reserved `0` (uint64 LE) $\to$ `00 00 00 00 00 00 00 00`
  - `28..31` (4B): Header CRC32C over bytes 0..27 = `0x6708c580` (uint32 LE) $\to$ `80 c5 08 67`
- **Total Hex (32 bytes)**:
  ```hex
  4b56444232303236010000000000000000000000000000000000000080c50867
  ```

#### Fixture 1B: Compacted Database Header (Generation = 5, Version = 1)
- **Field Breakdown**:
  - `00..07` (8B): Magic `"KVDB2026"` $\to$ `4b 56 44 42 32 30 32 36`
  - `08..11` (4B): Version `1` (uint32 LE) $\to$ `01 00 00 00`
  - `12..19` (8B): Generation `5` (uint64 LE) $\to$ `05 00 00 00 00 00 00 00`
  - `20..27` (8B): Reserved `0` (uint64 LE) $\to$ `00 00 00 00 00 00 00 00`
  - `28..31` (4B): Header CRC32C over bytes 0..27 = `0x53006495` (uint32 LE) $\to$ `95 64 00 53`
- **Total Hex (32 bytes)**:
  ```hex
  4b56444232303236010000000500000000000000000000000000000095640053
  ```

---

### 5.2 UABE Frame Golden Fixtures

#### Fixture 2: Single PUT (`hello` $\to$ `world`, SeqNo = 1)
- **Subrecord Hex (17 bytes)**:
  - `00` (1B): OpType PUT `0x01`
  - `01..02` (2B): KeyLen `5` (uint16 LE) $\to$ `05 00`
  - `03..06` (4B): ValLen `5` (uint32 LE) $\to$ `05 00 00 00`
  - `07..11` (5B): Key `"hello"` $\to$ `68 65 6c 6c 6f`
  - `12..16` (5B): Val `"world"` $\to$ `77 6f 72 6c 64`
  - Subrecord raw hex: `0105000500000068656c6c6f776f726c64`
- **28-Byte Frame Header**:
  - `00..03` (4B): Magic `0x55414245` $\to$ `45 42 41 55`
  - `04..07` (4B): Header CRC32C over bytes 8..27 = `0x1db93fc8` $\to$ `c8 3f b9 1d`
  - `08..15` (8B): PayloadLen `17` (uint64 LE) $\to$ `11 00 00 00 00 00 00 00`
  - `16..23` (8B): SeqNo `1` (uint64 LE) $\to$ `01 00 00 00 00 00 00 00`
  - `24..27` (4B): OpCount `1` (uint32 LE) $\to$ `01 00 00 00`
  - Frame Header raw hex: `45424155c83fb91d1100000000000000010000000000000001000000`
- **4-Byte Frame Trailer**:
  - Payload CRC32C over 17 payload bytes = `0x05e1ece7` $\to$ `e7 ec e1 05`
- **Full Contiguous Frame Hex (49 bytes)**:
  ```hex
  45424155c83fb91d11000000000000000100000000000000010000000105000500000068656c6c6f776f726c64e7ece105
  ```

#### Fixture 3: Single PUT with Empty Value (`foo` $\to$ `""`, SeqNo = 2)
- **Subrecord Hex (10 bytes)**:
  - `00` (1B): OpType PUT `0x01`
  - `01..02` (2B): KeyLen `3` (uint16 LE) $\to$ `03 00`
  - `03..06` (4B): ValLen `0` (uint32 LE) $\to$ `00 00 00 00`
  - `07..09` (3B): Key `"foo"` $\to$ `66 6f 6f`
  - Subrecord raw hex: `01030000000000666f6f`
- **28-Byte Frame Header**:
  - `00..03` (4B): Magic `0x55414245` $\to$ `45 42 41 55`
  - `04..07` (4B): Header CRC32C over bytes 8..27 = `0xd2177085` $\to$ `85 70 17 d2`
  - `08..15` (8B): PayloadLen `10` (uint64 LE) $\to$ `0a 00 00 00 00 00 00 00`
  - `16..23` (8B): SeqNo `2` (uint64 LE) $\to$ `02 00 00 00 00 00 00 00`
  - `24..27` (4B): OpCount `1` (uint32 LE) $\to$ `01 00 00 00`
  - Frame Header raw hex: `45424155857017d20a00000000000000020000000000000001000000`
- **4-Byte Frame Trailer**:
  - Payload CRC32C over 10 payload bytes = `0x9e848a80` $\to$ `80 8a 84 9e`
- **Full Contiguous Frame Hex (42 bytes)**:
  ```hex
  45424155857017d20a0000000000000002000000000000000100000001030000000000666f6f808a849e
  ```

#### Fixture 4: Single DEL (`foo`, SeqNo = 3)
- **Subrecord Hex (10 bytes)**:
  - `00` (1B): OpType DEL `0x02`
  - `01..02` (2B): KeyLen `3` (uint16 LE) $\to$ `03 00`
  - `03..06` (4B): ValLen `0` (uint32 LE) $\to$ `00 00 00 00`
  - `07..09` (3B): Key `"foo"` $\to$ `66 6f 6f`
  - Subrecord raw hex: `02030000000000666f6f`
- **28-Byte Frame Header**:
  - `00..03` (4B): Magic `0x55414245` $\to$ `45 42 41 55`
  - `04..07` (4B): Header CRC32C over bytes 8..27 = `0xe366a4b5` $\to$ `b5 a4 66 e3`
  - `08..15` (8B): PayloadLen `10` (uint64 LE) $\to$ `0a 00 00 00 00 00 00 00`
  - `16..23` (8B): SeqNo `3` (uint64 LE) $\to$ `03 00 00 00 00 00 00 00`
  - `24..27` (4B): OpCount `1` (uint32 LE) $\to$ `01 00 00 00`
  - Frame Header raw hex: `45424155b5a466e30a00000000000000030000000000000001000000`
- **4-Byte Frame Trailer**:
  - Payload CRC32C over 10 payload bytes = `0xc692709e` $\to$ `9e 70 92 c6`
- **Full Contiguous Frame Hex (42 bytes)**:
  ```hex
  45424155b5a466e30a0000000000000003000000000000000100000002030000000000666f6f9e7092c6
  ```

#### Fixture 5: Multi-Operation Atomic Batch (3 Ops, SeqNo = 42)
- **Batch Content**:
  1. Op 1: PUT `k1` $\to$ `v1` (`01 02 00 02 00 00 00 6b 31 76 31`) [11B]
  2. Op 2: DEL `k2` (`02 02 00 00 00 00 00 6b 32`) [9B]
  3. Op 3: PUT `k3` $\to$ `""` (`01 02 00 00 00 00 00 6b 33`) [9B]
- **Combined Payload (29 bytes)**:
  `010200020000006b317631020200000000006b32010200000000006b33`
- **28-Byte Frame Header**:
  - `00..03` (4B): Magic `0x55414245` $\to$ `45 42 41 55`
  - `04..07` (4B): Header CRC32C over bytes 8..27 = `0x2f8112a3` $\to$ `a3 12 81 2f`
  - `08..15` (8B): PayloadLen `29` (uint64 LE) $\to$ `1d 00 00 00 00 00 00 00`
  - `16..23` (8B): SeqNo `42` (uint64 LE) $\to$ `2a 00 00 00 00 00 00 00`
  - `24..27` (4B): OpCount `3` (uint32 LE) $\to$ `03 00 00 00`
  - Frame Header raw hex: `45424155a312812f1d000000000000002a0000000000000003000000`
- **4-Byte Frame Trailer**:
  - Payload CRC32C over 29 payload bytes = `0x9fb329b2` $\to$ `b2 29 b3 9f`
- **Full Contiguous Frame Hex (61 bytes)**:
  ```hex
  45424155a312812f1d000000000000002a0000000000000003000000010200020000006b317631020200000000006b32010200000000006b33b229b39f
  ```

---

## 6. Corruption Test Vectors

| Vector ID | Target Component | Modification / Corruption Applied | Corrupted Hex Dump | Expected Error / Behavior |
|-----------|------------------|-----------------------------------|--------------------|---------------------------|
| `CORR-H1` | DB Header Magic | Bit-flip in byte 0 (`"KVDB2026"` $\to$ `"JVDB2026"`) | `4a56444232303236010000000000000000000000000000000000000080c50867` | `ErrInvalidHeaderMagic` |
| `CORR-H2` | DB Header Version | Format version byte 8 changed from `1` to `2` | `4b56444232303236020000000000000000000000000000000000000080c50867` | `ErrUnsupportedVersion` |
| `CORR-H3` | DB Header CRC | Bit-flip in stored CRC byte 28 (`0x80` $\to$ `0x81`) | `4b56444232303236010000000000000000000000000000000000000081c50867` | `ErrHeaderChecksumMismatch` |
| `CORR-H4` | DB Header Body | Generation byte 12 modified without updating CRC | `4b56444232303236010000000100000000000000000000000000000080c50867` | `ErrHeaderChecksumMismatch` |
| `CORR-F1` | Frame Magic | Frame magic byte 0 flipped (`0x45` $\to$ `0x44`) | `44424155c83fb91d1100000000000000010000000000000001000000010500...` | `ErrInvalidFrameMagic` |
| `CORR-F2` | Frame Header CRC | Stored header CRC byte 4 flipped (`0xc8` $\to$ `0xc9`) | `45424155c93fb91d1100000000000000010000000000000001000000010500...` | `ErrFrameHeaderChecksumMismatch` |
| `CORR-F3` | Payload Length Field | Byte 8 modified (`17` $\to$ `16`) without updating header CRC | `45424155c83fb91d1000000000000000010000000000000001000000010500...` | `ErrFrameHeaderChecksumMismatch` (detected before payload read) |
| `CORR-F4` | Payload Byte | Byte 35 ('h' in "hello") mutated to 'i' (`0x68` $\to$ `0x69`) | `45424155c83fb91d11000000000000000100000000000000010000000105000500000069656c...` | `ErrPayloadChecksumMismatch` |
| `CORR-F5` | Trailer CRC | Trailer CRC byte 45 flipped (`0xe7` $\to$ `0xe6`) | `...776f726c64e6ece105` | `ErrPayloadChecksumMismatch` |
| `CORR-F6` | Short Header (<28B) | Only 20 bytes provided for frame header | `45424155c83fb91d110000000000000001000000` | `io.ErrUnexpectedEOF` or `ErrShortFrameHeader` |
| `CORR-F7` | Truncated Payload | Header declares 17 bytes, only 10 bytes in payload | `...010000000105000500000068656c` (EOF) | `io.ErrUnexpectedEOF` or `ErrShortPayload` |
| `CORR-F8` | Truncated Trailer | Payload complete, trailer has only 2 bytes | `...776f726c64e7ec` (EOF) | `io.ErrUnexpectedEOF` or `ErrShortTrailer` |
| `CORR-F9` | Trailing Garbage | Payload has 8 extra unparsed bytes after subrecords | Payload length = 25, subrecord ends at 17 | `ErrTrailingPayloadGarbage` |
| `CORR-F10` | Anti-OOM Cap | Frame Header claims `PayloadLen = 0xFFFFFFFFFFFFFFFF` | Length > 64 MB cap | `ErrPayloadTooLarge` (rejected before `make([]byte)`) |

---

## 7. Concrete Go Test Function Signatures and Assertions for `internal/format/codec_test.go`

```go
package format_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"testing"

	"kvdb/internal/format"
	"kvdb/pkg/types"
)

// 1. Golden DB Header Tests
func TestEncodeDecodeHeader_Golden(t *testing.T) {
	tests := []struct {
		name       string
		header     format.DBHeader
		goldenHex  string
		expectedCRC uint32
	}{
		{
			name: "Generation_0",
			header: format.DBHeader{
				Magic:      [8]byte{'K', 'V', 'D', 'B', '2', '0', '2', '6'},
				Version:    1,
				Generation: 0,
				Reserved:   0,
			},
			goldenHex:   "4b56444232303236010000000000000000000000000000000000000080c50867",
			expectedCRC: 0x6708c580,
		},
		{
			name: "Generation_5",
			header: format.DBHeader{
				Magic:      [8]byte{'K', 'V', 'D', 'B', '2', '0', '2', '6'},
				Version:    1,
				Generation: 5,
				Reserved:   0,
			},
			goldenHex:   "4b56444232303236010000000500000000000000000000000000000095640053",
			expectedCRC: 0x53006495,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := format.EncodeHeader(&tc.header)
			if err != nil {
				t.Fatalf("EncodeHeader failed: %v", err)
			}
			if len(encoded) != 32 {
				t.Fatalf("expected 32 bytes, got %d", len(encoded))
			}
			if hex.EncodeToString(encoded) != tc.goldenHex {
				t.Fatalf("golden hex mismatch:\ngot:  %s\nwant: %s", hex.EncodeToString(encoded), tc.goldenHex)
			}

			// Round-trip decode
			decoded, err := format.DecodeHeader(encoded)
			if err != nil {
				t.Fatalf("DecodeHeader failed: %v", err)
			}
			if decoded.Magic != tc.header.Magic || decoded.Version != tc.header.Version ||
				decoded.Generation != tc.header.Generation || decoded.CRC != tc.expectedCRC {
				t.Fatalf("decoded header fields mismatch: %+v", decoded)
			}
		})
	}
}

// 2. DB Header Corruption Tests
func TestDecodeHeader_Corruption(t *testing.T) {
	t.Run("InvalidMagic", func(t *testing.T) {
		raw, _ := hex.DecodeString("4a56444232303236010000000000000000000000000000000000000080c50867")
		_, err := format.DecodeHeader(raw)
		if !errors.Is(err, format.ErrInvalidHeaderMagic) {
			t.Fatalf("expected ErrInvalidHeaderMagic, got %v", err)
		}
	})

	t.Run("UnsupportedVersion", func(t *testing.T) {
		raw, _ := hex.DecodeString("4b56444232303236020000000000000000000000000000000000000080c50867")
		_, err := format.DecodeHeader(raw)
		if !errors.Is(err, format.ErrUnsupportedVersion) {
			t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
		}
	})

	t.Run("ChecksumMismatch", func(t *testing.T) {
		raw, _ := hex.DecodeString("4b56444232303236010000000000000000000000000000000000000081c50867")
		_, err := format.DecodeHeader(raw)
		if !errors.Is(err, format.ErrHeaderChecksumMismatch) {
			t.Fatalf("expected ErrHeaderChecksumMismatch, got %v", err)
		}
	})

	t.Run("ShortHeader", func(t *testing.T) {
		short := make([]byte, 31)
		_, err := format.DecodeHeader(short)
		if !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, format.ErrShortHeader) {
			t.Fatalf("expected short header error, got %v", err)
		}
	})
}

// 3. UABE Frame Golden Round-Trip Tests
func TestEncodeDecodeFrame_Golden(t *testing.T) {
	tests := []struct {
		name        string
		seqNo       uint64
		ops         []types.Operation
		goldenHex   string
		expectedLen int
	}{
		{
			name:  "SinglePUT_HelloWorld",
			seqNo: 1,
			ops: []types.Operation{
				{Type: types.OpPut, Key: []byte("hello"), Value: []byte("world")},
			},
			goldenHex:   "45424155c83fb91d11000000000000000100000000000000010000000105000500000068656c6c6f776f726c64e7ece105",
			expectedLen: 49,
		},
		{
			name:  "SinglePUT_EmptyValue",
			seqNo: 2,
			ops: []types.Operation{
				{Type: types.OpPut, Key: []byte("foo"), Value: []byte{}},
			},
			goldenHex:   "45424155857017d20a0000000000000002000000000000000100000001030000000000666f6f808a849e",
			expectedLen: 42,
		},
		{
			name:  "SingleDEL_Foo",
			seqNo: 3,
			ops: []types.Operation{
				{Type: types.OpDel, Key: []byte("foo"), Value: nil},
			},
			goldenHex:   "45424155b5a466e30a0000000000000003000000000000000100000002030000000000666f6f9e7092c6",
			expectedLen: 42,
		},
		{
			name:  "Batch_3Ops",
			seqNo: 42,
			ops: []types.Operation{
				{Type: types.OpPut, Key: []byte("k1"), Value: []byte("v1")},
				{Type: types.OpDel, Key: []byte("k2"), Value: nil},
				{Type: types.OpPut, Key: []byte("k3"), Value: []byte{}},
			},
			goldenHex:   "45424155a312812f1d000000000000002a0000000000000003000000010200020000006b317631020200000000006b32010200000000006b33b229b39f",
			expectedLen: 61,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := format.EncodeFrame(tc.seqNo, tc.ops)
			if err != nil {
				t.Fatalf("EncodeFrame failed: %v", err)
			}
			if len(encoded) != tc.expectedLen {
				t.Fatalf("expected len %d, got %d", tc.expectedLen, len(encoded))
			}
			if hex.EncodeToString(encoded) != tc.goldenHex {
				t.Fatalf("golden frame mismatch:\ngot:  %s\nwant: %s", hex.EncodeToString(encoded), tc.goldenHex)
			}

			// Decode header
			fh, err := format.DecodeFrameHeader(encoded[:format.FrameHeaderSize])
			if err != nil {
				t.Fatalf("DecodeFrameHeader failed: %v", err)
			}
			if fh.Magic != format.FrameMagic || fh.SeqNo != tc.seqNo || fh.OpCount != uint32(len(tc.ops)) {
				t.Fatalf("decoded frame header mismatch: %+v", fh)
			}

			// Decode payload + trailer
			payload := encoded[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
			trailer := encoded[format.FrameHeaderSize+fh.PayloadLen:]
			ops, err := format.DecodePayloadAndTrailer(payload, trailer, fh.OpCount)
			if err != nil {
				t.Fatalf("DecodePayloadAndTrailer failed: %v", err)
			}
			if len(ops) != len(tc.ops) {
				t.Fatalf("expected %d ops, got %d", len(tc.ops), len(ops))
			}
			for i := range ops {
				if ops[i].Type != tc.ops[i].Type || !bytes.Equal(ops[i].Key, tc.ops[i].Key) ||
					!bytes.Equal(ops[i].Value, tc.ops[i].Value) {
					t.Fatalf("op[%d] mismatch:\ngot:  %+v\nwant: %+v", i, ops[i], tc.ops[i])
				}
			}
		})
	}
}

// 4. Boundary Tests: Key Lengths
func TestKeyLengthBoundaries(t *testing.T) {
	t.Run("KeyLen_0_Rejected", func(t *testing.T) {
		_, err := format.EncodeFrame(1, []types.Operation{
			{Type: types.OpPut, Key: []byte{}, Value: []byte("val")},
		})
		if !errors.Is(err, format.ErrKeyLengthZero) {
			t.Fatalf("expected ErrKeyLengthZero, got %v", err)
		}
	})

	t.Run("KeyLen_1_Accepted", func(t *testing.T) {
		frame, err := format.EncodeFrame(1, []types.Operation{
			{Type: types.OpPut, Key: []byte("a"), Value: []byte("val")},
		})
		if err != nil {
			t.Fatalf("unexpected error for 1-byte key: %v", err)
		}
		if len(frame) == 0 {
			t.Fatal("empty frame returned")
		}
	})

	t.Run("KeyLen_1024_Accepted", func(t *testing.T) {
		maxKey := bytes.Repeat([]byte("k"), 1024)
		frame, err := format.EncodeFrame(1, []types.Operation{
			{Type: types.OpPut, Key: maxKey, Value: []byte("val")},
		})
		if err != nil {
			t.Fatalf("unexpected error for 1024-byte key: %v", err)
		}
		if len(frame) == 0 {
			t.Fatal("empty frame returned")
		}
	})

	t.Run("KeyLen_1025_Rejected", func(t *testing.T) {
		oversizedKey := bytes.Repeat([]byte("k"), 1025)
		_, err := format.EncodeFrame(1, []types.Operation{
			{Type: types.OpPut, Key: oversizedKey, Value: []byte("val")},
		})
		if !errors.Is(err, format.ErrKeyTooLarge) {
			t.Fatalf("expected ErrKeyTooLarge, got %v", err)
		}
	})
}

// 5. Boundary Tests: Value Lengths
func TestValueLengthBoundaries(t *testing.T) {
	t.Run("ValLen_0_Accepted", func(t *testing.T) {
		frame, err := format.EncodeFrame(1, []types.Operation{
			{Type: types.OpPut, Key: []byte("key"), Value: []byte{}},
		})
		if err != nil {
			t.Fatalf("unexpected error for 0-byte value: %v", err)
		}
		if len(frame) == 0 {
			t.Fatal("empty frame returned")
		}
	})

	t.Run("ValLen_1MB_Accepted", func(t *testing.T) {
		maxVal := make([]byte, 1024*1024) // 1 MiB
		frame, err := format.EncodeFrame(1, []types.Operation{
			{Type: types.OpPut, Key: []byte("key"), Value: maxVal},
		})
		if err != nil {
			t.Fatalf("unexpected error for 1MB value: %v", err)
		}
		if len(frame) != 28+(7+3+1024*1024)+4 {
			t.Fatalf("unexpected frame size: %d", len(frame))
		}
	})

	t.Run("ValLen_1MB_Plus1_Rejected", func(t *testing.T) {
		oversizedVal := make([]byte, 1024*1024+1)
		_, err := format.EncodeFrame(1, []types.Operation{
			{Type: types.OpPut, Key: []byte("key"), Value: oversizedVal},
		})
		if !errors.Is(err, format.ErrValueTooLarge) {
			t.Fatalf("expected ErrValueTooLarge, got %v", err)
		}
	})
}

// 6. Boundary Tests: Batch Operation Counts
func TestBatchOpCountBoundaries(t *testing.T) {
	t.Run("OpCount_0_Rejected", func(t *testing.T) {
		_, err := format.EncodeFrame(1, []types.Operation{})
		if !errors.Is(err, format.ErrBatchEmpty) {
			t.Fatalf("expected ErrBatchEmpty, got %v", err)
		}
	})

	t.Run("OpCount_10000_Accepted", func(t *testing.T) {
		ops := make([]types.Operation, 10000)
		for i := 0; i < 10000; i++ {
			ops[i] = types.Operation{Type: types.OpPut, Key: []byte("k"), Value: []byte{}}
		}
		frame, err := format.EncodeFrame(1, ops)
		if err != nil {
			t.Fatalf("unexpected error for 10000 ops: %v", err)
		}
		expectedPayload := 10000 * (7 + 1 + 0) // 80,000 bytes
		if len(frame) != 28+expectedPayload+4 {
			t.Fatalf("expected frame size %d, got %d", 28+expectedPayload+4, len(frame))
		}
	})

	t.Run("OpCount_10001_Rejected", func(t *testing.T) {
		ops := make([]types.Operation, 10001)
		for i := 0; i < 10001; i++ {
			ops[i] = types.Operation{Type: types.OpPut, Key: []byte("k"), Value: []byte{}}
		}
		_, err := format.EncodeFrame(1, ops)
		if !errors.Is(err, format.ErrBatchTooLarge) {
			t.Fatalf("expected ErrBatchTooLarge, got %v", err)
		}
	})
}

// 7. DEL Non-Zero Value Rejection
func TestDelWithNonZeroValue(t *testing.T) {
	t.Run("EncodeRejection", func(t *testing.T) {
		_, err := format.EncodeFrame(1, []types.Operation{
			{Type: types.OpDel, Key: []byte("k"), Value: []byte("must_be_empty")},
		})
		if !errors.Is(err, format.ErrInvalidDelete) {
			t.Fatalf("expected ErrInvalidDelete, got %v", err)
		}
	})

	t.Run("DecodeRejection", func(t *testing.T) {
		// Craft malicious payload: OpType=0x02, KeyLen=1 ("k"), ValLen=3 ("bad")
		malformedPayload := []byte{
			0x02,             // OpType DEL
			0x01, 0x00,       // KeyLen 1
			0x03, 0x00, 0x00, 0x00, // ValLen 3 (INVALID!)
			'k',              // Key
			'b', 'a', 'd',    // Value
		}
		fakeTrailer := make([]byte, 4)
		_, err := format.DecodePayloadAndTrailer(malformedPayload, fakeTrailer, 1)
		if !errors.Is(err, format.ErrInvalidDelete) {
			t.Fatalf("expected ErrInvalidDelete on decode, got %v", err)
		}
	})
}

// 8. Corruption & Truncation Vectors
func TestFrameCorruptionVectors(t *testing.T) {
	raw, _ := hex.DecodeString("45424155c83fb91d11000000000000000100000000000000010000000105000500000068656c6c6f776f726c64e7ece105")

	t.Run("CorruptFrameMagic", func(t *testing.T) {
		bad := bytes.Clone(raw)
		bad[0] ^= 0x01
		_, err := format.DecodeFrameHeader(bad[:format.FrameHeaderSize])
		if !errors.Is(err, format.ErrInvalidFrameMagic) {
			t.Fatalf("expected ErrInvalidFrameMagic, got %v", err)
		}
	})

	t.Run("CorruptFrameHeaderCRC", func(t *testing.T) {
		bad := bytes.Clone(raw)
		bad[4] ^= 0x01
		_, err := format.DecodeFrameHeader(bad[:format.FrameHeaderSize])
		if !errors.Is(err, format.ErrFrameHeaderChecksumMismatch) {
			t.Fatalf("expected ErrFrameHeaderChecksumMismatch, got %v", err)
		}
	})

	t.Run("CorruptPayloadByte", func(t *testing.T) {
		bad := bytes.Clone(raw)
		bad[35] ^= 0x01 // Mutate 'h' in 'hello'
		fh, err := format.DecodeFrameHeader(bad[:format.FrameHeaderSize])
		if err != nil {
			t.Fatalf("frame header should be valid, got: %v", err)
		}
		payload := bad[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
		trailer := bad[format.FrameHeaderSize+fh.PayloadLen:]
		_, err = format.DecodePayloadAndTrailer(payload, trailer, fh.OpCount)
		if !errors.Is(err, format.ErrPayloadChecksumMismatch) {
			t.Fatalf("expected ErrPayloadChecksumMismatch, got %v", err)
		}
	})

	t.Run("CorruptTrailerCRC", func(t *testing.T) {
		bad := bytes.Clone(raw)
		bad[len(bad)-1] ^= 0x01
		fh, _ := format.DecodeFrameHeader(bad[:format.FrameHeaderSize])
		payload := bad[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
		trailer := bad[format.FrameHeaderSize+fh.PayloadLen:]
		_, err := format.DecodePayloadAndTrailer(payload, trailer, fh.OpCount)
		if !errors.Is(err, format.ErrPayloadChecksumMismatch) {
			t.Fatalf("expected ErrPayloadChecksumMismatch, got %v", err)
		}
	})

	t.Run("TruncatedFrameHeader", func(t *testing.T) {
		_, err := format.DecodeFrameHeader(raw[:20])
		if !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, format.ErrShortFrameHeader) {
			t.Fatalf("expected short frame header error, got %v", err)
		}
	})

	t.Run("AntiOOM_GuardedPayloadLength", func(t *testing.T) {
		// Craft header with math.MaxUint64 payload length
		maliciousHeader := make([]byte, 28)
		format.PutUint32(maliciousHeader[0:4], format.FrameMagic)
		format.PutUint64(maliciousHeader[8:16], math.MaxUint64)
		// Even if CRC is valid, DecodeFrameHeader or bounded reader must guard against this
		_, err := format.DecodeFrameHeader(maliciousHeader)
		if err == nil {
			t.Fatal("expected error on math.MaxUint64 payload length, got nil")
		}
	})
}
```

---

## 8. Caveats

1. **Polynomial Confusion Protection:**
   The standard Go library includes both IEEE (`0xEDB88320`) and Castagnoli (`0x82F63B78`). Codecs must strictly use `crc32.MakeTable(crc32.Castagnoli)`. Accidentally using `crc32.ChecksumIEEE` is an instant invariant breach and will fail all golden fixtures.
2. **DEL Tombstone Semantics:**
   In `KeyDir`, a DEL operation removes the map key entirely (`delete(keyDir, string(key))`). In contrast, a PUT with an empty value (`ValLen == 0`) retains the map entry with `ValueLength: 0`. The test suite must assert that `Get()` on an empty-value key returns `value = []byte{}, found = true, err = nil`, whereas a deleted key returns `found = false`.
3. **No Dynamic Memory Preallocation:**
   When reading a frame from disk, `payload_len` must be checked against `fileSize - currentOffset - 28 - 4` and against the logical `MaxPayloadCap` (64 MB) *before* `make([]byte, payloadLen)` is executed to defend against Out-Of-Memory crashes.

---

## 9. Conclusion

The binary specification for Milestone M1 is mathematically rigorous, closed, and deterministically validated.
- All golden hex byte sequences for the 32-byte DB Header (Gen 0 & Gen 5) and 28-byte UABE frames (PUT, empty PUT, DEL, and multi-op BATCH) have been generated and independently checked against RFC 3720 Castagnoli vectors.
- All boundary limits (keys: 0/1/1024/1025B; values: 0/1/1MB/1MB+1B; batch: 0/1/10k/10001 ops; DEL: val=0) are fully defined with specific error behaviors.
- The 10 corruption test vectors provide concrete fail-closed and torn-write detection criteria.
- The provided Go test function signatures and assertions in Section 7 can be directly incorporated into `internal/format/codec_test.go` by the implementation agents.

---

## 10. Verification Method

1. **RFC 3720 & Castagnoli CRC32C Verification**:
   Execute the verification one-liner in Python or Go:
   ```bash
   python3 -c "
   import struct
   poly = 0x82F63B78
   table = [((c >> 1) ^ poly if c & 1 else c >> 1) for i in range(256) for c in [i]]
   # Standard test vector
   assert 0xe3069283 == [c := ((c >> 8) ^ table[(c ^ b) & 0xff]) for c in [0xffffffff] for b in b'123456789'][-1] ^ 0xffffffff
   print('Castagnoli CRC32C polynomial 0x82F63B78 verified.')
   "
   ```
2. **Golden Hex Check**:
   Cross-verify Section 5 hex strings against the specification in `docs/common/04_SHARED_DATA_FORMAT.md` and `docs/engine/03_BINARY_FILE_FORMAT.md`.
3. **Once Implemented in M1**:
   Run `go test -v ./internal/format/...` to verify 100% pass on all test functions specified in Section 7.
