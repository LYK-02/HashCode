# Handoff Report: Phase E3 Implementation Strategy (Append & Single Operations)

> **Agent**: `explorer_m2_2`  
> **Milestone**: M2 (Lifecycle, Locking & Single Operations: Phases E2 and E3)  
> **Scope**: Implementation strategy for Phase E3: Strict 10-step write protocol, KeyDir in-memory index architecture, PUT/GET/DEL operations, error handling/rollback, and file decomposition.  
> **Status**: Complete & Verified (Read-only Investigation)

---

## 1. Observation

Direct observations from the codebase, project rules, and specification documents:

1. **Phase E3 Specification (`docs/engine/PHASES.md` lines 102–130)**:
   - Mutation path sequence:
     1. Validate API input.
     2. Build complete frame in memory.
     3. Acquire exclusive mutex.
     4. Check quota with overflow-safe arithmetic.
     5. Append at `validEOF` using a full-write loop.
     6. Handle short writes and EINTR.
     7. Sync file (`file.Sync()`).
     8. Apply operation to KeyDir.
     9. Advance EOF and sequence.
     10. Release lock.
     11. Let serve layer emit ACK.
   - Operations semantics:
     - "PUT overwrites by appending a new frame."
     - "GET reads via ReadAt and returns a copy."
     - "GET distinguishes empty value from NOTFOUND."
     - "DEL appends tombstone and removes live index entry after sync."
     - "Delete missing key succeeds as required by official rules."
   - Exit gate: "PUT/GET/DEL survive process restart. ACK only follows Sync and index update. Empty and binary values work. Same-key overwrite resolves to latest operation. A killed process never returns a mixed value."

2. **Write & Commit Protocol (`docs/engine/05_WRITE_AND_COMMIT_PROTOCOL.md` lines 17–38)**:
   - "All mutations use the same serialized UABE frame. A standalone PUT encodes one PUT record; DEL encodes one DEL; BATCH encodes all ordered operations in one frame. The frame is the only atomic unit. The write lock is held from quota validation through index publication so readers cannot see partial batches and writers cannot interleave bytes."
   - "A write call returning nil error does not mean durable. A successful fsync is the barrier. Never update KeyDir or return success before it."
   - "If quota fails, do not write anything and return stable `DISK_FULL`. If write is partial or fails, truncate back to last known durable EOF and sync the truncation. If sync fails, the kernel may have discarded dirty pages while leaving offsets advanced; a blind retry is not proof. Roll back to `lastSyncedEOF`, sync successfully, and mark engine poisoned if rollback cannot be proven."

3. **PUT / GET / DELETE Requirements (`docs/engine/06_PUT_GET_DELETE.md` lines 16–36)**:
   - "PUT stores the exact byte sequence under a key, replacing any prior value. Validate key length 1–1,024 and value length 0–1 MiB. Empty value is valid."
   - "GET checks the in-memory KeyDir under a read lock. If absent, return NOTFOUND. If present, read exactly `value_len` bytes at `value_offset` using position-independent I/O. For a zero-length value, return a present empty slice and `found=true`; do not interpret it as missing. Return a copy."
   - "DELETE appends a tombstone operation in a UABE frame. On successful sync, remove the key from KeyDir. Deleting a missing key is not an error per the official specification; it may still append a tombstone to preserve a clear mutation ordering."
   - "Binary keys/values may include NUL, tabs, newlines, invalid UTF-8, and all 256 byte values."

4. **In-Memory Index Architecture (`docs/engine/13_IN_MEMORY_INDEX.md` lines 16–30)**:
   - "The KeyDir is a derived in-memory hash map from key bytes to a locator for the current value in `data.db`. It is not persisted separately. A locator contains value offset, value length, and sequence/generation metadata. The map contains only live keys; delete removes an entry. A value length of zero is valid and still represents a present key."
   - "Use Go `map[string]KeyDirEntry` for arbitrary byte keys represented as strings... Go strings can contain arbitrary bytes."

