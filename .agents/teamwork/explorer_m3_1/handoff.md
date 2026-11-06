# Architectural Handoff Report: Milestone M3 Phase E4 (Atomic Batch Mutation)

> **Author**: `explorer_m3_1` (Batch Atomicity & Frame Construction Explorer)  
> **Target Milestone**: M3 Phase E4  
> **Working Directory**: `/home/laik/workspace/srinathonn/.agents/teamwork/explorer_m3_1/`  
> **Date**: 2026-11-06  

---

## 1. Observation

### 1.1 Existing Codebase & Code Layout
1. **`PROJECT.md` Architecture & File Layout**:
   - `PROJECT.md` line 33 explicitly designates `internal/engine/batch.go` as the canonical file for `"Atomic batch execution (1..10,000 ops)"`.
   - `PROJECT.md` line 78 designates Milestone M3 outputs as `internal/engine/{batch,recovery}.go, recovery tests`.
   - Currently, `internal/engine/batch.go` does not exist. Instead, an initial `Batch` implementation was placed in `internal/engine/engine.go` (lines 292–343).

2. **Binary Codec Support in `internal/format/`**:
   - `internal/format/record.go` (lines 8–29):
     ```go
     RecordHeaderSize = 7 // 1 (OpType) + 2 (KeyLen) + 4 (ValueLen)
     MinKeyLen        = 1
     MaxKeyLen        = 1024
     MinValueLen      = 0
     MaxValLen        = 1048576 // 1 MiB (1 << 20)
     MaxBatchOps      = 10000
     MaxPayloadLen    = 64 * 1024 * 1024 // 64 MiB guard limit
     ```
   - `internal/format/record.go` (lines 77–99): `CalculatePayloadLength(ops []Operation) (uint64, error)` validates bounds ($1 \le \text{len} \le 10,000$), validates each operation via `ValidateOperation`, guards against integer overflow (`math.MaxUint64 - total < subLen`), and enforces `MaxPayloadLen`.
   - `internal/format/record.go` (lines 237–239):
     ```go
     func ValueFileOffset(frameStart uint64, payloadOffset uint32, keyLen uint16) uint64 {
         return frameStart + 28 + uint64(payloadOffset) + uint64(RecordHeaderSize) + uint64(keyLen)
     }
     ```
   - `internal/format/uabe.go` (lines 36–79): `EncodeFrame(seqNo uint64, ops []Operation) ([]byte, error)` serializes all operations into a single UABE frame:
     - 28-byte Header: Magic `0x55414245` (`UABE`), Header CRC32C over bytes 8..28, Payload Length (`uint64`), Sequence Number (`uint64`), Operation Count (`uint32`).
     - Variable Payload: Concatenation of subrecords (`OpType:1B` + `KeyLen:2B` + `ValLen:4B` + `Key` + `Value`).
     - 4-byte Trailer: Castagnoli CRC32C computed over the entire combined payload `buf[28 : 28+payloadLen]`.
   - `internal/format/uabe.go` (lines 132–143): `DecodePayloadRecords(payload []byte, trailer []byte, opCount uint32) ([]DecodedRecord, error)` verifies the 4-byte trailer CRC32C over the payload, then parses each subrecord.
   - `internal/format/errors.go` (lines 21–25, 42–50): Defines `ErrBatchEmpty`, `ErrBatchTooLarge`, `ErrPayloadTooLarge`, `ErrPayloadOverflow`, `ErrUnknownOpType`, `ErrKeyEmpty`, `ErrKeyTooLarge`, `ErrValueTooLarge`, `ErrDelNonZeroValue`.

