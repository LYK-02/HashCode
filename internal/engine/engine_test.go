package engine_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
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
	if os.Getenv(envSubprocessKey) == "1" {
		runLockHolderSubprocess()
		return
	}
	os.Exit(m.Run())
}

// runLockHolderSubprocess opens the database and holds the flock until signaled on stdin or killed.
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

// ----------------------------------------------------------------------------
// 1. Lifecycle and Lock Tests (Phase E2)
// ----------------------------------------------------------------------------

func TestEngine_OpenNewDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "new.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	if eng.KeyCount() != 0 {
		t.Errorf("expected 0 keys, got %d", eng.KeyCount())
	}
	if eng.ValidEOF() != uint64(format.HeaderSize) {
		t.Errorf("expected validEOF=%d, got %d", format.HeaderSize, eng.ValidEOF())
	}
	if eng.NextSeqNo() != 1 {
		t.Errorf("expected nextSeqNo=1, got %d", eng.NextSeqNo())
	}

	// Verify header bytes in file
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("failed to read db file: %v", err)
	}
	if len(data) != format.HeaderSize {
		t.Errorf("expected file size %d, got %d", format.HeaderSize, len(data))
	}
	if string(data[:8]) != format.HeaderMagicStr {
		t.Errorf("expected magic %q, got %q", format.HeaderMagicStr, string(data[:8]))
	}

	// Verify lock sidecar exists
	lockPath := dbPath + ".lock"
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file does not exist: %v", err)
	}
}

func TestEngine_OpenExistingDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "existing.db")

	eng1, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	if err := eng1.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	// Reopen
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer eng2.Close()

	if eng2.KeyCount() != 0 {
		t.Errorf("expected 0 keys, got %d", eng2.KeyCount())
	}
}

func TestEngine_Lock_SidecarPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist_lock.db")
	lockPath := dbPath + ".lock"

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file missing while open: %v", err)
	}

	if err := eng.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// The lock file MUST NEVER be unlinked or deleted
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file was deleted after Close(): %v", err)
	}
}

func TestEngine_Flock_MutualExclusion(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "flock_mutex.db")

	cmd, stdin := spawnLockHolder(t, dbPath)
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	// Secondary Open must fail immediately with ErrLocked
	_, err := engine.Open(dbPath, engine.Options{})
	if !errors.Is(err, engine.ErrLocked) {
		t.Fatalf("expected ErrLocked, got: %v", err)
	}

	// Signal subprocess to close
	_, _ = io.WriteString(stdin, "CLOSE\n")
	_ = cmd.Wait()

	// Once closed, parent acquires lock cleanly
	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("expected clean Open after handoff, got: %v", err)
	}
	_ = eng.Close()
}

func TestEngine_Flock_CleanCloseHandoff(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "handoff.db")

	cmd, stdin := spawnLockHolder(t, dbPath)
	_, _ = io.WriteString(stdin, "CLOSE\n")
	_ = cmd.Wait()

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed after clean handoff: %v", err)
	}
	_ = eng.Close()
}

func TestEngine_Flock_SIGKILL_Recovery(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sigkill.db")

	cmd, stdin := spawnLockHolder(t, dbPath)
	defer stdin.Close()

	// Confirm lock is held
	_, err := engine.Open(dbPath, engine.Options{})
	if !errors.Is(err, engine.ErrLocked) {
		t.Fatalf("expected ErrLocked while holder alive, got: %v", err)
	}

	// Kill lock holder abruptly with SIGKILL
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("Kill failed: %v", err)
	}
	_ = cmd.Wait()

	// Kernel releases lock automatically; next Open must succeed immediately
	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed after SIGKILL of holder: %v", err)
	}
	_ = eng.Close()
}

