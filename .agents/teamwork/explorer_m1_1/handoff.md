# Milestone M1 (Core Foundation & Codecs: Phases E0 and E1) Implementation Strategy Report

> **Identity:** `explorer_m1_1`  
> **Milestone:** M1 — Core Foundation & Codecs (Phases E0 & E1)  
> **Scope Document:** `.agents/teamwork/PROJECT.md`  
> **Date:** 2026-11-06  
> **Working Directory:** `/home/laik/workspace/srinathonn/.agents/teamwork/explorer_m1_1/`  
> **Target Package:** Go Key-Value Storage Engine (`kvdb`)  

---

## 1. Observation

Direct observations extracted from the authoritative specifications, rules, codebase state, and compiler toolchains:

### 1.1 Current Repository & Workspace State
- **Workspace root:** `/home/laik/workspace/srinathonn`
- **Go toolchain:** `go version go1.27.3-X:nodwarf5 linux/amd64` installed at `/usr/bin/go`.
- **Existing Go code:** Zero Go files (`*.go`), zero `go.mod`, zero `Makefile`, zero `.gitignore` exist in the repository root or subdirectories.
- **Git status:** Branch `main` tracking `origin/main`. Behind `origin/main` by commit `2a229d0` ("merge: preserve local memory and prompt updates"). Local uncommitted changes exist in `MEMORY.md`, `PROMPTS.md`, `.agents/.memory_updater_state.json`, and `.agents/.prompt_logger_state.json`.

### 1.2 Module & Build Constraints
- `docs/engine/PHASES.md:27-33`:
  > "Phase E0 — Contract and package setup. Implement: Read common/04_SHARED_DATA_FORMAT.md and common/05_CLI_AND_PROTOCOL_SPEC.md. Create Go module and package boundaries. Agree exact integer types, byte order, CRC32C coverage and error codes... Package builds offline. Design decisions are committed and reviewed."
- `.agents/rules/02_banned_libraries.md:6-14`:
  > "NO third-party database, storage engine, or persistence libraries (SQLite, LevelDB, RocksDB, LMDB, BoltDB, Badger, etc.). Standard library only. NO network access (100% offline)."
- `.agents/rules/08_user_experience_and_tooling.md:7-8`:
  > "Single-Command Build & Verification (`make demo-ready` or `./demo.sh`): One clean script that builds from scratch, runs a 50-iteration sanity torture test, and verifies all 8 `./kvdb` subcommands in under 20 seconds."

### 1.3 Exact Binary Format Specifications
- `docs/common/04_SHARED_DATA_FORMAT.md:17`:
  > "All persisted multi-byte integers are little-endian. The authoritative file begins with a fixed 32-byte header. Header offsets: `0..8` magic bytes `KVDB2026`; `8..12` format version (`u32`, initially 1); `12..20` compaction generation (`u64`); `20..28` reserved flags (`u64`, zero); `28..32` header CRC32C (`u32`) over bytes `0..28`."
- `docs/common/04_SHARED_DATA_FORMAT.md:21`:
  > "Each UABE frame has a 28-byte fixed header, payload, and 4-byte trailer. Frame header offsets relative to frame: `0..4` magic `0x55414245` (`UABE`, u32); `4..8` header CRC32C over bytes `8..28`; `8..16` payload length (`u64`); `16..24` sequence number (`u64`); `24..28` operation count (`u32`). Payload starts at frame offset 28. Trailer is the four-byte CRC32C of the entire payload. Total frame size is `32 + payload_len`."
- `docs/engine/04_RECORD_ENCODING_AND_CHECKSUMS.md:17-29`:
  > "Payload is a concatenation of exactly `op_count` operation subrecords. Each starts with `op_type:u8`, `key_len:u16`, `value_len:u32`, then `key[key_len]`, then `value[value_len]`. PUT type is 0x01; DEL type is 0x02. Key length is 1–1,024. Value length is 0–1,048,576. DEL requires value length zero... CRC32C uses the Castagnoli polynomial. Do not accidentally use IEEE CRC32. In Go, `hash/crc32` with `crc32.Castagnoli` is the intended standard-library primitive."