3. **Current `Batch` Execution in `internal/engine/engine.go` (lines 292–343)**:
   ```go
   func (e *Engine) Batch(ops []format.Operation) error {
       if len(ops) < 1 { return format.ErrBatchEmpty }
       if len(ops) > format.MaxBatchOps { return format.ErrBatchTooLarge }
       for i := range ops {
           if err := format.ValidateOperation(ops[i]); err != nil { return err }
       }
       e.mu.Lock()
       defer e.mu.Unlock()
       if e.closed { return ErrClosed }
       if e.poisoned { return ErrEnginePoisoned }

       frameBytes, err := e.appendFrameLocked(ops)
       if err != nil { return err }

       fh, err := format.DecodeFrameHeader(frameBytes)
       if err != nil { return err }
       payload := frameBytes[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
       records, err := format.DecodePayloadRecordsOnly(payload, fh.OpCount)
       if err != nil { return err }

       for _, r := range records {
           if r.Type == format.OpPut {
               valOffset := format.ValueFileOffset(e.validEOF, r.PayloadOffset, r.KeyLen)
               e.keyDir[string(r.Key)] = NewKeyDirEntry(valOffset, r.ValueLen, e.nextSeqNo)
           } else if r.Type == format.OpDel {
               delete(e.keyDir, string(r.Key))
           }
       }
       e.validEOF += uint64(len(frameBytes))
       e.nextSeqNo++
       return nil
   }
   ```
   *Performance and Allocation Finding*: Notice that after `appendFrameLocked(ops)`, the current code re-decodes `frameBytes` by calling `DecodeFrameHeader` and `DecodePayloadRecordsOnly`. In `DecodePayloadRecordsOnly` (lines 189–199 of `record.go`), new byte slices are allocated for every value (`valCopy := make([]byte, valLen); copy(...)`), only for `r.Value` to be immediately ignored during the KeyDir loop. In a 10,000-op batch or with large values, this produces megabytes of unnecessary heap churn.

4. **Existing Test Coverage Gap in `internal/engine/`**:
   - `go test -v ./internal/...` passed 100% of format tests and engine single-operation tests.
   - However, `grep -n -C 5 "Batch" internal/engine/*test.go` revealed that `internal/engine/` contains **zero direct unit tests** for `Batch` or Group B crash scenarios (`B1`–`B6`).
   - All existing batch tests exist exclusively in the high-level opaque E2E suite (`tests/e2e/tier1_test.go`, `tier2_test.go`, `tier3_test.go`, `tier4_test.go`).

5. **CLI Batch Parser in `cmd/kvdb/main.go` (lines 158–193)**:
   ```go
   parts := strings.SplitN(line, " ", 3)
   switch parts[0] {
   case "put":
       if len(parts) < 2 { ... os.Exit(1) }
       val := ""
       if len(parts) == 3 { val = parts[2] }
       ops = append(ops, format.Operation{Type: format.OpPut, Key: []byte(parts[1]), Value: []byte(val)})
   case "del":
       if len(parts) < 2 { ... os.Exit(1) }
       ops = append(ops, format.Operation{Type: format.OpDel, Key: []byte(parts[1]), Value: nil})
   }
   ```
   *Limitations Observed*:
   - Keys containing spaces or quotes cannot be parsed (e.g., `put "first key" "value with spaces"` splits at the first space, breaking `parts[1]`).
   - Hex escape sequences (e.g. `\x00`, `\x01`) and standard escapes (`\"`, `\\`, `\n`, `\t`) are not unescaped.
   - `del` does not validate that only 1 argument was supplied (extra trailing garbage on a `del` line is silently ignored).

---

## 2. Logic Chain

```
[Observation 1.1: PROJECT.md Layout]
   │
   ├──> internal/engine/batch.go is required for Milestone M3 Phase E4
   │
[Observation 1.2: Format Package Multi-Record Support]
   │
   ├──> EncodeFrame already serializes 1..10,000 ops into a single UABE frame
   │    Envelope = 28B Header + Combined Payload + 4B Trailer CRC32C
   │
[Observation 1.3: Redundant Re-Decoding in Engine Batch]
   │
   ├──> In Batch(ops), we already hold the validated ops slice.
   │    Subrecord i payload offset is mathematically:
   │    Offset(0) = 0
   │    Offset(i) = Offset(i-1) + 7 + len(ops[i-1].Key) + len(ops[i-1].Value)
   │    Using direct running offset calculation eliminates:
   │      - Re-decoding 28B header
   │      - Re-parsing 1..10,000 subrecords
   │      - Allocating up to 10,000 defensive value byte slices
   │    This guarantees O(N) performance and zero wasted allocations!
   │
[Observation 1.4: Zero Unit Tests for Batch in internal/engine]
   │
   ├──> Need internal/engine/batch_test.go covering:
   │      - 1..10,000 operation boundaries
   │      - Pre-flight validation rejections (KeyEmpty, KeyTooLarge, ValTooLarge, DelNonZero)
   │      - Quota guard atomic rejection (ErrDiskFull, zero state change)
   │      - Intra-batch conflict handling (PUT overwrite, PUT-then-DEL, DEL-then-PUT, PUT-DEL-PUT)
   │      - Group B crash recovery scenarios (B1, B2, B3, B4, B5, B6)
   │      - Reader isolation during batch execution (Scenario D4)
   │
[Observation 1.5: CLI Batch File Parser Gaps]
   │
   └──> Implement robust tokenizer supporting:
          - Space-separated tokens and comment lines (#)
          - Quoted strings ("key with spaces", "value with spaces")
          - Escape sequences (\", \\, \n, \r, \t, \xHH)
          - Exact argument count enforcement (del rejects trailing tokens)
          - Non-zero exit on error with stderr diagnostics
```