func TestEngine_CorruptHeaderRejection(t *testing.T) {
	tmpDir := t.TempDir()

	// Case 1: Header too short (< 32 bytes)
	shortPath := filepath.Join(tmpDir, "short.db")
	if err := os.WriteFile(shortPath, []byte("KVDB2026_short"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := engine.Open(shortPath, engine.Options{})
	if !errors.Is(err, format.ErrHeaderTooShort) {
		t.Errorf("expected ErrHeaderTooShort, got: %v", err)
	}

	// Case 2: Corrupted magic bytes
	magicPath := filepath.Join(tmpDir, "bad_magic.db")
	eng, err := engine.Open(magicPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = eng.Close()

	f, err := os.OpenFile(magicPath, os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteAt([]byte("BADMAGIC"), 0)
	_ = f.Close()

	_, err = engine.Open(magicPath, engine.Options{})
	if !errors.Is(err, format.ErrInvalidHeaderMagic) {
		t.Errorf("expected ErrInvalidHeaderMagic, got: %v", err)
	}

	// Case 3: Corrupted CRC byte
	crcPath := filepath.Join(tmpDir, "bad_crc.db")
	eng2, err := engine.Open(crcPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = eng2.Close()

	f2, err := os.OpenFile(crcPath, os.O_RDWR, 0644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f2.WriteAt([]byte{0xFF}, 28)
	_ = f2.Close()

	_, err = engine.Open(crcPath, engine.Options{})
	if !errors.Is(err, format.ErrCorruptHeader) {
		t.Errorf("expected ErrCorruptHeader, got: %v", err)
	}
}

// ----------------------------------------------------------------------------
// 2. Single Operations Tests (Phase E3)
// ----------------------------------------------------------------------------

func TestEngine_Put_Get_Single(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "single.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	key := []byte("hello")
	val := []byte("world")

	if err := eng.Put(key, val); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, found, err := eng.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if !bytes.Equal(got, val) {
		t.Fatalf("expected %q, got %q", val, got)
	}

	// Non-existent key
	gotMissing, foundMissing, err := eng.Get([]byte("non_existent"))
	if err != nil {
		t.Fatalf("Get missing failed: %v", err)
	}
	if foundMissing {
		t.Fatal("expected found=false for missing key")
	}
	if gotMissing != nil {
		t.Fatal("expected nil slice for missing key")
	}
}

func TestEngine_Put_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "overwrite.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	key := []byte("key1")
	if err := eng.Put(key, []byte("val1")); err != nil {
		t.Fatal(err)
	}
	if err := eng.Put(key, []byte("val2")); err != nil {
		t.Fatal(err)
	}

	got, found, err := eng.Get(key)
	if err != nil || !found {
		t.Fatalf("Get failed: %v, found=%v", err, found)
	}
	if !bytes.Equal(got, []byte("val2")) {
		t.Fatalf("expected 'val2', got %q", got)
	}
	if eng.KeyCount() != 1 {
		t.Errorf("expected 1 key, got %d", eng.KeyCount())
	}
}

func TestEngine_Put_EmptyValue_Boundary(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty_val.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	key := []byte("empty_key")
	if err := eng.Put(key, []byte{}); err != nil {
		t.Fatalf("Put empty value failed: %v", err)
	}

	got, found, err := eng.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("empty value key must return found=true (distinct from NOTFOUND)")
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 bytes, got %d bytes", len(got))
	}
}

func TestEngine_Put_1MB_Value_Boundary(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "1mb.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	key := bytes.Repeat([]byte("k"), format.MaxKeyLen) // 1024 bytes
	largeVal := make([]byte, format.MaxValLen)          // 1,048,576 bytes
	for i := range largeVal {
		largeVal[i] = byte(i % 251)
	}

	if err := eng.Put(key, largeVal); err != nil {
		t.Fatalf("Put 1MB value failed: %v", err)
	}

	got, found, err := eng.Get(key)
	if err != nil || !found {
		t.Fatalf("Get 1MB failed: %v, found=%v", err, found)
	}
	if len(got) != format.MaxValLen {
		t.Fatalf("expected %d bytes, got %d", format.MaxValLen, len(got))
	}
	if !bytes.Equal(got, largeVal) {
		t.Fatal("1MB payload content mismatch")
	}
}

func TestEngine_Delete_ExistingKey(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "del.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	key := []byte("to_del")
	if err := eng.Put(key, []byte("val")); err != nil {
		t.Fatal(err)
	}
	if err := eng.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, found, err := eng.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("deleted key should return found=false")
	}
	if eng.KeyCount() != 0 {
		t.Errorf("expected 0 keys, got %d", eng.KeyCount())
	}
}

func TestEngine_Delete_NonExistentKey(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "del_non_existent.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	beforeEOF := eng.ValidEOF()

	// Deleting non-existent key must succeed per official specification
	if err := eng.Delete([]byte("missing_key")); err != nil {
		t.Fatalf("Delete of missing key returned error: %v", err)
	}

	// Valid tombstone frame appended to disk
	if eng.ValidEOF() <= beforeEOF {
		t.Errorf("expected validEOF to advance for tombstone frame, got %d <= %d", eng.ValidEOF(), beforeEOF)
	}
	if eng.KeyCount() != 0 {
		t.Errorf("expected 0 keys, got %d", eng.KeyCount())
	}
}

func TestEngine_Delete_IdempotentDoubleDelete(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "double_del.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	key := []byte("revive_key")
	_ = eng.Put(key, []byte("v1"))
	_ = eng.Delete(key)
	if err := eng.Delete(key); err != nil {
		t.Fatalf("second delete failed: %v", err)
	}

	// Reinsert after double delete
	if err := eng.Put(key, []byte("v2")); err != nil {
		t.Fatalf("reinsert failed: %v", err)
	}

	got, found, err := eng.Get(key)
	if err != nil || !found {
		t.Fatalf("Get failed: %v, found=%v", err, found)
	}
	if !bytes.Equal(got, []byte("v2")) {
		t.Fatalf("expected 'v2', got %q", got)
	}
}

func TestEngine_InputValidation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "input_val.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	// Empty key
	if err := eng.Put([]byte{}, []byte("v")); !errors.Is(err, format.ErrKeyEmpty) {
		t.Errorf("expected ErrKeyEmpty, got: %v", err)
	}
	if err := eng.Delete([]byte{}); !errors.Is(err, format.ErrKeyEmpty) {
		t.Errorf("expected ErrKeyEmpty for Del, got: %v", err)
	}
	if _, _, err := eng.Get([]byte{}); !errors.Is(err, format.ErrKeyEmpty) {
		t.Errorf("expected ErrKeyEmpty for Get, got: %v", err)
	}

	// Key too large (> 1024)
	tooLargeKey := make([]byte, format.MaxKeyLen+1)
	if err := eng.Put(tooLargeKey, []byte("v")); !errors.Is(err, format.ErrKeyTooLarge) {
		t.Errorf("expected ErrKeyTooLarge, got: %v", err)
	}
	if err := eng.Delete(tooLargeKey); !errors.Is(err, format.ErrKeyTooLarge) {
		t.Errorf("expected ErrKeyTooLarge for Del, got: %v", err)
	}
	if _, _, err := eng.Get(tooLargeKey); !errors.Is(err, format.ErrKeyTooLarge) {
		t.Errorf("expected ErrKeyTooLarge for Get, got: %v", err)
	}

	// Value too large (> 1MB)
	tooLargeVal := make([]byte, format.MaxValLen+1)
	if err := eng.Put([]byte("k"), tooLargeVal); !errors.Is(err, format.ErrValueTooLarge) {
		t.Errorf("expected ErrValueTooLarge, got: %v", err)
	}
}

func TestEngine_BinaryKeysAndValues(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "binary.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}

	binaryKey := []byte{0x00, 0x01, 0x02, 0xff, 0xfe, '\n', '\r', '\t'}
	binaryVal := make([]byte, 256)
	for i := 0; i < 256; i++ {
		binaryVal[i] = byte(i)
	}

	if err := eng.Put(binaryKey, binaryVal); err != nil {
		t.Fatalf("Put binary failed: %v", err)
	}
	_ = eng.Close()

	// Reopen and verify
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng2.Close()

	got, found, err := eng2.Get(binaryKey)
	if err != nil || !found {
		t.Fatalf("Get binary failed: %v, found=%v", err, found)
	}
	if !bytes.Equal(got, binaryVal) {
		t.Fatal("binary roundtrip mismatch")
	}
}

// ----------------------------------------------------------------------------
// 3. Durability & Reopen Tests
// ----------------------------------------------------------------------------

func TestEngine_Durability_Reopen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "durability.db")

	eng1, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// Insert sequence of mutations
	_ = eng1.Put([]byte("k1"), []byte("val1"))
	_ = eng1.Put([]byte("k2"), []byte("val2_initial"))
	_ = eng1.Put([]byte("k3"), []byte("val3_to_delete"))
	_ = eng1.Put([]byte("k4"), []byte{}) // empty value
	_ = eng1.Put([]byte("k2"), []byte("val2_final"))
	_ = eng1.Delete([]byte("k3"))
	_ = eng1.Delete([]byte("non_existent_key"))

	if err := eng1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Cold reopen
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer eng2.Close()

	// Verify k1
	got1, found1, _ := eng2.Get([]byte("k1"))
	if !found1 || !bytes.Equal(got1, []byte("val1")) {
		t.Errorf("k1 mismatch: found=%v, val=%q", found1, got1)
	}

	// Verify k2 (overwritten)
	got2, found2, _ := eng2.Get([]byte("k2"))
	if !found2 || !bytes.Equal(got2, []byte("val2_final")) {
		t.Errorf("k2 mismatch: found=%v, val=%q", found2, got2)
	}

	// Verify k3 (deleted)
	_, found3, _ := eng2.Get([]byte("k3"))
	if found3 {
		t.Errorf("k3 was deleted but found=true")
	}

	// Verify k4 (empty value)
	got4, found4, _ := eng2.Get([]byte("k4"))
	if !found4 || len(got4) != 0 {
		t.Errorf("k4 mismatch: found=%v, len=%d", found4, len(got4))
	}

	if eng2.KeyCount() != 3 {
		t.Errorf("expected 3 live keys, got %d", eng2.KeyCount())
	}
}