### 1.4 Code Layout Specification
- `.agents/teamwork/PROJECT.md:14-43` defines:
  ```
  ├── go.mod
  ├── Makefile
  ├── cmd/
  │   └── kvdb/main.go
  ├── internal/
  │   ├── format/{header,uabe,record,errors}.go
  │   ├── engine/{engine,lock,write,read,keydir,batch,recovery,compaction,quota}.go
  │   ├── protocol/serve.go
  │   └── testutil/fixtures.go
  └── tests/e2e/
  ```

### 1.5 Verified Checksum Vectors
Using Go stdlib `hash/crc32` (`crc32.Castagnoli`) and cross-checked against a Python Castagnoli table (`0x82F63B78`):
1. **Initial 32-Byte Header:**
   - Pre-CRC bytes (28 bytes): `4b564442323032360100000000000000000000000000000000000000`
   - CRC32C: `0x6708c580` (Little-Endian: `80 c5 08 67`)
   - Full Header (32 bytes): `4b56444232303236010000000000000000000000000000000000000080c50867`
2. **Single PUT Frame (Key: `"foo"`, Value: `"bar"`, Seq: 1, OpCount: 1):**
   - Payload: `01030003000000666f6f626172` (13 bytes)
   - Header CRC32C (over bytes 8..28): `0x054ecae6` (Little-Endian: `e6 ca 4e 05`)
   - Trailer CRC32C (over payload): `0xdd783b3a` (Little-Endian: `3a 3b 78 dd`)
   - Full Frame Hex (45 bytes): `45424155e6ca4e050d0000000000000001000000000000000100000001030003000000666f6f6261723a3b78dd`
3. **Single DEL Frame (Key: `"foo"`, Seq: 2, OpCount: 1):**
   - Payload: `02030000000000666f6f` (10 bytes)
   - Header CRC32C: `0xd2177085` (Little-Endian: `85 70 17 d2`)
   - Trailer CRC32C: `0xc692709e` (Little-Endian: `9e 70 92 c6`)
   - Full Frame Hex (42 bytes): `45424155857017d20a0000000000000002000000000000000100000002030000000000666f6f9e7092c6`

---

## 2. Logic Chain

1. **Standard Library Purity & Offline Compilation:**
   - Based on Rule 02 (`02_banned_libraries.md`) and Observation 1.2, any external module dependency or network fetch during build will trigger automatic disqualification.
   - Therefore, `go.mod` must declare only `module kvdb` and `go 1.27` with no external `require` directives.
   - The build process must run with `GOPROXY=off` and `CGO_ENABLED=0` to guarantee offline, reproducible compilation of static binaries without relying on C dynamic libraries or proxy downloads.

2. **Phase Separation & M1 Scope Boundary:**
   - In accordance with `PROJECT.md:76` and `docs/engine/PHASES.md:24-75`, Milestone M1 is strictly bounded to Phases E0 (Contract & package setup) and E1 (DB & UABE Codecs).
   - Milestone M1 must deliver pure serialization/deserialization logic in `internal/format/`, binary test fixtures in `internal/testutil/`, an `internal/engine/` skeleton establishing types and interfaces, and a stub `cmd/kvdb/main.go` that enables `make build` and `make test` to pass cleanly.
   - File system mutation, lock acquisition, write loops, log recovery, and compaction are explicitly reserved for M2 and M3 to maintain phase discipline.