---

## 3. Caveats

1. **Recovery Stream Scanner (Phase E5)**:
   While `engine.go`'s current recovery loop successfully parses batch frames, Milestone M3 Phase E5 will refactor recovery into `internal/engine/recovery.go`. In Phase E4, `Batch` must ensure the frames it emits on disk match the exact contract that `format.DecodePayloadRecords` and `recovery.go` expect.
2. **Streaming SERVE Mode (Phase E6)**:
   The `serve` daemon will invoke `(e *Engine) Batch(ops)` via the line-oriented socket command `BATCH <op-id> <count> <encoded-ops>`. Phase E4 must keep the `(e *Engine) Batch(ops []format.Operation) error` method signature strictly public and agnostic of CLI vs socket callers.
3. **Compaction Generation Preservation**:
   Compaction (Phase E8) rewrites live records. Each frame written by `Batch` carries `nextSeqNo`. KeyDir locators must strictly retain this sequence number so that future compaction or dump operations can trace generation ordering.

---

## 4. Conclusion & Technical Implementation Blueprint

Phase E4 must deliver:
1. `internal/engine/batch.go` containing `(e *Engine) Batch(ops []format.Operation) error` with direct payload offset computation.
2. Removal of `Batch` from `internal/engine/engine.go` to eliminate code duplication and maintain project layout.
3. Enhanced CLI batch file parser in `cmd/kvdb/main.go`.
4. Comprehensive unit test suite in `internal/engine/batch_test.go` covering contract boundaries, intra-batch conflicts, quota guards, and crash matrix Group B.

### 4.1 Detailed Contract Specifications

#### A. Bounds and Validation Contract
- **Count**: $1 \le \text{len}(ops) \le 10,000$.
  - If $\text{len}(ops) == 0$: return `format.ErrBatchEmpty`.
  - If $\text{len}(ops) > 10,000$: return `format.ErrBatchTooLarge`.
- **Pre-flight Validation**:
  - Validated **prior to** taking locks or initiating disk I/O.
  - Every operation must satisfy:
    * `op.Type == format.OpPut || op.Type == format.OpDel` (else `format.ErrUnknownOpType`)
    * $1 \le \text{len}(op.Key) \le 1,024$ (if 0: `format.ErrKeyEmpty`; if > 1024: `format.ErrKeyTooLarge`)
    * If `op.Type == format.OpDel`: $\text{len}(op.Value) == 0$ (else `format.ErrDelNonZeroValue`)
    * If `op.Type == format.OpPut`: $\text{len}(op.Value) \le 1,048,576$ (else `format.ErrValueTooLarge`)
  - Aggregate payload length calculated via `format.CalculatePayloadLength(ops)`:
    * Checks integer overflow (`math.MaxUint64 - total < subLen`)
    * Checks total payload size $\le 64 \text{ MiB}$ (`format.MaxPayloadLen`)

#### B. Single Frame Encoding Contract
- **Structure**:
  - `FrameHeader` (28 bytes):
    * `0..4`: `0x55414245` (UABE magic)
    * `4..8`: Header CRC32C over bytes 8..28 (Castagnoli CRC32C)
    * `8..16`: `PayloadLen` (`uint64`)
    * `16..24`: `SeqNo` (`uint64`) = `e.nextSeqNo`
    * `24..28`: `OpCount` (`uint32`) = `uint32(len(ops))`
  - `Payload` (variable bytes):
    * Sequentially ordered subrecords. Each subrecord: `Type (1B) + KeyLen (2B, LE) + ValLen (4B, LE) + Key + Value`.
  - `Trailer` (4 bytes):
    * Castagnoli CRC32C calculated over the entire combined payload `buf[28 : 28+payloadLen]`.
  - Total on-disk frame size = $32 + \text{PayloadLen}$.