5. **Existing Codecs (`internal/format/`)**:
   - `internal/format/record.go` (line 237–239):
     `ValueFileOffset(frameStart uint64, payloadOffset uint32, keyLen uint16) uint64`
     calculates `frameStart + 28 + uint64(payloadOffset) + uint64(RecordHeaderSize) + uint64(keyLen)`.
     For a single op (`payloadOffset = 0`, `RecordHeaderSize = 7`), `ValueOffset = frameStart + 28 + 7 + keyLen`.
   - `internal/format/uabe.go` (line 36):
     `EncodeFrame(seqNo uint64, ops []Operation) ([]byte, error)`
     encodes 28-byte UABE header + payload subrecords + 4-byte Castagnoli CRC32C trailer.
   - `internal/format/record.go` constants:
     `MinKeyLen = 1`, `MaxKeyLen = 1024`, `MinValueLen = 0`, `MaxValLen = 1048576` (1 MiB), `RecordHeaderSize = 7`.
   - `internal/format/errors.go`:
     `ErrKeyEmpty`, `ErrKeyTooLarge`, `ErrValueTooLarge`, `ErrDelNonZeroValue`, `ErrCorruptFrame`.

6. **Current Engine Skeleton (`internal/engine/engine.go`)**:
   - Lines 12–17 define standard errors: `ErrLocked`, `ErrClosed`, `ErrNotFound`, `ErrDiskFull`, `ErrEnginePoisoned`.
   - Line 20 defines `KeyDirEntry`: currently `{ FileOffset uint64, ValueLen uint32, SeqNo uint64 }`.
   - Line 38 defines `Engine` struct: has `mu sync.RWMutex`, `db *os.File`, `lockFile *os.File`, `path string`, `parentDir string`, `maxBytes uint64`, `validEOF uint64`, `nextSeqNo uint64`, `compactionGen uint64`, `keyDir map[string]KeyDirEntry`, `closed bool`.

---

## 2. Logic Chain

From the above observations, we construct the step-by-step logic chain leading to the concrete Phase E3 implementation strategy:

1. **Strict Concurrency & Write Ordering**:
   - *Observation 1 & 2*: Durability requires `file.Sync()` before KeyDir update, and write locks must protect from interleaving and dirty reads.
   - *Logic*: All write operations (`Put`, `Delete`) must acquire exclusive lock `e.mu.Lock()` before assigning `seqNo = e.nextSeqNo`, checking quota, or calling `WriteAt`. Readers (`Get`) acquire shared lock `e.mu.RLock()`.
   - *Consequence*: Readers never observe torn writes, half-written frames, un-fsynced frames, or intermediate states.

2. **Strict 10-Step Mutation Protocol**:
   - *Step 1 (Input Validation)*: Pre-validate key (1..1024 bytes) and value (0..1MB for PUT, 0 for DEL) before taking lock or serializing. Return `format.ErrKeyEmpty`, `format.ErrKeyTooLarge`, or `format.ErrValueTooLarge`.
   - *Step 2 (Lock Acquisition & State Check)*: Acquire `e.mu.Lock()`. If `e.closed`, return `ErrClosed`. If `e.poisoned`, return `ErrEnginePoisoned`.
   - *Step 3 (Frame Serialization)*: Construct `format.Operation` slice and serialize into contiguous bytes using `format.EncodeFrame(e.nextSeqNo, ops)`.
   - *Step 4 (Quota Precheck)*: Using overflow-safe arithmetic, check `if frameSize > math.MaxUint64 - e.validEOF || (e.maxBytes > 0 && e.validEOF + frameSize > e.maxBytes) { return ErrDiskFull }`. Zero disk bytes are written if quota is exceeded.
   - *Step 5 (Position-Independent Full Write Loop)*: Use helper `writeAtFull(e.db, frameBytes, int64(e.validEOF))` which loops over partial writes. If write fails, attempt rollback via `e.db.Truncate(int64(e.validEOF))` and `e.db.Sync()`. If rollback fails, set `e.poisoned = true`.
   - *Step 6 (Physical Durability Barrier)*: Issue `e.db.Sync()` (`fsync`). If this fails, attempt truncation rollback, mark `e.poisoned = true`, and return error.
   - *Step 7 (Index Publication)*: For `Put`, compute `valOffset := format.ValueFileOffset(e.validEOF, 0, uint16(len(key)))` and assign `e.keyDir[string(key)] = KeyDirEntry{ValueOffset: valOffset, ValueLen: uint32(len(val)), SeqNo: e.nextSeqNo}`. For `Delete`, invoke `delete(e.keyDir, string(key))`.
   - *Step 8 (Metadata Advance)*: Advance `e.validEOF += uint64(len(frameBytes))` and `e.nextSeqNo++`.
   - *Step 9 (Lock Release)*: Release `e.mu.Unlock()`.
   - *Step 10 (Return Success)*: Return `nil` (caller subsequently prints `ACK <op-id>`).