3. **Defensive Parsing & Memory Safety (Zero Panic / Anti-OOM):**
   - Untrusted byte inputs during recovery or file scanning could contain arbitrary or corrupted `payload_len` values (e.g. `0xFFFFFFFFFFFFFFFF`).
   - If an allocation `make([]byte, payload_len)` were attempted before validating headers or bounds, the engine would crash with an unhandled Out-Of-Memory panic.
   - Therefore, the decoding logic in `internal/format/uabe.go` must enforce strict staged validation:
     1. Validate slice length $\ge 28$ bytes.
     2. Validate magic bytes `0x55414245`.
     3. Verify `HeaderCRC32C` over bytes 8..28 before inspecting or trusting `payload_len`, `seq_no`, or `op_count`.
     4. Enforce hard bounds on `payload_len`: reject if `payload_len > MaxPayloadLength` (64 MiB) or if `payload_len < op_count * 7`.
     5. Enforce `1 <= op_count <= 10,000`.
     6. Only after passing header validation and checksums is the payload buffer read and its trailer `PayloadCRC32C` verified.
     7. Parse subrecords sequentially, validating `op_type` (0x01 or 0x02), `1 <= key_len <= 1,024`, `value_len <= 1,048,576`, and strictly `value_len == 0` for `DEL`.
     8. Require exact byte consumption (`remaining == 0`) with zero trailing junk.

4. **Endianness & Checksum Consistency:**
   - All multi-byte integers (`uint16`, `uint32`, `uint64`) must use `encoding/binary.LittleEndian`.
   - Checksums must strictly use `hash/crc32` configured with `crc32.MakeTable(crc32.Castagnoli)`.
   - The table must be initialized once as a package-level constant/singleton to eliminate allocation overhead during hot serialization loops.

---

## 3. Concrete Implementation Strategy for Worker

The Worker should create and organize the files exactly as detailed below.

### 3.1 Pre-Execution Git Alignment
Before creating any files:
1. Reconcile or stash local modifications to `MEMORY.md` and `PROMPTS.md` resulting from previous agent activity.
2. Synchronize with remote commits using `git pull --rebase` or `git pull` per Rule 09.

### 3.2 Root `go.mod`
Create `/home/laik/workspace/srinathonn/go.mod`:
```go
module kvdb

go 1.27
```
- No external dependencies.
- No `go.sum` needed for stdlib-only.

### 3.3 Root `Makefile`
Create `/home/laik/workspace/srinathonn/Makefile`:
```makefile
.PHONY: all build test clean demo-ready

GO ?= go
BINARY := kvdb

all: build

build:
	CGO_ENABLED=0 GOPROXY=off $(GO) build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/kvdb

test:
	GOPROXY=off $(GO) test -v -count=1 ./...

clean:
	rm -f $(BINARY) torture
	rm -f *.db *.lock *.wal *.tmp *.compact.tmp
	rm -rf testdata/tmp

demo-ready: build test
	@echo "Demo ready: build and test passed offline."
```

### 3.4 Root `.gitignore`
Create `/home/laik/workspace/srinathonn/.gitignore`:
```gitignore
# Executables
/kvdb
/torture

# Database files & locks
*.db
*.db.lock
*.lock
*.wal
*.tmp
*.compact.tmp

# Test data & artifacts
testdata/tmp/
*.test
*.prof
*.out

# Python cache & tooling
__pycache__/
*.py[cod]
*$py.class
.pytest_cache/

# OS / Editor artifacts
.DS_Store
.idea/
.vscode/
```

### 3.5 Package `internal/format` (Codecs & Validation)

