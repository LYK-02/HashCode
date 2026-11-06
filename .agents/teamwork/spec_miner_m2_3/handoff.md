# Handoff Report: Milestone M2 Specification Mining & Test Architecture Plan

**Author**: `spec_miner_m2_3`  
**Date**: 2026-11-06  
**Scope**: Milestone M2 (Engine Lifecycle, Locking & Single Operations: Phases E2 & E3)  
**Target Test Suite**: `internal/engine/engine_test.go`  
**Referenced Specs**:
- `docs/engine/PHASES.md § E2 & E3`
- `docs/engine/11_CONCURRENCY_AND_LOCKING.md`
- `docs/engine/05_WRITE_AND_COMMIT_PROTOCOL.md`
- `docs/engine/06_PUT_GET_DELETE.md`
- `docs/engine/13_IN_MEMORY_INDEX.md`
- `docs/engine/16_ENGINE_TESTING_AND_BENCHMARKS.md`
- `.agents/rules/01_invariants.md`
- `.agents/rules/03_scenarios_matrix.md` (Scenarios A1–A7, D1–D4)

---

## 1. Observation

Authoritative source observations gathered from the project specifications:

1. **Locking & Lifecycle (`docs/engine/11_CONCURRENCY_AND_LOCKING.md:23-35`, `docs/engine/PHASES.md:78-95`)**:
   - *"Cross-process lock: permanent `<db>.lock` sidecar, exclusive flock from open through close."*
   - *"Do not unlink or rename the lock file: flock attaches to inode, and replacing the path can split ownership across two inodes."*
   - *"A second process must block or fail cleanly, not open concurrently as an independent writer."*
   - *"On SIGKILL, OS closes file descriptors and releases advisory lock automatically. The sidecar file remains."*
   - *"Lock order is process flock, then engine mutex."*
   - *"No API call after Close."*

2. **Single Mutation & Durability (`docs/engine/05_WRITE_AND_COMMIT_PROTOCOL.md:17-29`, `docs/engine/06_PUT_GET_DELETE.md:16-26`)**:
   - *"All mutations use the same serialized UABE frame. A standalone PUT encodes one PUT record; DEL encodes one DEL."*
   - *"Required sequence: (1) validate request and build full frame; (2) acquire exclusive engine lock; (3) verify frame size and overflow-safe `validEOF + frameSize <= maxBytes`; (4) write all bytes at exactly `validEOF` using a loop that handles short writes; (5) call file sync and require success; (6) apply validated operations to KeyDir in order; (7) advance `validEOF` and `nextSeqNo`; (8) release lock; (9) emit ACK."*
   - *"PUT always appends a new frame; do not overwrite an earlier value in place. The index locator points to the value bytes in the new frame."*
   - *"For a zero-length value, return a present empty slice and `found=true`; do not interpret it as missing."*
   - *"DELETE appends a tombstone operation in a UABE frame. On successful sync, remove the key from KeyDir. Deleting a missing key is not an error per the official specification; it may still append a tombstone to preserve a clear mutation ordering."*
   - *"PUT stores the exact byte sequence... Validate key length 1–1,024 and value length 0–1 MiB."*

3. **Concurrency Scenarios D1–D4 (`.agents/rules/03_scenarios_matrix.md:31-35`)**:
   - **D1**: *"Two writers putting different keys, killed at random. $\to$ Every acknowledged put from either writer is present."*
   - **D2**: *"Two writers putting same key, killed at random. $\to$ Key holds value that one writer actually wrote; never a mix of two values."*
   - **D3**: *"Multiple goroutines executing concurrent GETs while writer performs PUTs."*
   - **D4**: *"Reader calling `get` during a batch on same keys (no crash). $\to$ Reader sees state before batch or after batch, never in between."*

4. **In-Memory Index (`docs/engine/13_IN_MEMORY_INDEX.md:16-30`)**:
   - *"The KeyDir is a derived in-memory hash map from key bytes to a locator for the current value in `data.db`. It is not persisted separately. A locator contains value offset, value length, and sequence/generation metadata."*
   - *"GET checks the in-memory KeyDir under a read lock... reads exact bytes with `ReadAt`, verifies read length, and returns a copy."*