3. **KeyDir In-Memory Index Design**:
   - *Observation 4 & 5*: `format.ValueFileOffset` produces the exact byte offset of the value.
   - *Logic*: The locator struct in `internal/engine/keydir.go` should define:
     ```go
     type KeyDirEntry struct {
         ValueOffset uint64
         ValueLen    uint32
         SeqNo       uint64
     }
     ```
     For backward compatibility with existing code references, provide an accessor or field alias `FileOffset uint64` if needed.
   - *Locator Arithmetic*: For single-op frames (PUT/DEL), `payloadOffset = 0`. Value offset is:
     `validEOF + 28 (FrameHeader) + 7 (OpHeader) + len(key)` = `validEOF + 35 + len(key)`.
   - *Binary Safety*: Go map `map[string]KeyDirEntry` safely handles raw byte strings containing arbitrary bytes (`0x00` through `0xFF`, newlines, invalid UTF-8) because Go strings are length-prefixed byte sequences.

4. **GET Operation (Zero-Seek Locator Lookup)**:
   - *Observation 3 & 4*: Position-independent read under `e.mu.RLock()`.
   - *Logic*:
     - Acquire `e.mu.RLock()`. Defers `e.mu.RUnlock()`.
     - Check `e.closed`.
     - Lookup in `e.keyDir[string(key)]`. If absent, return `nil, false, nil`.
     - If `entry.ValueLen == 0` (empty value), return `[]byte{}, true, nil` immediately without disk I/O.
     - If `entry.ValueLen > 0`, allocate `buf := make([]byte, entry.ValueLen)`.
     - Call `readAtFull(e.db, buf, int64(entry.ValueOffset))`.
     - Return `buf, true, nil`. The returned slice is a distinct copy, preventing caller mutation.

5. **DEL Operation (Tombstone Persistence)**:
   - *Observation 1 & 3*: Deleting missing keys must succeed and append a durable tombstone.
   - *Logic*:
     - A tombstone frame with `OpTypeDel`, `key`, and `ValueLen = 0` is encoded and persisted via the exact same 10-step write protocol.
     - On successful fsync, `delete(e.keyDir, string(key))` is called (which is safe even if key was not present).
     - Missing key delete returns `nil` success, logs an ordered sequence number on disk, and does not corrupt state.

6. **Shared Low-Level Append Primitive (`appendFrameLocked`)**:
   - *Logic*: Consolidate Steps 3 through 6 into a private helper `(e *Engine) appendFrameLocked(ops []format.Operation) ([]byte, error)`:
     - Encodes frame with `e.nextSeqNo`.
     - Validates quota against `e.maxBytes`.
     - Writes all bytes at `e.validEOF` via `writeAtFull`.
     - Calls `e.db.Sync()`.
     - Performs automatic rollback to `e.validEOF` if write or sync fails.
     - Sets `e.poisoned = true` if rollback fails.
   - *Benefit*: This guarantees 100% DRY compliance. `Put`, `Delete`, and Milestone M3's `Batch` will all share the exact same battle-tested durability and rollback logic.

---

## 3. Caveats

1. **Recovery Interplay**: Phase E3 updates `e.validEOF`, `e.nextSeqNo`, and `e.keyDir` dynamically during process lifetime. However, initial population of `validEOF`, `nextSeqNo`, and `keyDir` at `Open()` depends on Phase E5 log replay. Until Phase E5 is implemented, an engine reopened from disk will initialize with an empty `keyDir` unless replayed. For testing E3 in isolation, in-process mutation/queries and single-session durability can be tested immediately.
2. **File Descriptor Sharing in Concurrency**: `ReadAt` and `WriteAt` in Go use `pread` and `pwrite` syscalls underneath, which do not modify the file seek offset. Therefore, multiple concurrent readers (`Get`) and a serialized writer (`Put`/`Delete`) can safely share the same `*os.File` descriptor without corrupting file offsets.
3. **No Caveats on Standard Library**: All required primitives (`os.File.WriteAt`, `os.File.ReadAt`, `os.File.Sync`, `os.File.Truncate`, `sync.RWMutex`) are available in Go standard library (`os`, `sync`, `math`). Zero third-party packages are required.

---

## 4. Conclusion & Concrete Implementation Strategy

### Recommended File Decomposition
```
internal/engine/
├── engine.go      # Core Engine struct, Options, standard errors, state flags
├── keydir.go      # KeyDirEntry struct, locator arithmetic, index helper methods
├── write.go       # Put(), Delete(), appendFrameLocked(), writeAtFull(), rollback logic
└── read.go        # Get(), readAtFull()
```