#### 1. `internal/format/errors.go`
Defines format errors conforming to `docs/engine/12_DISK_FULL_AND_ERROR_HANDLING.md`:
```go
package format

import "errors"

var (
	ErrCorruptHeader   = errors.New("CORRUPT_HEADER: invalid magic, version, or header crc")
	ErrCorruptFrame    = errors.New("CORRUPT_FRAME: invalid frame magic, header crc, or trailer crc")
	ErrKeyTooLarge     = errors.New("KEY_TOO_LARGE: key length must be between 1 and 1024 bytes")
	ErrValueTooLarge   = errors.New("VALUE_TOO_LARGE: value length exceeds 1048576 bytes")
	ErrBatchTooLarge   = errors.New("BATCH_TOO_LARGE: operation count must be between 1 and 10000")
	ErrInvalidOpType   = errors.New("INVALID_ARGUMENT: unknown operation type")
	ErrInvalidDelValue = errors.New("INVALID_ARGUMENT: DEL operation requires value length zero")
	ErrPayloadOverflow = errors.New("INVALID_ARGUMENT: calculated payload length exceeds maximum allowed bounds")
	ErrTruncatedFrame  = errors.New("CORRUPT_FRAME: buffer truncated before complete frame")
	ErrTrailingBytes   = errors.New("CORRUPT_FRAME: extraneous payload bytes after operations")
)
```

#### 2. `internal/format/header.go`
Defines 32-byte Database Header serialization and Castagnoli CRC32C:
```go
package format

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
)

const (
	HeaderSize    = 32
	HeaderVersion = uint32(1)
)

var (
	HeaderMagic  = [8]byte{'K', 'V', 'D', 'B', '2', '0', '2', '6'}
	crcTableCast = crc32.MakeTable(crc32.Castagnoli)
)

type Header struct {
	Magic         [8]byte
	Version       uint32
	CompactionGen uint64
	Reserved      uint64
	CRC32C        uint32
}

func EncodeHeader(h *Header) ([]byte, error) {
	buf := make([]byte, HeaderSize)
	copy(buf[0:8], h.Magic[:])
	binary.LittleEndian.PutUint32(buf[8:12], h.Version)
	binary.LittleEndian.PutUint64(buf[12:20], h.CompactionGen)
	binary.LittleEndian.PutUint64(buf[20:28], h.Reserved)

	checksum := crc32.Checksum(buf[0:28], crcTableCast)
	binary.LittleEndian.PutUint32(buf[28:32], checksum)
	return buf, nil
}

func DecodeHeader(b []byte) (*Header, error) {
	if len(b) < HeaderSize {
		return nil, ErrCorruptHeader
	}
	if !bytes.Equal(b[0:8], HeaderMagic[:]) {
		return nil, ErrCorruptHeader
	}
	version := binary.LittleEndian.Uint32(b[8:12])
	if version != HeaderVersion {
		return nil, ErrCorruptHeader
	}
	compactionGen := binary.LittleEndian.Uint64(b[12:20])
	reserved := binary.LittleEndian.Uint64(b[20:28])
	if reserved != 0 {
		return nil, ErrCorruptHeader
	}
	storedCRC := binary.LittleEndian.Uint32(b[28:32])
	computedCRC := crc32.Checksum(b[0:28], crcTableCast)
	if storedCRC != computedCRC {
		return nil, ErrCorruptHeader
	}

	h := &Header{
		Magic:         HeaderMagic,
		Version:       version,
		CompactionGen: compactionGen,
		Reserved:      reserved,
		CRC32C:        storedCRC,
	}
	return h, nil
}
```