#### C. Durability, Quota & Atomicity Protocol (10-Step Sequence)
1. **Pre-flight Validation**: Bounds and operation invariants verified.
2. **Lock Acquisition**: Acquire `e.mu.Lock()`. Verify engine is open and unpoisoned.
3. **Quota Pre-check**: Verify `validEOF + totalFrameSize <= maxBytes` (with overflow safety) before serialization. If exceeded, return `ErrDiskFull`.
4. **Serialization**: `format.EncodeFrame(e.nextSeqNo, ops)` serializes the frame into memory.
5. **Physical Disk Write**: `writeAtFull(e.db, frameBytes, int64(e.validEOF))`.
   - On write error: invoke `e.rollbackAppendLocked(err)`, do NOT touch KeyDir, fail closed.
6. **Physical Sync Barrier (`file.Sync()`)**:
   - `e.db.Sync()` executed **BEFORE** any in-memory index updates.
   - On sync error: invoke `e.rollbackAppendLocked(err)`, do NOT touch KeyDir, fail closed.
7. **KeyDir Publication**:
   - Iterates through `ops` in sequential order:
     * Maintains running `currPayloadOffset uint32 = 0`.
     * For `OpPut`:
       ```go
       valOffset := format.ValueFileOffset(e.validEOF, currPayloadOffset, uint16(len(op.Key)))
       e.keyDir[string(op.Key)] = NewKeyDirEntry(valOffset, uint32(len(op.Value)), e.nextSeqNo)
       ```
     * For `OpDel`:
       ```go
       delete(e.keyDir, string(op.Key))
       ```
     * Advances `currPayloadOffset += uint32(format.RecordHeaderSize + len(op.Key) + len(op.Value))`.
   - Readers holding `e.mu.RLock()` are completely blocked; reader visibility is atomic (Scenario D4).
8. **Metadata Update**:
   - Advance `e.validEOF += uint64(len(frameBytes))`.
   - Advance `e.nextSeqNo++`.
9. **Lock Release**: Release `e.mu.Unlock()` (via defer).
10. **Acknowledge**: Return nil (or emit `ACK <op-id>`).

#### D. Intra-Batch Conflict Resolution
- **Duplicate PUTs**: All operations are written to disk in order within the frame. In KeyDir, later PUTs overwrite earlier PUTs. On recovery, the same payload replay ordering guarantees the last PUT wins.
- **PUT followed by DEL on same key**: Both written to disk. Key is evicted from KeyDir. On recovery, replay deletes the key. Key is absent.
- **DEL followed by PUT on same key**: Both written to disk. KeyDir contains the final PUT. On recovery, key is restored with the final PUT value.
- **PUT - DEL - PUT on same key**: All three written to disk in order. Final PUT is published in KeyDir.

#### E. Crash Matrix Alignment (Group B & D)
| Scenario | Condition | Expected Behavior | Mechanism |
|---|---|---|---|
| **B1** | Crash after partial subrecords written | All 10 operations absent | Frame extends past EOF or trailer CRC mismatch at EOF $\to$ recovery truncates to `validEOF` |
| **B2** | Crash after subrecords written, trailer missing | All 10 operations absent | Missing 4-byte trailer detected at EOF $\to$ tail truncated to `validEOF` |
| **B3** | Crash after trailer written, before fsync | Either all 10 present or all 10 absent | All-or-nothing: if physical media synced, entire frame is valid; if discarded by OS, tail truncated |
| **B4** | Crash after batch returns success | All 10 operations present | Success is returned strictly after `fsync()` and KeyDir commit |
| **B5** | Intra-batch conflicts (PUT + DEL on same key) | Strict ordering honored | Replay order matches input order; last operation wins |
| **B6** | 10,000-op batch killed halfway | Zero operations visible; recovery $\le 2.0$s | Trailer CRC fails $\to$ single truncation at `validEOF`; no partial operations applied |
| **D3** | Concurrent batch writer & single PUT writer | Strict serialization | `e.mu.Lock()` prevents interleaving on disk and in KeyDir |
| **D4** | Reader calling `Get` during batch | Zero dirty reads | Reader acquires `RLock()`, sees 100% pre-batch or 100% post-batch state |
| **E2** | Quota reached halfway through batch | Entire batch rejected; zero operations visible | Quota checked against full frame size before write; `ErrDiskFull` returned; KeyDir untouched |