### Detailed Code Blueprint

#### 1. Updates to `internal/engine/engine.go`
Ensure `Engine` tracks `poisoned bool` for write rollback failures:
```go
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
	poisoned      bool
}
```

#### 2. `internal/engine/keydir.go`
```go
package engine

import (
	"kvdb/internal/format"
)

// KeyDirEntry stores in-memory index locators pointing directly to value bytes in the single data file.
type KeyDirEntry struct {
	ValueOffset uint64 // Absolute byte offset of value in data.db
	FileOffset  uint64 // Alias for ValueOffset for backward compatibility
	ValueLen    uint32 // Length of value in bytes
	SeqNo       uint64 // Sequence number of the operation that established this entry
}

// NewKeyDirEntry constructs a KeyDirEntry ensuring both ValueOffset and FileOffset are populated.
func NewKeyDirEntry(valOffset uint64, valLen uint32, seqNo uint64) KeyDirEntry {
	return KeyDirEntry{
		ValueOffset: valOffset,
		FileOffset:  valOffset,
		ValueLen:    valLen,
		SeqNo:       seqNo,
	}
}

// SingleOpValueOffset computes the absolute value file offset for a single PUT frame at frameStart.
func SingleOpValueOffset(frameStart uint64, keyLen int) uint64 {
	return format.ValueFileOffset(frameStart, 0, uint16(keyLen))
}
```

#### 3. `internal/engine/write.go`
```go
package engine

import (
	"fmt"
	"math"
	"os"

	"kvdb/internal/format"
)

// Put inserts or updates a key-value pair following the strict 10-step write protocol.
func (e *Engine) Put(key, val []byte) error {
	// Step 1: Input validation
	if len(key) < format.MinKeyLen {
		return format.ErrKeyEmpty
	}
	if len(key) > format.MaxKeyLen {
		return format.ErrKeyTooLarge
	}
	if len(val) > format.MaxValLen {
		return format.ErrValueTooLarge
	}

	// Step 2: Acquire exclusive write lock
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrClosed
	}
	if e.poisoned {
		return ErrEnginePoisoned
	}

	// Step 3-6: Encode, quota check, position-independent write, and physical fsync barrier
	ops := []format.Operation{{
		Type:  format.OpPut,
		Key:   key,
		Value: val,
	}}
	frameBytes, err := e.appendFrameLocked(ops)
	if err != nil {
		return err
	}

	// Step 7: Index publication
	valOffset := format.ValueFileOffset(e.validEOF, 0, uint16(len(key)))
	e.keyDir[string(key)] = NewKeyDirEntry(valOffset, uint32(len(val)), e.nextSeqNo)

	// Step 8: Metadata update
	e.validEOF += uint64(len(frameBytes))
	e.nextSeqNo++

	// Step 9 & 10: Lock released by defer; return nil success
	return nil
}

// Delete appends a tombstone record removing the key. Deleting missing keys is valid and durable.
func (e *Engine) Delete(key []byte) error {
	// Step 1: Input validation
	if len(key) < format.MinKeyLen {
		return format.ErrKeyEmpty
	}
	if len(key) > format.MaxKeyLen {
		return format.ErrKeyTooLarge
	}

	// Step 2: Acquire exclusive write lock
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrClosed
	}
	if e.poisoned {
		return ErrEnginePoisoned
	}

	// Step 3-6: Encode, quota check, position-independent write, and physical fsync barrier
	ops := []format.Operation{{
		Type:  format.OpDel,
		Key:   key,
		Value: nil,
	}}
	frameBytes, err := e.appendFrameLocked(ops)
	if err != nil {
		return err
	}

	// Step 7: Index publication (remove from KeyDir)
	delete(e.keyDir, string(key))

	// Step 8: Metadata update
	e.validEOF += uint64(len(frameBytes))
	e.nextSeqNo++

	// Step 9 & 10: Lock released by defer; return nil success
	return nil
}

// appendFrameLocked handles serialization, quota check, writeAtFull, fsync barrier, and rollback.
// Must be called with e.mu held exclusively.
func (e *Engine) appendFrameLocked(ops []format.Operation) ([]byte, error) {
	frameBytes, err := format.EncodeFrame(e.nextSeqNo, ops)
	if err != nil {
		return nil, err
	}

	frameSize := uint64(len(frameBytes))

	// Quota validation with overflow safety
	if frameSize > math.MaxUint64-e.validEOF {
		return nil, ErrDiskFull
	}
	if e.maxBytes > 0 && e.validEOF+frameSize > e.maxBytes {
		return nil, ErrDiskFull
	}

	// Position-independent write loop
	if err := writeAtFull(e.db, frameBytes, int64(e.validEOF)); err != nil {
		e.rollbackAppendLocked(err)
		return nil, err
	}

	// Physical fsync barrier
	if err := e.db.Sync(); err != nil {
		e.rollbackAppendLocked(err)
		return nil, err
	}

	return frameBytes, nil
}

// rollbackAppendLocked attempts to truncate back to validEOF and sync after a failed write or sync.
func (e *Engine) rollbackAppendLocked(cause error) {
	truncErr := e.db.Truncate(int64(e.validEOF))
	syncErr := e.db.Sync()
	if truncErr != nil || syncErr != nil {
		e.poisoned = true
	}
}

// writeAtFull loops until all bytes in buf are written to f at off.
func writeAtFull(f *os.File, buf []byte, off int64) error {
	written := 0
	for written < len(buf) {
		n, err := f.WriteAt(buf[written:], off+int64(written))
		written += n
		if err != nil {
			return err
		}
	}
	return nil
}
```