5. **Existing Code Baseline (`internal/engine/engine.go:12-59`)**:
   - Defines typed errors: `ErrLocked`, `ErrClosed`, `ErrNotFound`, `ErrDiskFull`, `ErrEnginePoisoned`.
   - Defines struct `KeyDirEntry { FileOffset uint64; ValueLen uint32; SeqNo uint64 }`.
   - Defines `Engine` methods: `Open(path, opts)`, `Close()`, `Put(key, val)`, `Get(key)`, `Delete(key)`.

---

## 2. Logic Chain

1. **Multi-process `flock` and SIGKILL testing without external tooling**:
   - Go's `testing` framework runs within a single process by default. However, cross-process mutual exclusion (`flock`) and `SIGKILL` cleanup are operating system kernel-level behaviors tied to process boundaries and file descriptor tables.
   - Therefore, integration tests must execute actual subprocesses using `os/exec.Command(os.Args[0], ...)` with a dedicated environment variable (`KVDB_TEST_SUBPROCESS=1`).
   - In `TestMain(m *testing.M)`, if `KVDB_TEST_SUBPROCESS=1` is detected, the process runs a minimal helper routine that opens the database, acquires `flock`, signals readiness via stdout, and waits.
   - This enables deterministic testing of:
     - `ErrLocked` when a second process calls `Open()`.
     - Clean handover when the primary process exits normally via `Close()`.
     - Kernel lock release when the primary process is killed via `cmd.Process.Kill()` (`SIGKILL`), proving the core hackathon requirement that no stale lock file remains wedged.

2. **Durable mutation and recovery verification pattern**:
   - Every single mutation test must follow a three-phase assertion:
     - Phase 1: In-memory verification immediately after the call returns (verifying `KeyDir` state, returned values, and file size).
     - Phase 2: Engine closure via `engine.Close()` to flush/release descriptors.
     - Phase 3: Engine reopening via `Open(path, ...)` to prove cold recovery from disk frames alone, asserting that the reconstructed `KeyDir` returns the identical value without data corruption or memory leakage.

3. **Concurrency and un-torn reads (D1–D4)**:
   - In Go, goroutines running within the same engine instance share the engine's `sync.RWMutex`.
   - Writer holds `mu.Lock()` across frame construction, disk write, `Sync()`, and `KeyDir` publication.
   - Reader holds `mu.RLock()` across `KeyDir` lookup and `ReadAt(buf, int64(locator.FileOffset))` copy.
   - To verify D4 (no torn reads), writers must alternate writing large, uniform byte patterns (e.g. 1KB of `'A'` vs 1KB of `'B'`), while readers continuously sample the key and assert that every read value consists solely of all `'A'`s or all `'B'`s, never a byte-interleaved slice.
   - Running with `go test -race` verifies zero data races exist across all concurrent operations.

---

## 3. Features Discovered