// ----------------------------------------------------------------------------
// 4. Quota and Closed State Guard Tests
// ----------------------------------------------------------------------------

func TestEngine_Quota_MaxBytesExceeded(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "quota.db")

	// Header is 32 bytes. Limit to 150 bytes.
	eng, err := engine.Open(dbPath, engine.Options{MaxBytes: 150})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	// Write 1 fits: ~47 bytes -> validEOF = ~79
	if err := eng.Put([]byte("k1"), []byte("small")); err != nil {
		t.Fatalf("first put should fit: %v", err)
	}
	savedEOF := eng.ValidEOF()

	// Write 2 exceeds 150 bytes: 100 byte payload -> frame size ~140 bytes -> total ~219 > 150
	bigVal := make([]byte, 100)
	err = eng.Put([]byte("k2"), bigVal)
	if !errors.Is(err, engine.ErrDiskFull) {
		t.Fatalf("expected ErrDiskFull, got: %v", err)
	}

	// Assert validEOF and KeyDir remain unchanged
	if eng.ValidEOF() != savedEOF {
		t.Errorf("validEOF corrupted on quota error: %d != %d", eng.ValidEOF(), savedEOF)
	}
	if eng.KeyCount() != 1 {
		t.Errorf("keyCount corrupted on quota error: %d", eng.KeyCount())
	}

	// Verify k1 still readable
	got, found, err := eng.Get([]byte("k1"))
	if err != nil || !found || !bytes.Equal(got, []byte("small")) {
		t.Errorf("k1 read failed after quota rejection: %v, found=%v", err, found)
	}
}

