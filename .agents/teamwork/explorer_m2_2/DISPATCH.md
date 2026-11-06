## 2026-11-06T12:47:13Z
MANDATORY FIRST STEP: Read `/home/laik/workspace/srinathonn/.agents/teamwork/ORIGINAL_REQUEST.md` before starting work.

Context: Milestone M2 (Lifecycle, Locking & Single Operations: Phases E2 and E3).
Scope Document: `/home/laik/workspace/srinathonn/.agents/teamwork/PROJECT.md`
Prior Specifications:
- `docs/engine/PHASES.md § E3`
- `docs/engine/05_WRITE_AND_COMMIT_PROTOCOL.md`
- `docs/engine/06_PUT_GET_DELETE.md`
- `docs/engine/13_IN_MEMORY_INDEX.md`
- `internal/format/` (codec files created in M1)

Working directory: `/home/laik/workspace/srinathonn/.agents/teamwork/explorer_m2_2/`
Identity: `explorer_m2_2`

Task:
Analyze and formulate the concrete implementation strategy for Phase E3 (Append & Single Operations):
1. Strict 10-step write protocol:
   - Input validation (key 1..1024B, value 0..1MB).
   - Write lock acquisition (`e.mu.Lock()`).
   - Frame serialization using `format.EncodeFrame` (28B UABE header + payload + 4B Castagnoli trailer).
   - Quota check (`validEOF + frameSize <= maxBytes`).
   - Position-independent write loop (`WriteAt` at `validEOF` until full slice written).
   - Physical durability barrier: `e.dbFile.Sync()` (`fsync`).
   - Index publication: update `e.keyDir[string(key)] = KeyDirEntry{offset, valLen, seqNo}`.
   - Metadata update: `e.validEOF += frameSize`, `e.nextSeqNo++`.
   - Write lock release (`e.mu.Unlock()`).
   - Return nil success (caller emits ACK).
2. KeyDir index architecture:
   - `KeyDirEntry`: `ValueOffset uint64`, `ValueLen uint32`, `SeqNo uint64`.
   - Value offset arithmetic: `frameStart + 28 + 7 + keyLen`.
3. PUT operation: Overwrites previous entry; supports 0-byte and 1MB values.
4. GET operation: `e.mu.RLock()`; lookup in KeyDir; read value bytes via `ReadAt(buf, int64(entry.ValueOffset))`; return value copy + `found=true`. Key missing returns `nil, false, nil` (or `ErrNotFound`).
5. DEL operation: Serializes tombstone frame (`OpTypeDel`, `ValueLen=0`), writes and syncs to disk, removes key from KeyDir (`delete(e.keyDir, string(key))`). Deleting missing key is valid and durable.
6. Recommend clean method signatures and file decomposition (`internal/engine/write.go`, `internal/engine/read.go`, `internal/engine/keydir.go`).

Deliverable:
Write report to `/home/laik/workspace/srinathonn/.agents/teamwork/explorer_m2_2/handoff.md`.
Do NOT implement code directly. Send a message to your caller upon completion with your report path.