| # | Category | Feature | Description | Inputs | Outputs | Error Behavior | Discovered Via |
|---|----------|---------|-------------|--------|---------|----------------|----------------|
| 1 | Lifecycle | Database Initialization | Creates `<path>` and writes 32B DB Header on first open | `path string, opts Options` | `*Engine, error` | Fails if dir unwritable or disk full | `docs/engine/PHASES.md:82-85` |
| 2 | Lifecycle | Lock Sidecar Creation | Creates `<path>.lock` sidecar file permanently | `path string` | File descriptor | Fails if permission denied | `docs/engine/11_CONCURRENCY_AND_LOCKING.md:25` |
| 3 | Lifecycle | Non-blocking Exclusive `flock` | Acquires `LOCK_EX \| LOCK_NB` on `<path>.lock` | File descriptor | Lock acquired or errno | Returns `ErrLocked` if held | `docs/engine/11_CONCURRENCY_AND_LOCKING.md:25` |
| 4 | Lifecycle | SIGKILL Lock Release | OS automatically drops flock on process death | OS `kill -9` | Kernel closes FDs | Lock freely acquired by next opener | `docs/engine/11_CONCURRENCY_AND_LOCKING.md:33` |
| 5 | Lifecycle | Clean Engine Close | Syncs DB, closes DB file, closes lock file, sets closed flag | `(e *Engine) Close()` | `error` | Returns `ErrClosed` if called again | `docs/engine/PHASES.md:86`, `engine.go:62` |
| 6 | Lifecycle | Path Canonicalization | Canonicalizes DB path to prevent duplicate lock inodes | `filepath.Abs` / `Clean` | Canonical path | Returns resolution error if invalid | `docs/engine/PHASES.md:79` |
| 7 | Mutation | PUT Operation | Encodes UABE frame, appends, syncs, updates KeyDir | `key []byte, val []byte` | `error` | `ErrKeyEmpty`, `ErrKeyTooLarge`, `ErrValueTooLarge`, `ErrDiskFull` | `docs/engine/06_PUT_GET_DELETE.md:17` |
| 8 | Read | GET Operation | Zero-seek `ReadAt` lookup using KeyDir locator | `key []byte` | `([]byte, bool, error)` | Returns `(nil, false, nil)` if missing; `ErrClosed` if closed | `docs/engine/06_PUT_GET_DELETE.md:20` |
| 9 | Mutation | DEL Operation | Appends tombstone frame, syncs, removes from KeyDir | `key []byte` | `error` | `ErrKeyEmpty`, `ErrKeyTooLarge`, `ErrDiskFull` | `docs/engine/06_PUT_GET_DELETE.md:23` |
| 10 | Mutation | Non-Existent Key DEL | Appends tombstone frame even if key was not present | `key []byte` (not in DB) | `error` (nil) | Succeeds, advances sequence, logs frame | `docs/engine/06_PUT_GET_DELETE.md:25` |
| 11 | Boundary | Zero-Byte Value PUT | Stores present key with 0-byte value | `key, []byte{}` | `error` (nil) | GET returns `([]byte{}, true, nil)` | `docs/engine/06_PUT_GET_DELETE.md:21` |
| 12 | Boundary | 1MB Value PUT | Stores 1,048,576 byte value across multi-block boundaries | `key, bytes(1048576)` | `error` (nil) | GET returns exact 1MB slice | `docs/engine/06_PUT_GET_DELETE.md:17` |
| 13 | Durability | Reopen Durability | Reconstructs KeyDir from persisted frames on open | `Open(path)` | `*Engine` with KeyDir | Replays frames sequentially | `.agents/rules/01_invariants.md:6-12` |
| 14 | Durability | Overwrite Resolution | Later PUT frame supersedes earlier PUT for same key | Multiple PUTs on `k` | Latest value on GET | Older versions skipped by KeyDir | `docs/engine/06_PUT_GET_DELETE.md:33` |
| 15 | Concurrency | Disjoint Writers (D1) | Concurrent goroutines writing different keys | Distinct keys | All keys durable | No torn frames, no race detector errors | `.agents/rules/03_scenarios_matrix.md:32` |
| 16 | Concurrency | Same-Key Writers (D2) | Concurrent goroutines updating the same key | Same key, distinct vals | Consistent single val | Never mixed/torn value bytes | `.agents/rules/03_scenarios_matrix.md:33` |
| 17 | Concurrency | Readers During Writes (D3) | Concurrent readers calling GET while writer writes | Concurrent keys | Consistent read states | Readers never block or observe errors | `.agents/rules/03_scenarios_matrix.md:34` |
| 18 | Concurrency | Un-torn Reads (D4) | Readers never observe intermediate/torn values | Rapid alternating PUTs | Either old or new val | Zero interleaved bytes | `docs/engine/11_CONCURRENCY_AND_LOCKING.md:21` |
| 19 | Hardening | Logical Quota Guard | Checks `validEOF + frameSize <= maxBytes` | `Options{MaxBytes: N}` | `ErrDiskFull` | Write aborted, file & KeyDir intact | `docs/engine/05_WRITE_AND_COMMIT_PROTOCOL.md:29` |
| 20 | Hardening | Binary Transparency | Keys and values with NUL, newlines, raw binary | `[]byte{0x00, 0xff...}` | Exact byte roundtrip | Zero C-string truncation or split | `docs/engine/06_PUT_GET_DELETE.md:29` |

