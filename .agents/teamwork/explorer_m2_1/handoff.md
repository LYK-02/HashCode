# Phase E2 Implementation Strategy: Engine Lifecycle & Locks

## 1. Observation

Direct observations from the codebase, project specifications, and test harnesses:

1. **Existing Format Codec (`internal/format/`)**:
   - `internal/format/header.go`:
     - Line 10: `HeaderSize = 32`, `HeaderPreCRCSize = 28`, `HeaderMagicStr = "KVDB2026"`, `FormatVersion1 = 1`.
     - Lines 42–50: `NewHeader(compactionGen uint64) Header` initializes header with `KVDB2026`, version 1, and compaction generation.
     - Lines 53–59: `EncodeHeader(h *Header) ([]byte, error)` serializes 32-byte header with LittleEndian encoding and Castagnoli CRC32C over bytes 0..27.
     - Lines 94–125: `DecodeHeader(b []byte) (*Header, error)` checks magic `"KVDB2026"`, version 1, reserved == 0, and Castagnoli CRC32C. Returns typed errors: `ErrHeaderTooShort`, `ErrInvalidHeaderMagic`, `ErrUnsupportedVersion`, `ErrReservedNonZero`, `ErrCorruptHeader`.
   - `internal/format/errors.go`:
     - Lines 6–14: Format sentinel errors for header validation.
   - All format tests currently pass cleanly via `go test -v ./internal/format/...` (0.066s).