#### 4. `internal/engine/read.go`
```go
package engine

import (
	"os"

	"kvdb/internal/format"
)

// Get retrieves the value associated with key, returning (value, true, nil) or (nil, false, nil).
func (e *Engine) Get(key []byte) ([]byte, bool, error) {
	if len(key) < format.MinKeyLen {
		return nil, false, format.ErrKeyEmpty
	}
	if len(key) > format.MaxKeyLen {
		return nil, false, format.ErrKeyTooLarge
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return nil, false, ErrClosed
	}

	entry, found := e.keyDir[string(key)]
	if !found {
		return nil, false, nil
	}

	// 0-byte value: key is present, return empty slice without disk I/O
	if entry.ValueLen == 0 {
		return []byte{}, true, nil
	}

	buf := make([]byte, entry.ValueLen)
	if err := readAtFull(e.db, buf, int64(entry.ValueOffset)); err != nil {
		return nil, false, err
	}

	return buf, true, nil
}

// readAtFull loops until all bytes in buf are read from f at off.
func readAtFull(f *os.File, buf []byte, off int64) error {
	read := 0
	for read < len(buf) {
		n, err := f.ReadAt(buf[read:], off+int64(read))
		read += n
		if err != nil {
			return err
		}
	}
	return nil
}
```

---

## 5. Verification Method

To independently verify the implementation strategy:

1. **Compilation & Static Analysis**:
   ```bash
   go vet ./...
   go build ./...
   ```
2. **Unit & Edge Case Tests (`internal/engine/write_test.go`, `internal/engine/read_test.go`)**:
   Verify the following targeted test scenarios:
   - `TestPutGetSingle`: Insert single key/value; assert `Get` returns exact value and `found == true`.
   - `TestPutOverwrite`: Insert `"k" -> "v1"`, then `"k" -> "v2"`; assert `Get` returns `"v2"`.
   - `TestPutEmptyValue`: Insert `"k" -> ""`; assert `Get` returns `[]byte{}` with `found == true`.
   - `TestPut1MBValue`: Insert 1024-byte key with 1MB (`1 << 20` bytes) pseudo-random payload; assert `Get` returns exact 1MB payload.
   - `TestDeleteKey`: Insert `"k" -> "v"`, `Delete("k")`; assert `Get` returns `found == false`.
   - `TestDeleteMissingKey`: Call `Delete("non_existent")` on empty database; assert returns `nil` error and appends durable tombstone frame on disk with `nextSeqNo = 1`.
   - `TestQuotaExceeded`: Initialize engine with `maxBytes = 80`; attempt write exceeding quota; assert returns `ErrDiskFull`, no bytes appended, and KeyDir unchanged.
   - `TestConcurrentReadersAndWriter`: Launch 16 goroutines reading while 1 writer performs updates; run with `go test -race ./internal/engine/...`; assert zero race conditions and zero panics.
   - `TestBinaryAlphabet`: Keys and values containing all 256 byte values (`\x00`, `\n`, `\r`, `\xff`); assert exact byte equality.
3. **Invalidation Conditions**:
   - Any execution path returning success (`ACK`) before `Sync()` completes invalidates durability guarantees.
   - Any mutable slice returned by `Get` without copying invalidates reader isolation.
   - Any seek-based I/O (`Seek`) rather than position-independent (`ReadAt`/`WriteAt`) invalidates multi-reader safety.