---

## 4. Edge Cases

| # | Feature | Input | Observed / Expected Behavior |
|---|---------|-------|------------------------------|
| 1 | Process Lock | Second process opens while first is active | Fails immediately with typed `ErrLocked`; does not block indefinitely or corrupt DB |
| 2 | Process Lock | First process killed via `SIGKILL` | Kernel frees flock descriptor; second process opens immediately without manual lock removal |
| 3 | Process Lock | Primary closed cleanly | Second process opens DB successfully; lock sidecar `<path>.lock` is retained intact |
| 4 | Process Lock | Relative vs Absolute path opening | `Open("./test.db")` and `Open("test.db")` map to same lock file and properly conflict |
| 5 | Key Boundary | PUT with 0-byte key (`[]byte{}`) | Returns `format.ErrKeyEmpty`; no disk write or KeyDir modification |
| 6 | Key Boundary | PUT with 1-byte key (`[]byte("a")`) | Accepted; persists frame and updates KeyDir |
| 7 | Key Boundary | PUT with 1024-byte key | Accepted; maximum valid key length persists cleanly |
| 8 | Key Boundary | PUT with 1025-byte key | Returns `format.ErrKeyTooLarge`; zero disk mutation |
| 9 | Value Boundary | PUT with 0-byte value (`[]byte{}`) | Accepted; GET returns `[]byte{}` and `found=true` (distinct from `found=false`) |
| 10 | Value Boundary | PUT with 1MB value (1,048,576 bytes) | Accepted; survives reopen; GET returns identical 1,048,576 bytes |
| 11 | Value Boundary | PUT with 1MB + 1 byte (1,048,577 bytes) | Returns `format.ErrValueTooLarge`; zero disk mutation |
| 12 | Delete | DEL non-existent key | Returns `nil`; appends tombstone frame to disk; sequence advances; GET remains `found=false` |
| 13 | Delete | DEL followed by DEL of same key | Both succeed; appends two tombstones; GET remains `found=false` |
| 14 | Delete | PUT then DEL then PUT of same key | Reopen engine; GET returns second PUT's value (resurrected key) |
| 15 | Concurrency | 10 readers during 1MB value write | Readers see either previous value or new 1MB value; never partial buffer or crash |
| 16 | Concurrency | Rapid alternating PUTs ("A"*1000 vs "B"*1000) | Readers observe strictly 1000 'A's or 1000 'B's; zero mixed/torn bytes |
| 17 | Quota | Write exceeds configured `maxBytes` | Returns `ErrDiskFull`; validEOF unchanged; existing data untouched |
| 18 | Closed State | Calling `Put`, `Get`, `Delete` on closed engine | Returns `ErrClosed`; no panics or nil pointer dereferences |
| 19 | Closed State | Calling `Close()` twice on engine | Returns `ErrClosed` (or clean nil); no panics |
| 20 | Corrupt Header | Opening database with damaged header | Fails with `format.ErrCorruptHeader` or `format.ErrInvalidHeaderMagic`; refuses to wipe file |

---

## 5. Concrete Go Test Architecture & Suite Specification

Below is the concrete design for `internal/engine/engine_test.go`.

### 5.1 Subprocess Helper Harness (Self-Executing Test Binary)