2. **Existing Engine Skeleton (`internal/engine/engine.go`)**:
   - Lines 12–17: Sentinel errors defined:
     ```go
     ErrLocked         = errors.New("LOCKED: database lock held by another process")
     ErrClosed         = errors.New("CLOSED: engine instance has been closed")
     ErrNotFound       = errors.New("NOTFOUND: key not present in database")
     ErrDiskFull       = errors.New("DISK_FULL: file size limit exceeded")
     ErrEnginePoisoned = errors.New("ENGINE_POISONED: previous write rollback failed")
     ```
   - Lines 38–50: `Engine` struct definition currently has:
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
     }
     ```
   - Lines 53–59: `Open(path string, opts Options) (*Engine, error)` is an M1 stub returning an uninitialized in-memory engine without acquiring file locks or creating data files.
   - Lines 61–67: `Close()` is an M1 stub setting `e.closed = true`.

3. **Phase E2 Specification (`docs/engine/PHASES.md § E2`)**:
   - Lines 78–87:
     ```
     - Canonicalize database path.
     - Open permanent <db>.lock sidecar.
     - Acquire exclusive flock before recovery or temp cleanup.
     - Open/create database file.
     - On first creation, write header completely.
     - Sync header and parent directory before accepting mutations.
     - Initialize mutex, KeyDir, valid EOF, next sequence and generation.
     - Define clean Close behaviour.
     - Never unlink/replace the lock sidecar.
     ```
   - Lines 90–94:
     ```
     - No API call after Close.
     - Lock order: process flock, then engine mutex.
     - A second process must block or fail cleanly.
     - SIGKILL must release OS lock automatically.
     - Do not destructively reinitialize a potentially populated invalid-header file.
     ```
   - Lines 97–100: Exit gates:
     - Two processes cannot own same database concurrently.
     - Kill lock holder; a new opener acquires lock.
     - Initial header survives close/reopen.
     - Empty DB opens with correct state.

4. **Concurrency & Locking Specification (`docs/engine/11_CONCURRENCY_AND_LOCKING.md`)**:
   - Lines 25–26: "Cross-process lock: permanent `<db>.lock` sidecar, exclusive flock from open through close. Lock the sidecar before recovery, compaction, or temp cleanup. Do not unlink or rename the lock file: flock attaches to inode, and replacing the path can split ownership across two inodes. A second process must block or fail cleanly, not open concurrently as an independent writer."
   - Line 33: "On SIGKILL, OS closes file descriptors and releases advisory lock automatically. The sidecar file remains. On normal Close, sync any required state, close database descriptor, release/close lock descriptor, and return the first meaningful error."

5. **Test Harness & E2E Validation (`tests/e2e/tier1_test.go`)**:
   - Lines 23–47 (`TestTier1_01_Open_NewDatabaseCreatesHeader`): Tests that `kvdb open <path>` creates file $\ge 32$ bytes with first 8 bytes == `"KVDB2026"`.
   - Lines 62–105 (`TestTier1_03`–`TestTier1_05`): Corrupted magic byte 0, invalid version byte 8, or corrupt CRC byte 28 causes `open` to fail non-zero.
   - Lines 111–149 (`TestTier1_06`–`TestTier1_07`): Checks sidecar file `<path>.lock` exists and concurrent open is rejected when flock is held by another process.
   - Lines 175–187 (`TestTier1_09`): Verifies lock is released upon normal process exit.

---

## 2. Logic Chain

From the observations above, we establish the following step-by-step reasoning:

1. **Canonical Path Resolution & Directory Sync** (Ref: Observation 3, 5):
   - User paths can be relative (e.g. `./data.db`, `foo/../data.db`). If paths are not canonicalized via `filepath.Abs` and `filepath.Clean`, lock paths and directory syncing could target ambiguous filesystem locations.
   - Extracting `parentDir := filepath.Dir(cleanPath)` and ensuring `os.MkdirAll(parentDir, 0755)` prevents errors when initializing databases in new subdirectories.
   - In Linux POSIX filesystems (ext4, xfs), creating a new directory entry (`open(O_CREAT)`) modifies parent directory data blocks. Physical persistence of this link requires calling `fsync()` on an open directory file descriptor (`syncDir(parentDir)`).

2. **Sidecar Lock Inode Preservation Invariant** (Ref: Observation 3, 4):
   - Linux `flock(2)` is advisory and binds to the open file table description and the underlying filesystem inode.
   - If an engine unlinks, removes, or renames `<path>.lock` on close or cleanup, another process creating a new `<path>.lock` will allocate a different inode and successfully acquire an exclusive flock on that new inode while an existing process still holds the old inode.
   - Therefore, `<path>.lock` must be created with `os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)` and **NEVER unlinked, deleted, or renamed under any circumstance**.

3. **Process Mutual Exclusion via Non-Blocking Flock** (Ref: Observation 2, 3, 4, 5):
   - Calling `syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)` attempts immediate exclusive acquisition.
   - If another process holds the lock, the syscall returns `syscall.EWOULDBLOCK` (or `syscall.EAGAIN` on Linux).
   - In this contention event, the engine must close the newly opened lock descriptor (`_ = lockFile.Close()`) and immediately return `ErrLocked`.
   - When a process terminates normally, `Close()` explicitly unlocks (`syscall.LOCK_UN`) and closes the descriptor.
   - When a process is killed abruptly by SIGKILL (`kill -9`), the Linux kernel automatically closes all open file descriptors for that process table entry, which atomically releases the advisory flock on the inode. The sidecar file remains on disk with 0 bytes, allowing the next opening process to acquire the lock without manual cleanup.

4. **Data File Initialization & Fail-Closed Header Protection** (Ref: Observation 1, 3, 5):
   - Opening the data file with `os.OpenFile(cleanPath, os.O_CREATE|os.O_RDWR, 0666)`:
     - Case A: `fileInfo.Size() == 0`:
       The database is brand new. Encode a 32-byte header with Generation 0 (`format.NewHeader(0)`). Write 32 bytes at offset 0 (`dataFile.WriteAt`), sync the file (`dataFile.Sync()`), and sync the parent directory (`syncDir(parentDir)`). Set `validEOF = 32`, `nextSeqNo = 1`, and `compactionGen = 0`.
     - Case B: `fileInfo.Size() >= 32`:
       The database already exists. Read the first 32 bytes at offset 0, decode via `format.DecodeHeader(hdrBuf)`. If decoding fails (invalid magic, version $\ne 1$, non-zero reserved, or CRC32C mismatch), fail closed immediately and return the specific format error. **Never overwrite or reinitialize a corrupt existing header.**
     - Case C: `0 < fileInfo.Size() < 32`:
       An existing file has fewer than 32 bytes. Return `format.ErrHeaderTooShort` and fail closed.
   - Deferred cleanup pattern: During `Open()`, maintain a boolean flag `cleanup = true`. If any error occurs between acquiring the flock and successfully returning the `*Engine`, defer executes `releaseLock(lockFile)` and `dataFile.Close()`, ensuring no dangling descriptors or locks on failure.

5. **Strict Lock Ordering Hierarchy** (Ref: Observation 3, 4):
   - Fixed lock order:
     1. OS Process `flock` on `<path>.lock` (acquired at `Open()`).
     2. Engine `sync.RWMutex` (`e.mu`):
        - Exclusive `e.mu.Lock()` for mutations (`Put`, `Delete`, `Batch`, `Compact`, `Close`).
        - Shared `e.mu.RLock()` for queries (`Get`, `Dump`, `Check`).
   - Reverse lock ordering is impossible because flock is acquired prior to returning `*Engine`.
   - Post-close rejection: All public methods check `if e.closed { return ErrClosed }` under `e.mu`.

6. **Deterministic Clean `Close()` Sequence** (Ref: Observation 2, 3, 4):
   - `e.mu.Lock()`
   - If already closed, return `nil` (idempotent closer).
   - Set `e.closed = true`.
   - Call `e.db.Sync()` to flush kernel buffers.
   - Close `e.db.Close()`.
   - Call `syscall.Flock(int(e.lockFile.Fd()), syscall.LOCK_UN)`.
   - Close `e.lockFile.Close()`.
   - Retain `<path>.lock` on disk.
   - Return first non-nil error.

---

## 3. Caveats

1. **Recovery Scanner Deferred to Phase E5**:
   - In Phase E2, for an existing file with `size > 32` bytes containing valid UABE frames, recovery replay logic (scanning frames from byte 32 to valid EOF, torn-tail truncation) belongs to Phase E5 (`internal/engine/recovery.go`). In Phase E2, `validEOF` is set to 32 for an empty DB, with an extensible hook prepared for the recovery scanner.
2. **Directory Sync on Non-POSIX / Network Filesystems**:
   - On Linux ext4/xfs/btrfs, opening a directory and calling `Sync()` (`fsync(dirfd)`) is supported and mandatory for directory entry durability. On filesystems that do not support directory fsync (e.g., Windows FAT/NTFS or some CIFS mounts returning `EINVAL`), errors can be ignored if needed; however, Linux is the hackathon target OS where directory sync is standard.
3. **In-Process Multiple Instances**:
   - In Linux POSIX, `flock` locks are per-open-file description. If the same process opens `<path>.lock` a second time via a new `os.OpenFile` call, Linux `flock` will correctly return `EWOULDBLOCK`. However, in-process goroutines using the same `*Engine` instance synchronize via `e.mu` (`sync.RWMutex`).

---

## 4. Conclusion & Concrete Strategy

### Recommended File Decomposition

Structure Phase E2 across two clean files in `internal/engine/`:
1. `internal/engine/lock.go` — OS-level file locking routines and lock lifecycle.
2. `internal/engine/engine.go` — Engine struct, `Open`, `Close`, directory sync, lifecycle enforcement, and method stubs.

---

### Concrete Implementation Blueprint

#### `internal/engine/lock.go`

```go
package engine

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// acquireLock creates or opens the sidecar lock file and acquires an exclusive non-blocking flock.
// On contention, it returns ErrLocked. The lock file is NEVER unlinked or deleted.
func acquireLock(lockPath string) (*os.File, error) {
	// Open sidecar with O_CREATE|O_RDWR without truncation
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file %s: %w", lockPath, err)
	}

	// Non-blocking exclusive advisory flock
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("failed to acquire flock on %s: %w", lockPath, err)
	}

	return f, nil
}