#### 3. `internal/format/record.go`
Defines PUT and DEL subrecords and operations:
```go
package format

import (
	"encoding/binary"
	"math"
)

type OpType uint8

const (
	OpPut OpType = 0x01
	OpDel OpType = 0x02

	MinKeyLen      = 1
	MaxKeyLen      = 1024
	MaxValLen      = 1048576 // 1 MiB
	MaxBatchOps    = 10000
	MaxPayloadLen  = 64 * 1024 * 1024 // 64 MiB guard
	OpHeaderSize   = 7
)

type Operation struct {
	Type  OpType
	Key   []byte
	Value []byte
}

func ValidateOperation(op *Operation) error {
	if op.Type != OpPut && op.Type != OpDel {
		return ErrInvalidOpType
	}
	if len(op.Key) < MinKeyLen || len(op.Key) > MaxKeyLen {
		return ErrKeyTooLarge
	}
	if len(op.Value) > MaxValLen {
		return ErrValueTooLarge
	}
	if op.Type == OpDel && len(op.Value) != 0 {
		return ErrInvalidDelValue
	}
	return nil
}

func CalculatePayloadLength(ops []Operation) (uint64, error) {
	if len(ops) < 1 || len(ops) > MaxBatchOps {
		return 0, ErrBatchTooLarge
	}
	var total uint64
	for i := range ops {
		if err := ValidateOperation(&ops[i]); err != nil {
			return 0, err
		}
		subLen := uint64(OpHeaderSize) + uint64(len(ops[i].Key)) + uint64(len(ops[i].Value))
		if math.MaxUint64-total < subLen {
			return 0, ErrPayloadOverflow
		}
		total += subLen
	}
	if total > MaxPayloadLen {
		return 0, ErrPayloadOverflow
	}
	return total, nil
}

func EncodePayload(ops []Operation, dst []byte) (int, error) {
	offset := 0
	for i := range ops {
		dst[offset] = byte(ops[i].Type)
		offset++
		binary.LittleEndian.PutUint16(dst[offset:offset+2], uint16(len(ops[i].Key)))
		offset += 2
		binary.LittleEndian.PutUint32(dst[offset:offset+4], uint32(len(ops[i].Value)))
		offset += 4
		copy(dst[offset:], ops[i].Key)
		offset += len(ops[i].Key)
		copy(dst[offset:], ops[i].Value)
		offset += len(ops[i].Value)
	}
	return offset, nil
}

func DecodePayload(payload []byte, opCount uint32) ([]Operation, error) {
	if opCount < 1 || opCount > MaxBatchOps {
		return nil, ErrBatchTooLarge
	}
	ops := make([]Operation, 0, opCount)
	offset := 0
	payloadLen := len(payload)

	for i := uint32(0); i < opCount; i++ {
		if offset+OpHeaderSize > payloadLen {
			return nil, ErrTruncatedFrame
		}
		opType := OpType(payload[offset])
		keyLen := int(binary.LittleEndian.Uint16(payload[offset+1 : offset+3]))
		valLen := int(binary.LittleEndian.Uint32(payload[offset+3 : offset+7]))
		offset += OpHeaderSize

		if opType != OpPut && opType != OpDel {
			return nil, ErrInvalidOpType
		}
		if keyLen < MinKeyLen || keyLen > MaxKeyLen {
			return nil, ErrKeyTooLarge
		}
		if valLen > MaxValLen {
			return nil, ErrValueTooLarge
		}
		if opType == OpDel && valLen != 0 {
			return nil, ErrInvalidDelValue
		}
		if offset+keyLen+valLen > payloadLen {
			return nil, ErrTruncatedFrame
		}

		key := make([]byte, keyLen)
		copy(key, payload[offset:offset+keyLen])
		offset += keyLen

		var val []byte
		if valLen > 0 {
			val = make([]byte, valLen)
			copy(val, payload[offset:offset+valLen])
			offset += valLen
		} else {
			val = []byte{}
		}

		ops = append(ops, Operation{
			Type:  opType,
			Key:   key,
			Value: val,
		})
	}

	if offset != payloadLen {
		return nil, ErrTrailingBytes
	}
	return ops, nil
}
```