```go
package engine_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"kvdb/internal/engine"
	"kvdb/internal/format"
)

const (
	envSubprocessKey  = "KVDB_TEST_SUBPROCESS"
	envSubprocessPath = "KVDB_TEST_DB_PATH"
	readySignal       = "LOCKED_READY\n"
)

func TestMain(m *testing.M) {
	// If executed in subprocess mode, act as lock-holding helper
	if os.Getenv(envSubprocessKey) == "1" {
		runLockHolderSubprocess()
		return
	}
	os.Exit(m.Run())
}

// runLockHolderSubprocess opens the database and holds the flock until signaled or killed.
func runLockHolderSubprocess() {
	dbPath := os.Getenv(envSubprocessPath)
	if dbPath == "" {
		fmt.Fprintln(os.Stderr, "missing KVDB_TEST_DB_PATH")
		os.Exit(2)
	}

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "subprocess open failed: %v\n", err)
		os.Exit(1)
	}

	// Signal to parent test runner that lock is acquired
	fmt.Print(readySignal)
	_ = os.Stdout.Sync()

	// Wait on stdin for clean shutdown signal ("CLOSE\n")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		text := scanner.Text()
		if text == "CLOSE" {
			_ = eng.Close()
			os.Exit(0)
		}
	}
	os.Exit(0)
}

// helper to spawn a background lock-holding subprocess
func spawnLockHolder(t *testing.T, dbPath string) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(),
		envSubprocessKey+"=1",
		envSubprocessPath+"="+dbPath,
	)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe failed: %v", err)
	}
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe failed: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start subprocess: %v", err)
	}

	// Read ready signal with timeout
	reader := bufio.NewReader(stdoutPipe)
	lineChan := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		line, err := reader.ReadString('\n')
		if err != nil {
			errChan <- err
			return
		}
		lineChan <- line
	}()

	select {
	case line := <-lineChan:
		if line != readySignal {
			_ = cmd.Process.Kill()
			t.Fatalf("unexpected subprocess signal: %q", line)
		}
	case err := <-errChan:
		_ = cmd.Process.Kill()
		t.Fatalf("subprocess read failed: %v", err)
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("subprocess lock timed out after 5s")
	}

	return cmd, stdinPipe
}
```

---

### 5.2 Process Concurrency & Flock Test Signatures

1. **`TestEngine_Flock_MutualExclusion(t *testing.T)`**:
   - Spawns subprocess on `dbPath`.
   - Parent calls `engine.Open(dbPath, engine.Options{})`.
   - Asserts: `errors.Is(err, engine.ErrLocked) == true` immediately without blocking.
   - Cleanly closes subprocess.

2. **`TestEngine_Flock_CleanCloseHandoff(t *testing.T)`**:
   - Spawns subprocess holding lock.
   - Verifies parent fails with `ErrLocked`.
   - Sends `"CLOSE\n"` to subprocess stdin; waits for subprocess to exit cleanly (code 0).
   - Parent calls `engine.Open(dbPath, engine.Options{})`.
   - Asserts: `err == nil`; parent successfully holds lock and closes.

3. **`TestEngine_Flock_SIGKILL_Recovery(t *testing.T)`**:
   - Spawns subprocess holding lock.
   - Verifies parent fails with `ErrLocked`.
   - Sends `cmd.Process.Kill()` (`SIGKILL`) to subprocess.
   - Subprocess dies abruptly without running defer or close code.
   - Parent immediately calls `engine.Open(dbPath, engine.Options{})`.
   - Asserts: `err == nil`; kernel releases `flock` instantly; no manual lock file cleanup is required.

4. **`TestEngine_Flock_SidecarPersistence(t *testing.T)`**:
   - Opens engine; verifies `dbPath + ".lock"` exists on disk.
   - Closes engine.
   - Asserts: `dbPath + ".lock"` STILL exists (never unlinked or deleted).

---

### 5.3 Single Mutation & Durability Test Signatures

1. **`TestEngine_Put_Get_Reopen(t *testing.T)`**:
   - Opens fresh engine.
   - `eng.Put([]byte("alpha"), []byte("beta"))`.
   - `val, found, err := eng.Get([]byte("alpha"))` $\to$ asserts `found == true`, `bytes.Equal(val, []byte("beta"))`.
   - `eng.Close()`.
   - `eng2, err := engine.Open(dbPath, ...)`
   - `val2, found2, err := eng2.Get([]byte("alpha"))` $\to$ asserts `found2 == true`, `bytes.Equal(val2, []byte("beta"))`.