---

### 4.2 Code Blueprints for Implementation

#### File 1: `internal/engine/batch.go`
```go
package engine

import (
	"math"

	"kvdb/internal/format"
)

// Batch applies 1..10,000 operations atomically within a single UABE frame (Phase E4).
// It strictly guarantees:
// 1. Pre-flight bounds and operation validation.
// 2. Pre-write quota validation against total frame size.
// 3. Serialization into a single contiguous UABE frame.
// 4. Physical writeAtFull followed by fsync barrier before any index publication.
// 5. Atomic KeyDir publication with exact physical locators and Last-Operation-Wins resolution.
// 6. Reader isolation: readers see either 100% pre-batch or 100% post-batch state.
func (e *Engine) Batch(ops []format.Operation) error {
	// Step 1: Pre-flight bounds and operation validation
	if len(ops) < 1 {
		return format.ErrBatchEmpty
	}
	if len(ops) > format.MaxBatchOps {
		return format.ErrBatchTooLarge
	}

	payloadLen, err := format.CalculatePayloadLength(ops)
	if err != nil {
		return err
	}

	totalFrameSize := format.TotalFrameSize(payloadLen)

	// Step 2: Acquire exclusive write lock
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrClosed
	}
	if e.poisoned {
		return ErrEnginePoisoned
	}

	// Step 3: Quota validation with overflow safety
	if totalFrameSize > math.MaxUint64-e.validEOF {
		return ErrDiskFull
	}
	if e.maxBytes > 0 && e.validEOF+totalFrameSize > e.maxBytes {
		return ErrDiskFull
	}

	// Step 4-6: Frame encoding, position-independent write, and physical fsync barrier
	frameBytes, err := e.appendFrameLocked(ops)
	if err != nil {
		return err
	}

	// Step 7: Atomic in-memory KeyDir publication
	// Directly compute payload offsets in O(N) without redundant re-decoding or heap churn
	currPayloadOffset := uint32(0)
	for i := range ops {
		op := ops[i]
		if op.Type == format.OpPut {
			valOffset := format.ValueFileOffset(e.validEOF, currPayloadOffset, uint16(len(op.Key)))
			e.keyDir[string(op.Key)] = NewKeyDirEntry(valOffset, uint32(len(op.Value)), e.nextSeqNo)
		} else if op.Type == format.OpDel {
			delete(e.keyDir, string(op.Key))
		}
		currPayloadOffset += uint32(format.RecordHeaderSize + len(op.Key) + len(op.Value))
	}

	// Step 8: Metadata advancement
	e.validEOF += uint64(len(frameBytes))
	e.nextSeqNo++

	// Step 9-10: Lock released by defer; return nil
	return nil
}
```

#### File 2: Removal from `internal/engine/engine.go`
Delete lines 291–343 of `internal/engine/engine.go` (the existing duplicate `Batch` definition), redirecting all batch mutations to `batch.go`.