#### 4. `internal/format/uabe.go`
Defines 28-byte UABE Frame Header and 4-byte CRC32C Trailer:
```go
package format

import (
	"encoding/binary"
	"hash/crc32"
)

const (
	FrameMagic     = uint32(0x55414245)
	FrameHeaderLen = 28
	FrameTrailerLen = 4
	FrameEnvelopeLen = FrameHeaderLen + FrameTrailerLen // 32 bytes
)

type FrameHeader struct {
	Magic      uint32
	HeaderCRC  uint32
	PayloadLen uint64
	SeqNo      uint64
	OpCount    uint32
}

func EncodeFrame(seqNo uint64, ops []Operation) ([]byte, error) {
	payloadLen, err := CalculatePayloadLength(ops)
	if err != nil {
		return nil, err
	}
	totalLen := FrameHeaderLen + payloadLen + FrameTrailerLen
	buf := make([]byte, totalLen)

	// Encode Frame Header
	binary.LittleEndian.PutUint32(buf[0:4], FrameMagic)
	binary.LittleEndian.PutUint64(buf[8:16], payloadLen)
	binary.LittleEndian.PutUint64(buf[16:24], seqNo)
	binary.LittleEndian.PutUint32(buf[24:28], uint32(len(ops)))

	// Header CRC over bytes 8..28
	headerCRC := crc32.Checksum(buf[8:28], crcTableCast)
	binary.LittleEndian.PutUint32(buf[4:8], headerCRC)

	// Encode Payload
	_, err = EncodePayload(ops, buf[FrameHeaderLen:FrameHeaderLen+payloadLen])
	if err != nil {
		return nil, err
	}

	// Payload Trailer CRC
	trailerCRC := crc32.Checksum(buf[FrameHeaderLen:FrameHeaderLen+payloadLen], crcTableCast)
	binary.LittleEndian.PutUint32(buf[FrameHeaderLen+payloadLen:totalLen], trailerCRC)

	return buf, nil
}

func DecodeFrameHeader(b []byte) (*FrameHeader, error) {
	if len(b) < FrameHeaderLen {
		return nil, ErrTruncatedFrame
	}
	magic := binary.LittleEndian.Uint32(b[0:4])
	if magic != FrameMagic {
		return nil, ErrCorruptFrame
	}
	storedHeaderCRC := binary.LittleEndian.Uint32(b[4:8])
	computedHeaderCRC := crc32.Checksum(b[8:28], crcTableCast)
	if storedHeaderCRC != computedHeaderCRC {
		return nil, ErrCorruptFrame
	}

	payloadLen := binary.LittleEndian.Uint64(b[8:16])
	seqNo := binary.LittleEndian.Uint64(b[16:24])
	opCount := binary.LittleEndian.Uint32(b[24:28])

	if opCount < 1 || opCount > MaxBatchOps {
		return nil, ErrBatchTooLarge
	}
	if payloadLen > MaxPayloadLen || payloadLen < uint64(opCount*OpHeaderSize) {
		return nil, ErrCorruptFrame
	}

	return &FrameHeader{
		Magic:      magic,
		HeaderCRC:  storedHeaderCRC,
		PayloadLen: payloadLen,
		SeqNo:      seqNo,
		OpCount:    opCount,
	}, nil
}

func DecodePayloadAndTrailer(payload []byte, trailer []byte, opCount uint32) ([]Operation, error) {
	if len(trailer) < FrameTrailerLen {
		return nil, ErrTruncatedFrame
	}
	storedTrailerCRC := binary.LittleEndian.Uint32(trailer[0:4])
	computedTrailerCRC := crc32.Checksum(payload, crcTableCast)
	if storedTrailerCRC != computedTrailerCRC {
		return nil, ErrCorruptFrame
	}

	return DecodePayload(payload, opCount)
}

func DecodeFullFrame(b []byte) (*FrameHeader, []Operation, error) {
	fh, err := DecodeFrameHeader(b)
	if err != nil {
		return nil, nil, err
	}
	totalLen := FrameHeaderLen + fh.PayloadLen + FrameTrailerLen
	if uint64(len(b)) < totalLen {
		return nil, nil, ErrTruncatedFrame
	}

	payload := b[FrameHeaderLen : FrameHeaderLen+fh.PayloadLen]
	trailer := b[FrameHeaderLen+fh.PayloadLen : totalLen]

	ops, err := DecodePayloadAndTrailer(payload, trailer, fh.OpCount)
	if err != nil {
		return nil, nil, err
	}
	return fh, ops, nil
}
```