2. **`TestEngine_Put_Overwrite_Reopen(t *testing.T)`**:
   - `eng.Put([]byte("key1"), []byte("initial_value"))`.
   - `eng.Put([]byte("key1"), []byte("overwritten_value"))`.
   - `val, found, _ := eng.Get([]byte("key1"))` $\to$ asserts `overwritten_value`.
   - Closes and reopens engine.
   - Asserts: GET returns `overwritten_value`. Verifies file contains two appended frames and locator points to latest offset.

3. **`TestEngine_Put_EmptyValue_Boundary(t *testing.T)`**:
   - `eng.Put([]byte("empty_key"), []byte{})`.
   - `val, found, err := eng.Get([]byte("empty_key"))`.
   - Asserts: `err == nil`, `found == true`, `len(val) == 0`.
   - Reopens engine; GET still returns `found == true`, `len(val) == 0`.
   - GET of `"non_existent"` returns `found == false`, `val == nil`.

4. **`TestEngine_Put_1MB_Value_Boundary(t *testing.T)`**:
   - Allocates 1 MiB slice (`1048576` bytes) with deterministic pattern (`val[i] = byte(i % 251)`).
   - `eng.Put([]byte("large_key"), val)`.
   - Reopens engine.
   - `readVal, found, err := eng2.Get([]byte("large_key"))`.
   - Asserts: `found == true`, `len(readVal) == 1048576`, `bytes.Equal(readVal, val)`.

5. **`TestEngine_Delete_ExistingKey(t *testing.T)`**:
   - `eng.Put([]byte("to_del"), []byte("temp"))`.
   - `eng.Delete([]byte("to_del"))`.
   - `_, found, _ := eng.Get([]byte("to_del"))` $\to$ asserts `found == false`.
   - Reopens engine.
   - Asserts: `found == false`.

6. **`TestEngine_Delete_NonExistentKey(t *testing.T)`**:
   - Records file size before delete.
   - `err := eng.Delete([]byte("never_existed"))`.
   - Asserts: `err == nil` (success per official rules).
   - Records file size after delete: asserts size increased by exactly 1 UABE DEL frame.
   - Reopens engine; asserts GET returns `found == false`.

7. **`TestEngine_InputValidation(t *testing.T)`**:
   - Table-driven tests validating:
     - Key length 0 $\to$ `format.ErrKeyEmpty`.
     - Key length 1025 $\to$ `format.ErrKeyTooLarge`.
     - Value length 1MB + 1 $\to$ `format.ErrValueTooLarge`.
     - Delete key length 0 $\to$ `format.ErrKeyEmpty`.
     - Delete key length 1025 $\to$ `format.ErrKeyTooLarge`.

8. **`TestEngine_BinaryKeysAndValues(t *testing.T)`**:
   - Tests binary keys: `[]byte{0x00, 0x01, 0x02, 0xff, 0xfe, '\n', '\r', '\t'}`.
   - Tests binary values containing all 256 byte values (`0x00` through `0xFF`).
   - Asserts perfect byte roundtrip and persistence across reopen.

---

### 5.4 Concurrency Tests (Scenarios D1–D4)

1. **`TestEngine_Concurrency_D1_DisjointWriters(t *testing.T)`**:
   - Spawns 8 writer goroutines.
   - Each writer writes 100 unique keys (`key_worker_id_idx`).
   - Uses `sync.WaitGroup` to wait for all writers.
   - Asserts: zero errors during writes.
   - Closes and reopens engine.
   - Asserts: all 800 keys are present with their exact expected values.

2. **`TestEngine_Concurrency_D2_SameKeyWriters(t *testing.T)`**:
   - Spawns 8 writer goroutines all mutating `"hot_key"`.
   - Each writer writes 100 tagged values (`fmt.Sprintf("worker_%d_seq_%d", id, i)`).
   - Asserts: zero write errors.
   - At conclusion and upon reopen, GET returns a valid string produced by one of the writers; never a mixed or corrupt payload.