// releaseLock explicitly unlocks and closes the lock file descriptor.
// It deliberately leaves the sidecar file intact on the filesystem.
func releaseLock(f *os.File) error {
	if f == nil {
		return nil
	}
	var firstErr error
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		firstErr = err
	}
	if err := f.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}
```

---

#### `internal/engine/engine.go`

```go
package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// KeyDirEntry stores in-memory index locators pointing directly to value bytes in the data file.
type KeyDirEntry struct {
	FileOffset uint64
	ValueLen   uint32
	SeqNo      uint64
}

// Pair represents a live key-value pair returned by Dump.
type Pair struct {
	Key   string
	Value []byte
}

// Options configures engine operational parameters.
type Options struct {
	MaxBytes uint64
}

// Engine implements the single-file append-only key-value storage engine.
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

// Open canonicalizes path, acquires the sidecar lock, initializes/validates the data file header,
// and returns a ready Engine instance.
func Open(path string, opts Options) (*Engine, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	cleanPath := filepath.Clean(absPath)
	parentDir := filepath.Dir(cleanPath)

	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", parentDir, err)
	}

	lockPath := cleanPath + ".lock"
	lockFile, err := acquireLock(lockPath)
	if err != nil {
		return nil, err
	}

	// Defensive cleanup on initialization error
	cleanup := true
	defer func() {
		if cleanup {
			_ = releaseLock(lockFile)
		}
	}()

	dataFile, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open data file %s: %w", cleanPath, err)
	}
	defer func() {
		if cleanup {
			_ = dataFile.Close()
		}
	}()

	fi, err := dataFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat data file: %w", err)
	}

	var (
		validEOF      uint64
		nextSeqNo     uint64 = 1
		compactionGen uint64 = 0
	)

	if fi.Size() == 0 {
		// New database: write 32-byte DB Header (Gen 0)
		hdr := format.NewHeader(0)
		hdrBytes, err := format.EncodeHeader(&hdr)
		if err != nil {
			return nil, fmt.Errorf("failed to encode db header: %w", err)
		}
		if _, err := dataFile.WriteAt(hdrBytes, 0); err != nil {
			return nil, fmt.Errorf("failed to write db header: %w", err)
		}
		if err := dataFile.Sync(); err != nil {
			return nil, fmt.Errorf("failed to sync db header: %w", err)
		}
		if err := syncDir(parentDir); err != nil {
			return nil, fmt.Errorf("failed to sync parent dir: %w", err)
		}
		validEOF = uint64(format.HeaderSize)
	} else if fi.Size() < format.HeaderSize {
		// Non-empty file with incomplete header
		return nil, format.ErrHeaderTooShort
	} else {
		// Existing database: read and validate header at offset 0
		hdrBuf := make([]byte, format.HeaderSize)
		if _, err := dataFile.ReadAt(hdrBuf, 0); err != nil {
			return nil, fmt.Errorf("failed to read db header: %w", err)
		}
		hdr, err := format.DecodeHeader(hdrBuf)
		if err != nil {
			return nil, err // Fail closed: ErrCorruptHeader, ErrInvalidHeaderMagic, etc.
		}
		compactionGen = hdr.CompactionGen
		validEOF = uint64(format.HeaderSize)
	}

	keyDir := make(map[string]KeyDirEntry)

	// Engine successfully initialized; transfer ownership of descriptors
	cleanup = false

	return &Engine{
		path:          cleanPath,
		parentDir:     parentDir,
		db:            dataFile,
		lockFile:      lockFile,
		maxBytes:      opts.MaxBytes,
		validEOF:      validEOF,
		nextSeqNo:     nextSeqNo,
		compactionGen: compactionGen,
		keyDir:        keyDir,
		closed:        false,
	}, nil
}