### 3.6 Package `internal/engine` (Skeleton)
Create `/home/laik/workspace/srinathonn/internal/engine/engine.go`:
```go
package engine

import (
	"errors"
	"os"
	"sync"

	"kvdb/internal/format"
)

var (
	ErrLocked         = errors.New("LOCKED: database lock held by another process")
	ErrClosed         = errors.New("CLOSED: engine instance has been closed")
	ErrNotFound       = errors.New("NOTFOUND: key not present in database")
	ErrDiskFull       = errors.New("DISK_FULL: file size limit exceeded")
	ErrEnginePoisoned = errors.New("ENGINE_POISONED: previous write rollback failed")
)

type KeyDirEntry struct {
	FileOffset uint64
	ValueLen   uint32
	SeqNo      uint64
}

type Pair struct {
	Key   string
	Value []byte
}

type Options struct {
	MaxBytes uint64
}

type Engine struct {
	mu            sync.RWMutex
	db            *os.File
	lockFile      *os.File
	path          string
	parentDir     string
	maxBytes      uint64
	validEOF      uint64
	nextSeqNo     uint64
	compactionGen uint64
	keyDir        map[string]KeyDirEntry
	closed        bool
}

func Open(path string, opts Options) (*Engine, error) {
	// Skeleton stub for M1
	return &Engine{
		path:     path,
		maxBytes: opts.MaxBytes,
		keyDir:   make(map[string]KeyDirEntry),
	}, nil
}

func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
	return nil
}

func (e *Engine) Put(key, val []byte) error {
	return nil
}

func (e *Engine) Get(key []byte) ([]byte, bool, error) {
	return nil, false, nil
}

func (e *Engine) Delete(key []byte) error {
	return nil
}

func (e *Engine) Batch(ops []format.Operation) error {
	return nil
}

func (e *Engine) Dump() ([]Pair, error) {
	return nil, nil
}

func (e *Engine) Compact() error {
	return nil
}

func (e *Engine) Check() error {
	return nil
}
```

### 3.7 Package `internal/testutil` (Fixtures)
Create `/home/laik/workspace/srinathonn/internal/testutil/fixtures.go`:
```go
package testutil

import (
	"encoding/hex"
)

const (
	// Golden 32-byte Header (KVDB2026, version 1, compaction gen 0, reserved 0, CRC 0x6708c580)
	GoldenHeaderHex = "4b56444232303236010000000000000000000000000000000000000080c50867"

	// Golden 45-byte PUT Frame (Seq 1, OpCount 1, PUT "foo"="bar", HeaderCRC 0x054ecae6, TrailerCRC 0xdd783b3a)
	GoldenPutFrameHex = "45424155e6ca4e050d0000000000000001000000000000000100000001030003000000666f6f6261723a3b78dd"

	// Golden 42-byte DEL Frame (Seq 2, OpCount 1, DEL "foo", HeaderCRC 0xd2177085, TrailerCRC 0xc692709e)
	GoldenDelFrameHex = "45424155857017d20a0000000000000002000000000000000100000002030000000000666f6f9e7092c6"
)

func MustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}
```

### 3.8 Command Entrypoint `cmd/kvdb/main.go`
Create `/home/laik/workspace/srinathonn/cmd/kvdb/main.go`:
```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: kvdb <command> [args...]\n")
		os.Exit(1)
	}
	cmd := os.Args[1]
	switch cmd {
	case "open", "put", "get", "del", "batch", "dump", "check", "serve":
		fmt.Fprintf(os.Stderr, "Command '%s' stub initialized (M1 build target)\n", cmd)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}
```

### 3.9 Comprehensive Unit & Golden Tests
Implement:
- `internal/format/header_test.go`:
  - `TestEncodeDecodeHeader`: round-trip validation.
  - `TestHeaderGoldenHex`: byte-for-byte hex comparison against `GoldenHeaderHex`.
  - `TestCorruptHeader`: truncated slice, invalid magic, invalid version, non-zero reserved field, single bit flip in CRC.