3. **`TestEngine_Concurrency_D3_ReadersDuringWrites(t *testing.T)`**:
   - 4 writer goroutines inserting and updating 50 keys.
   - 10 reader goroutines continuously calling `Get()` on random keys from the set.
   - Runs for 2 seconds using `context.WithTimeout`.
   - Asserts: readers never return an error or torn state; if `found == true`, the value matches the writer payload format.

4. **`TestEngine_Concurrency_D4_NoTornReads(t *testing.T)`**:
   - Single writer alternates writing 1,000 bytes of `'A'` and 1,000 bytes of `'B'` to `"flip_key"`.
   - 8 reader goroutines continuously read `"flip_key"` in a tight loop.
   - For every successful read, reader asserts:
     - `val` consists of 1,000 bytes of `'A'` OR 1,000 bytes of `'B'`.
     - If any byte does not match the first byte, test fails immediately with `"TORN READ DETECTED"`.

---

### 5.5 Quota & Lifecycle Guard Tests

1. **`TestEngine_Quota_MaxBytesExceeded(t *testing.T)`**:
   - Configures engine with `Options{MaxBytes: 150}`.
   - Database header is 32 bytes.
   - First write (`Put("k1", "small")` ~ 47 bytes) succeeds $\to$ file size = 79 bytes.
   - Second write (`Put("k2", bytes(100))` ~ 140 bytes $\to$ total 219 > 150) fails with `engine.ErrDiskFull`.
   - Asserts: `"k1"` remains readable; file size remains 79 bytes; subsequent `Open` succeeds.

2. **`TestEngine_ClosedEngine_Rejection(t *testing.T)`**:
   - Closes engine.
   - Calling `Put`, `Get`, `Delete` all return `engine.ErrClosed`.
   - Calling `Close()` again returns `engine.ErrClosed`.

---

## 6. Caveats

1. **Flock Semantics on NFS/Network Filesystems**: Advisory `flock` behavior can vary on NFS/CIFS. The hackathon environment runs on local Linux filesystems (`ext4`/`xfs`/`tmpfs`), where `flock` is strictly kernel-enforced and reliable.
2. **Subprocess Signal Timing**: In `spawnLockHolder`, reading from the subprocess stdout pipe requires an explicit flush (`os.Stdout.Sync()`) in the subprocess to ensure the parent does not block on buffered I/O.
3. **Phase M2 Scope Boundary**: Batch mutations (`Batch`), startup log replay of damaged non-tail frames, and compaction (`Compact`) belong to Milestones M3 and M5; Milestone M2 focuses strictly on single-op PUT/GET/DEL, KeyDir locators, and `flock` process lifecycle.

---

## 7. Conclusion

The specification for Milestone M2 is completely mined, unambiguous, and fully aligns with:
- `docs/engine/PHASES.md § E2 & E3`
- `docs/engine/11_CONCURRENCY_AND_LOCKING.md`
- `.agents/rules/01_invariants.md`
- `.agents/rules/03_scenarios_matrix.md` (Scenarios A1–A7, D1–D4)

The test architecture formulated above provides a 100% standard-library, offline test harness covering process-level concurrency, abrupt `SIGKILL` lock recovery, durability across cold restarts, boundary value handling (0B and 1MB), idempotent non-existent key deletes, and un-torn concurrent reader protection.

---

## 8. Verification Method

To verify the test suite when implemented:
```bash
# 1. Run all engine unit and concurrency tests with the Go race detector enabled:
go test -v -race -timeout 30s ./internal/engine/...

# 2. Specifically verify flock and process isolation tests:
go test -v -run "TestEngine_Flock_*" ./internal/engine/

# 3. Specifically verify concurrency scenarios D1-D4:
go test -v -race -run "TestEngine_Concurrency_*" ./internal/engine/
```