#### File 3: CLI Parser Enhancements in `cmd/kvdb/main.go`
Replace lines 154–197 of `cmd/kvdb/main.go` with a dedicated tokenizer:
```go
// parseBatchFile reads and tokenizes line-oriented batch commands from file.
func parseBatchFile(file string) ([]format.Operation, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read batch file: %w", err)
	}

	var ops []format.Operation
	scanner := bufio.NewScanner(bytes.NewReader(content))
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 64*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		op, err := parseBatchLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		ops = append(ops, *op)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	return ops, nil
}

// parseBatchLine parses a single line into a format.Operation.
func parseBatchLine(line string) (*format.Operation, error) {
	line = strings.TrimSpace(line)
	idx := strings.IndexAny(line, " \t")
	var cmd, rest string
	if idx == -1 {
		cmd = line
		rest = ""
	} else {
		cmd = line[:idx]
		rest = strings.TrimSpace(line[idx+1:])
	}

	switch cmd {
	case "put":
		if rest == "" {
			return nil, fmt.Errorf("truncated line: put requires key")
		}
		key, rem, err := parseToken(rest)
		if err != nil {
			return nil, fmt.Errorf("invalid put key: %w", err)
		}
		val, err := parseValueToken(rem)
		if err != nil {
			return nil, fmt.Errorf("invalid put value: %w", err)
		}
		return &format.Operation{
			Type:  format.OpPut,
			Key:   key,
			Value: val,
		}, nil

	case "del":
		if rest == "" {
			return nil, fmt.Errorf("truncated line: del requires key")
		}
		key, rem, err := parseToken(rest)
		if err != nil {
			return nil, fmt.Errorf("invalid del key: %w", err)
		}
		if strings.TrimSpace(rem) != "" {
			return nil, fmt.Errorf("invalid del syntax: excess arguments")
		}
		return &format.Operation{
			Type:  format.OpDel,
			Key:   key,
			Value: nil,
		}, nil

	default:
		return nil, fmt.Errorf("invalid operation %q (expected 'put' or 'del')", cmd)
	}
}

// parseToken extracts the first token (quoted or unquoted) and returns the rest of the string.
func parseToken(s string) ([]byte, string, error) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return nil, "", fmt.Errorf("empty token")
	}
	if s[0] == '"' {
		// Quoted token with escape sequences
		return unescapeQuoted(s)
	}
	// Space-delimited token
	idx := strings.IndexAny(s, " \t")
	if idx == -1 {
		return []byte(s), "", nil
	}
	return []byte(s[:idx]), s[idx+1:], nil
}

// parseValueToken extracts the value token, supporting empty values, quotes, or unquoted rest of line.
func parseValueToken(rem string) ([]byte, error) {
	rem = strings.TrimLeft(rem, " \t")
	if rem == "" {
		return []byte{}, nil
	}
	if rem[0] == '"' {
		val, remaining, err := unescapeQuoted(rem)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(remaining) != "" {
			return nil, fmt.Errorf("trailing characters after quoted value")
		}
		return val, nil
	}
	return []byte(rem), nil
}

// unescapeQuoted parses a leading quoted string handling \", \\, \n, \r, \t, and \xHH escapes.
func unescapeQuoted(s string) ([]byte, string, error) {
	if len(s) < 2 || s[0] != '"' {
		return nil, "", fmt.Errorf("expected opening quote")
	}
	var res bytes.Buffer
	i := 1
	for i < len(s) {
		c := s[i]
		if c == '"' {
			return res.Bytes(), s[i+1:], nil
		}
		if c == '\\' {
			if i+1 >= len(s) {
				return nil, "", fmt.Errorf("unfinished escape sequence")
			}
			nxt := s[i+1]
			switch nxt {
			case '"', '\\':
				res.WriteByte(nxt)
				i += 2
			case 'n':
				res.WriteByte('\n')
				i += 2
			case 'r':
				res.WriteByte('\r')
				i += 2
			case 't':
				res.WriteByte('\t')
				i += 2
			case 'x':
				if i+3 >= len(s) {
					return nil, "", fmt.Errorf("incomplete hex escape")
				}
				hexStr := s[i+2 : i+4]
				b, err := strconv.ParseUint(hexStr, 16, 8)
				if err != nil {
					return nil, "", fmt.Errorf("invalid hex escape: \\x%s", hexStr)
				}
				res.WriteByte(byte(b))
				i += 4
			default:
				res.WriteByte(c)
				res.WriteByte(nxt)
				i += 2
			}
		} else {
			res.WriteByte(c)
			i++
		}
	}
	return nil, "", fmt.Errorf("unclosed quote")
}
```