func TestEngine_ClosedEngine_Rejection(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "closed.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}

	if err := eng.Put([]byte("k"), []byte("v")); !errors.Is(err, engine.ErrClosed) {
		t.Errorf("expected ErrClosed for Put, got: %v", err)
	}
	if _, _, err := eng.Get([]byte("k")); !errors.Is(err, engine.ErrClosed) {
		t.Errorf("expected ErrClosed for Get, got: %v", err)
	}
	if err := eng.Delete([]byte("k")); !errors.Is(err, engine.ErrClosed) {
		t.Errorf("expected ErrClosed for Delete, got: %v", err)
	}
	if err := eng.Close(); !errors.Is(err, engine.ErrClosed) {
		t.Errorf("expected ErrClosed for duplicate Close, got: %v", err)
	}
}

// ----------------------------------------------------------------------------
// 5. Concurrency Scenarios D1-D4
// ----------------------------------------------------------------------------

func TestEngine_Concurrency_D1_DisjointWriters(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "d1.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}

	numWriters := 8
	keysPerWriter := 50
	var wg sync.WaitGroup

	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for i := 0; i < keysPerWriter; i++ {
				k := fmt.Sprintf("w%d_k%d", writerID, i)
				v := fmt.Sprintf("val_%d_%d", writerID, i)
				if err := eng.Put([]byte(k), []byte(v)); err != nil {
					t.Errorf("writer %d put failed: %v", writerID, err)
					return
				}
			}
		}(w)
	}

	wg.Wait()
	_ = eng.Close()

	// Reopen and verify all keys are present
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng2.Close()

	expectedTotal := numWriters * keysPerWriter
	if eng2.KeyCount() != expectedTotal {
		t.Fatalf("expected %d keys, got %d", expectedTotal, eng2.KeyCount())
	}

	for w := 0; w < numWriters; w++ {
		for i := 0; i < keysPerWriter; i++ {
			k := fmt.Sprintf("w%d_k%d", w, i)
			expectedVal := fmt.Sprintf("val_%d_%d", w, i)
			got, found, err := eng2.Get([]byte(k))
			if err != nil || !found || string(got) != expectedVal {
				t.Fatalf("key %s mismatch: found=%v, got=%q", k, found, string(got))
			}
		}
	}
}