// Close flushes buffers, syncs data file to physical disk, closes descriptors,
// and releases the flock. The lock sidecar is preserved on disk.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}
	e.closed = true

	var firstErr error

	if e.db != nil {
		if err := e.db.Sync(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to sync db: %w", err)
		}
		if err := e.db.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close db: %w", err)
		}
		e.db = nil
	}

	if e.lockFile != nil {
		if err := releaseLock(e.lockFile); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to release lock: %w", err)
		}
		e.lockFile = nil
	}

	return firstErr
}

// syncDir forces the directory entry containing the database file to physical disk.
func syncDir(dirPath string) error {
	d, err := os.Open(dirPath)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Stubs with lifecycle checking for methods implemented in subsequent phases:

func (e *Engine) Put(key, val []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrClosed
	}
	return nil
}

func (e *Engine) Get(key []byte) ([]byte, bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.closed {
		return nil, false, ErrClosed
	}
	return nil, false, nil
}

func (e *Engine) Delete(key []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrClosed
	}
	return nil
}

func (e *Engine) Batch(ops []format.Operation) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrClosed
	}
	return nil
}

func (e *Engine) Dump() ([]Pair, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.closed {
		return nil, ErrClosed
	}
	return nil, nil
}

func (e *Engine) Compact() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return ErrClosed
	}
	return nil
}

func (e *Engine) Check() error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.closed {
		return ErrClosed
	}
	return nil
}
```

---

## 5. Verification Method

To independently verify the Phase E2 implementation, the implementer must execute:

1. **Unit Testing (`internal/engine/engine_test.go` and `internal/engine/lock_test.go`)**:
   - Command: `go test -v -race ./internal/engine/...`
   - Test cases:
     - `TestEngine_OpenNewDatabase`: Validates creation of 32-byte header with magic `KVDB2026`, version 1, and valid CRC32C.
     - `TestEngine_OpenExistingDatabase`: Validates reopening existing database without overwriting header.
     - `TestEngine_LockSidecarCreated`: Confirms `<path>.lock` file exists and has mode 0666.
     - `TestEngine_ConcurrentLockContention`: Runs secondary lock acquisition in child goroutine/process and asserts `ErrLocked` is returned.
     - `TestEngine_SIGKILLRelease`: Launches subprocess holding flock, kills with SIGKILL (`cmd.Process.Kill()`), verifies parent immediately acquires flock.
     - `TestEngine_CorruptHeaderRejection`: Flips magic byte, version byte, and CRC byte; asserts `Open` fails with format error without altering the file.
     - `TestEngine_ClosedAPIRejection`: Confirms `Put`, `Get`, `Delete`, `Batch`, `Dump`, `Check` return `ErrClosed` after `Close()`.
     - `TestEngine_IdempotentClose`: Calling `Close()` multiple times returns `nil` without panics or errors.

2. **Full Workspace Build & Test**:
   - Command: `make all && make test`
   - Validates offline compilation (`GOPROXY=off`, `CGO_ENABLED=0`) and regression-free test passes across all packages.

3. **Invalidation Conditions**:
   - Any unlinking or deletion of `<path>.lock` at any time.
   - Any destructive overwrite or truncation of an existing file when header decoding fails.
   - Any inversion of the lock order (acquiring `flock` while holding `sync.RWMutex`).
   - Failure to sync parent directory or data file on new database initialization.