#### File 4: Comprehensive Test Suite in `internal/engine/batch_test.go`
A full unit test suite must test:
1. `TestBatch_EmptyBatch_Rejected`: `ops = []` returns `format.ErrBatchEmpty`.
2. `TestBatch_TooLargeBatch_Rejected`: 10,001 ops returns `format.ErrBatchTooLarge`.
3. `TestBatch_SingleOp_PutAndDel`: 1-op batch works cleanly for PUT and DEL.
4. `TestBatch_Mixed10Ops_CommittedDurable`: 10 mixed operations commit atomically and persist across engine re-open.
5. `TestBatch_10000Ops_Accepted`: Exactly 10,000 ops commit and spot checks succeed.
6. `TestBatch_IntraBatch_DuplicatePuts_LastWins`: `[PUT k v1, PUT k v2, PUT k v3]` $\to$ `Get(k)` returns `v3`.
7. `TestBatch_IntraBatch_PutThenDel_KeyAbsent`: `[PUT k v1, DEL k]` $\to$ `Get(k)` returns `NOTFOUND`.
8. `TestBatch_IntraBatch_DelThenPut_KeyPresent`: `[DEL k, PUT k v2]` $\to$ `Get(k)` returns `v2`.
9. `TestBatch_IntraBatch_PutDelPut_FinalWins`: `[PUT k v1, DEL k, PUT k v3]` $\to$ `Get(k)` returns `v3`.
10. `TestBatch_Validation_InvalidOpType`: Batch containing unknown op type fails pre-flight.
11. `TestBatch_Validation_KeyTooLarge`: Batch containing 1025-byte key fails pre-flight.
12. `TestBatch_Validation_ValueTooLarge`: Batch containing 1MB+1 byte value fails pre-flight.
13. `TestBatch_Validation_DelNonZeroValue`: Batch containing DEL with non-empty value fails pre-flight.
14. `TestBatch_Quota_AtomicRejection`: Quota exceeded by 1 byte rejects batch without partial disk writes or KeyDir changes (Scenario E2).
15. `TestBatch_GroupB_CrashScenarios`:
    - Simulated torn write at EOF during batch append:
      * Truncate batch frame trailer bit flip / short payload.
      * Re-open engine.
      * Verify recovery truncates torn tail back to `validEOF` and **zero** operations from the batch are present.
16. `TestBatch_Concurrency_ReaderIsolation`: Goroutine performing readers during batch execution never observes an intermediate subset of keys (Scenario D4).

---

## 5. Verification Method

### 5.1 Unit Test Verification
Once implemented by the implementer agent:
```bash
# 1. Run all internal package tests including batch_test.go
go test -v -count=1 ./internal/engine/...
go test -v -count=1 ./internal/format/...

# 2. Run specific batch unit tests
go test -v -count=1 -run ^TestBatch_ ./internal/engine/...
```

### 5.2 E2E Opaque-Box Suite Verification
Execute the complete multi-tier E2E opaque-box suite:
```bash
# 1. Run Tier 1 Feature 6 (Cases 26-30: Batch atomicity and intra-batch conflicts)
go test -v -count=1 -run ^TestTier1_2[6-9]|^TestTier1_30 ./tests/e2e/...

# 2. Run Tier 2 Boundary Tests (Cases 16-25: 0-op, 1-op, 10,000-op, 10,001-op, intra-batch conflicts)
go test -v -count=1 -run ^TestTier2_1[6-9]|^TestTier2_2[0-5] ./tests/e2e/...

# 3. Run Tier 2 Quota Batch Tests (Case 30) & CLI Tokenizer Tests (Cases 44-45)
go test -v -count=1 -run ^TestTier2_30|^TestTier2_4[4-5] ./tests/e2e/...

# 4. Run Tier 3 & Tier 4 Batch Scenarios (Ledger commit, Scale recovery)
go test -v -count=1 -run ^TestTier3_0[4-8]|^TestTier4_01 ./tests/e2e/...

# 5. Full E2E suite
bash tests/e2e/run_all.sh
```

### 5.3 Invalidation Conditions
The design is invalidated if:
1. Any operation in a batch becomes readable before `file.Sync()` successfully completes.
2. A crash during a batch leaves a subset of the batch operations visible in the database.
3. Recovery fails to truncate a torn batch tail at EOF or incorrectly rejects healthy preceding frames.
4. Duplicate keys in a batch do not resolve according to strict input sequence (last write wins).
5. Empty batches or batches $> 10,000$ operations are accepted.
6. A batch exceeding `--max-bytes` quota writes partial records to disk.