func TestEngine_Concurrency_D2_SameKeyWriters(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "d2.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}

	numWriters := 8
	iterations := 50
	var wg sync.WaitGroup

	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				v := fmt.Sprintf("writer_%02d_val_%04d", writerID, i)
				if err := eng.Put([]byte("hot_key"), []byte(v)); err != nil {
					t.Errorf("writer put failed: %v", err)
					return
				}
			}
		}(w)
	}

	wg.Wait()

	got, found, err := eng.Get([]byte("hot_key"))
	if err != nil || !found {
		t.Fatalf("Get hot_key failed: %v, found=%v", err, found)
	}
	// Value must be a valid complete string, not torn or corrupted
	if len(got) != len("writer_00_val_0000") {
		t.Fatalf("torn value length detected: %d bytes, value=%q", len(got), got)
	}

	_ = eng.Close()

	// Verify survives reopen
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng2.Close()

	got2, found2, _ := eng2.Get([]byte("hot_key"))
	if !found2 || !bytes.Equal(got, got2) {
		t.Fatalf("value mismatch on reopen: %q != %q", got2, got)
	}
}

func TestEngine_Concurrency_D3_ReadersDuringWrites(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "d3.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	numKeys := 20
	// Prepopulate
	for i := 0; i < numKeys; i++ {
		_ = eng.Put([]byte(fmt.Sprintf("key_%d", i)), []byte(fmt.Sprintf("initial_%d", i)))
	}

	var wg sync.WaitGroup

	// 4 Writers
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			iter := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					idx := rand.Intn(numKeys)
					k := fmt.Sprintf("key_%d", idx)
					v := fmt.Sprintf("writer_%d_iter_%d", id, iter)
					_ = eng.Put([]byte(k), []byte(v))
					iter++
				}
			}
		}(w)
	}

	// 8 Readers
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					idx := rand.Intn(numKeys)
					k := fmt.Sprintf("key_%d", idx)
					val, found, err := eng.Get([]byte(k))
					if err != nil {
						t.Errorf("reader Get failed: %v", err)
						return
					}
					if !found {
						t.Errorf("prepopulated key %s not found", k)
						return
					}
					if len(val) == 0 {
						t.Errorf("reader observed empty value for %s", k)
						return
					}
				}
			}
		}()
	}

	wg.Wait()
}

func TestEngine_Concurrency_D4_NoTornReads(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "d4.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	payloadA := bytes.Repeat([]byte("A"), 1000)
	payloadB := bytes.Repeat([]byte("B"), 1000)

	_ = eng.Put([]byte("flip_key"), payloadA)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Writer alternating A and B
	wg.Add(1)
	go func() {
		defer wg.Done()
		flip := false
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if flip {
					_ = eng.Put([]byte("flip_key"), payloadA)
				} else {
					_ = eng.Put([]byte("flip_key"), payloadB)
				}
				flip = !flip
			}
		}
	}()

	// 8 Readers ensuring value is purely all A or all B
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					val, found, err := eng.Get([]byte("flip_key"))
					if err != nil {
						t.Errorf("Get failed: %v", err)
						return
					}
					if !found {
						t.Errorf("flip_key not found")
						return
					}
					if len(val) != 1000 {
						t.Errorf("torn read length: expected 1000, got %d", len(val))
						return
					}
					// Assert all bytes match the first byte
					b0 := val[0]
					if b0 != 'A' && b0 != 'B' {
						t.Errorf("unexpected first byte: %c", b0)
						return
					}
					for i, b := range val {
						if b != b0 {
							t.Fatalf("TORN READ DETECTED: byte %d is %c, expected %c", i, b, b0)
						}
					}
				}
			}
		}()
	}

	wg.Wait()
}