- `internal/format/uabe_test.go`:
  - `TestEncodeDecodeFrame`: round-trip single PUT and DEL frames.
  - `TestPutFrameGoldenHex`: byte-for-byte hex comparison against `GoldenPutFrameHex`.
  - `TestDelFrameGoldenHex`: byte-for-byte hex comparison against `GoldenDelFrameHex`.
  - `TestBatchFrame`: multi-operation batch (PUT + DEL) encoding and decoding.
  - `TestTornHeaderAndTrailer`: truncated frame header, truncated payload, truncated trailer.
- `internal/format/record_test.go`:
  - Boundary cases: key length 0 (rejected), key length 1024 (accepted), key length 1025 (rejected).
  - Value length 0 (accepted for PUT), value length 1MB (accepted), value length 1MB + 1 (rejected).
  - DEL with non-zero value length (rejected).
  - Trailing payload garbage bytes (rejected).
- `internal/format/fuzz_test.go`:
  - Fuzzing parser inputs to guarantee zero panics and memory bounds on malformed bytes.

---

## 4. Caveats

1. **Git State Synchronization:**
   - The workspace has local uncommitted modifications in `MEMORY.md` and `PROMPTS.md`, and is behind `origin/main` by 1 commit (`2a229d0`).
   - The Worker must reconcile local memory updates before executing `git pull` to avoid merge conflicts.
2. **Standard Library Constraints:**
   - Python's standard `zlib.crc32` uses IEEE 802.3 polynomial, whereas the database specification strictly requires Castagnoli CRC32C. Python scripts in `torture/` or verification tests must use an explicit Castagnoli polynomial table (`0x82F63B78`), whereas Go natively provides `hash/crc32` with `crc32.Castagnoli`.
3. **M1 Scope Isolation:**
   - Milestone M1 must strictly avoid implementing active file operations (`os.OpenFile`, `flock`, `file.Sync`) or KeyDir index updates, which belong to Milestone M2.

---

## 5. Conclusion

1. **Architecture & Deliverables Readiness:**
   - The requirements for Milestone M1 (Phases E0 and E1) are unambiguous and mathematically verified.
   - The module `kvdb` with Go 1.27 and zero external dependencies satisfies 100% of the Rule 02 offline compilation rules.
   - The root `Makefile` provides all required phony targets (`all`, `build`, `test`, `clean`, `demo-ready`) using strict flags (`CGO_ENABLED=0`, `GOPROXY=off`, `-trimpath`, `-ldflags="-s -w"`).
   - Package boundaries (`internal/format`, `internal/engine`, `internal/testutil`, `cmd/kvdb`) strictly align with `PROJECT.md § Code Layout`.
2. **Golden Hex Parity:**
   - All golden binary fixtures (32-byte header `4b56...`, PUT frame `4542...`, DEL frame `4542...`) have been calculated and verified with dual Castagnoli CRC32C checksums in both Go and Python.
3. **Actionable Worker Plan:**
   - The Worker has complete, concrete implementation blueprints, struct designs, error definitions, and test suites ready for immediate execution.

---

## 6. Verification Method

To independently verify this investigation report and validate the Worker's implementation:

1. **Module & Offline Build Verification:**
   ```bash
   # Confirm module and offline compilation
   go env GOPROXY=off
   make build
   ./kvdb
   ```
2. **Unit Test & Fixture Verification:**
   ```bash
   # Run all package unit tests offline
   make test
   ```
3. **Golden Hex Vectors Verification:**
   ```bash
   # Test golden hex fixtures against Go stdlib Castagnoli CRC32C
   go test -v -run TestHeaderGoldenHex ./internal/format
   go test -v -run TestPutFrameGoldenHex ./internal/format
   go test -v -run TestDelFrameGoldenHex ./internal/format
   ```
4. **Fuzz Test & Memory Bounds Verification:**
   ```bash
   go test -v -run TestFuzz ./internal/format
   ```
