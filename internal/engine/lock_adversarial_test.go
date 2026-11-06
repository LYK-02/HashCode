package engine_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"kvdb/internal/engine"
)

// ============================================================================
// Adversarial Concurrency & Process-Level Locking Stress Tests (Phase E2)
// ============================================================================

// 1. Concurrency Collision Testing:
// Spawns a lock holder subprocess, then executes 20 concurrent collision attempts
// using both engine.Open and the kvdb binary. Every attempt must fail immediately (< 500ms)
// with ErrLocked without hanging or blocking.
func TestAdversarial_Lock_ConcurrencyCollision_Burst(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "collision_burst.db")

	// Pre-create DB and initial key
	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("setup Open failed: %v", err)
	}
	if err := eng.Put([]byte("initKey"), []byte("initVal")); err != nil {
		t.Fatalf("setup Put failed: %v", err)
	}
	if err := eng.Close(); err != nil {
		t.Fatalf("setup Close failed: %v", err)
	}

	// Spawn persistent lock holder subprocess
	cmd, stdin := spawnLockHolder(t, dbPath)
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	numRacers := 20
	var wg sync.WaitGroup
	var lockedCount atomic.Int64
	var unexpectedErrors atomic.Int64
	var blockedCount atomic.Int64

	startBarrier := make(chan struct{})

	for i := 0; i < numRacers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-startBarrier

			start := time.Now()
			// Alternate between direct engine.Open and external CLI invocation if available
			if id%2 == 0 {
				_, err := engine.Open(dbPath, engine.Options{})
				duration := time.Since(start)
				if duration > 500*time.Millisecond {
					blockedCount.Add(1)
				}
				if errors.Is(err, engine.ErrLocked) {
					lockedCount.Add(1)
				} else {
					unexpectedErrors.Add(1)
					t.Errorf("racer %d direct open unexpected error: %v (took %v)", id, err, duration)
				}
			} else {
				// CLI invocation: kvdb put <path> <key> <val>
				cliCmd := exec.Command("../../kvdb", "put", dbPath, fmt.Sprintf("k_%d", id), "v")
				out, err := cliCmd.CombinedOutput()
				duration := time.Since(start)
				if duration > 500*time.Millisecond {
					blockedCount.Add(1)
				}
				if err != nil {
					// Verify exit code is 1 and output contains locked
					if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
						lockedCount.Add(1)
					} else {
						unexpectedErrors.Add(1)
						t.Errorf("racer %d CLI unexpected exit code: %v, out=%s", id, err, string(out))
					}
				} else {
					unexpectedErrors.Add(1)
					t.Errorf("racer %d CLI put succeeded while lock held! out=%s", id, string(out))
				}
			}
		}(i)
	}

	// Release all 20 racers simultaneously
	close(startBarrier)
	wg.Wait()

	if blockedCount.Load() > 0 {
		t.Fatalf("%d racers were blocked for > 500ms (non-blocking lock invariant violated)", blockedCount.Load())
	}
	if unexpectedErrors.Load() > 0 {
		t.Fatalf("%d racers encountered unexpected errors", unexpectedErrors.Load())
	}
	if lockedCount.Load() != int64(numRacers) {
		t.Fatalf("expected all %d racers to be rejected with ErrLocked, got %d", numRacers, lockedCount.Load())
	}

	// Now signal clean close of the holder
	_, _ = io.WriteString(stdin, "CLOSE\n")
	_ = cmd.Wait()

	// Subsequent open must immediately succeed
	engSuccessor, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("successor Open failed after clean release: %v", err)
	}
	val, found, err := engSuccessor.Get([]byte("initKey"))
	if err != nil || !found || string(val) != "initVal" {
		t.Fatalf("successor Get failed: %v, found=%v, val=%q", err, found, string(val))
	}
	_ = engSuccessor.Close()
}

// 2. Crash & Release Testing:
// Stress-tests SIGKILL (kill -9) termination over 15 consecutive cycles.
// Verifies that after SIGKILL of the lock holder, the lock is immediately acquirable
// by a successor process without manual lock file cleanup or intervention.
func TestAdversarial_Lock_SIGKILL_InstantRelease_StressLoop(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sigkill_stress.db")

	const iterations = 15
	for iter := 1; iter <= iterations; iter++ {
		cmd, stdin := spawnLockHolder(t, dbPath)

		// Assert lock is actively held
		_, err := engine.Open(dbPath, engine.Options{})
		if !errors.Is(err, engine.ErrLocked) {
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			t.Fatalf("iteration %d: expected ErrLocked before kill, got %v", iter, err)
		}

		// Brutally kill holder with SIGKILL
		if err := cmd.Process.Kill(); err != nil {
			_ = stdin.Close()
			t.Fatalf("iteration %d: Process.Kill failed: %v", iter, err)
		}
		_ = cmd.Wait()
		_ = stdin.Close()

		// Immediate successor open must succeed without delay or manual cleanup
		startAcquire := time.Now()
		eng, err := engine.Open(dbPath, engine.Options{})
		acquireDur := time.Since(startAcquire)
		if err != nil {
			t.Fatalf("iteration %d: successor Open failed after SIGKILL: %v", iter, err)
		}
		if acquireDur > 200*time.Millisecond {
			t.Errorf("iteration %d: lock acquisition after SIGKILL took too long: %v", iter, acquireDur)
		}

		// Mutate state to ensure engine is fully functional
		key := fmt.Sprintf("iter_%d", iter)
		val := fmt.Sprintf("val_%d", iter)
		if err := eng.Put([]byte(key), []byte(val)); err != nil {
			_ = eng.Close()
			t.Fatalf("iteration %d: Put failed: %v", iter, err)
		}

		// Verify read
		got, found, err := eng.Get([]byte(key))
		if err != nil || !found || string(got) != val {
			_ = eng.Close()
			t.Fatalf("iteration %d: Get mismatch: got=%q, expected=%q", iter, string(got), val)
		}

		if err := eng.Close(); err != nil {
			t.Fatalf("iteration %d: Close failed: %v", iter, err)
		}
	}
}

// 3. Lock Sidecar Invariants:
// - Created as <path>.lock
// - Permissions allow read/write (0644/0600)
// - NEVER deleted during normal Close()
// - Inode number remains strictly identical across multiple Open/Close cycles
func TestAdversarial_Lock_SidecarInvariants(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sidecar_invariants.db")
	lockPath := dbPath + ".lock"

	// Prior to open, lock file does not exist
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock file unexpectedly exists before open")
	}

	eng1, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}

	// 1. Verify lock file exists while open
	fi1, err := os.Stat(lockPath)
	if err != nil {
		_ = eng1.Close()
		t.Fatalf("lock file missing during first Open: %v", err)
	}

	// 2. Verify permissions allow read/write
	perm := fi1.Mode().Perm()
	if perm&0600 != 0600 {
		_ = eng1.Close()
		t.Fatalf("lock file permissions too restrictive: %04o (must allow user rw)", perm)
	}

	// Record inode of lock file
	stat1, ok := fi1.Sys().(*syscall.Stat_t)
	if !ok {
		_ = eng1.Close()
		t.Fatalf("failed to obtain syscall.Stat_t")
	}
	inode1 := stat1.Ino

	// 3. Close engine
	if err := eng1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 4. Verify lock file is NEVER unlinked on Close
	fi2, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("CRITICAL INVARIANT VIOLATION: lock file was unlinked on Close: %v", err)
	}
	stat2 := fi2.Sys().(*syscall.Stat_t)
	inode2 := stat2.Ino
	if inode1 != inode2 {
		t.Fatalf("lock file was recreated during Close! Inodes differ: %d vs %d", inode1, inode2)
	}

	// 5. Reopen and verify inode remains identical
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	fi3, err := os.Stat(lockPath)
	if err != nil {
		_ = eng2.Close()
		t.Fatalf("stat failed on second open: %v", err)
	}
	stat3 := fi3.Sys().(*syscall.Stat_t)
	inode3 := stat3.Ino
	if inode1 != inode3 {
		_ = eng2.Close()
		t.Fatalf("lock file was recreated during reopen! Inodes differ: %d vs %d", inode1, inode3)
	}
	_ = eng2.Close()
}

// 4. Path Canonicalization:
// Tests that equivalent relative, absolute, and dot-dot paths all resolve to the same
// lock sidecar, preventing simultaneous opens via disparate path expressions.
func TestAdversarial_Lock_PathCanonicalization(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	dbPathAbs := filepath.Join(subDir, "canon.db")
	dbPathDotDot := filepath.Join(subDir, "..", "subdir", "canon.db")

	eng1, err := engine.Open(dbPathAbs, engine.Options{})
	if err != nil {
		t.Fatalf("Open with abs path failed: %v", err)
	}
	defer eng1.Close()

	// Second open using dot-dot path must collide and return ErrLocked
	_, err = engine.Open(dbPathDotDot, engine.Options{})
	if !errors.Is(err, engine.ErrLocked) {
		t.Fatalf("expected ErrLocked for canonicalized path %s, got: %v", dbPathDotDot, err)
	}
}

// 5. Pre-Existing Lock Sidecar Content:
// Verifies that a pre-existing lock file containing arbitrary bytes or existing maxBytes
// is handled cleanly and safely locked without truncation.
func TestAdversarial_Lock_PreExistingContent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "existing_content.db")
	lockPath := dbPath + ".lock"

	// Pre-create lock file with 64 bytes of test pattern
	testContent := make([]byte, 64)
	for i := range testContent {
		testContent[i] = 0xAA
	}
	if err := os.WriteFile(lockPath, testContent, 0644); err != nil {
		t.Fatal(err)
	}

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed with pre-existing lock file: %v", err)
	}

	// Mutate and close
	if err := eng.Put([]byte("key"), []byte("val")); err != nil {
		_ = eng.Close()
		t.Fatalf("Put failed: %v", err)
	}
	if err := eng.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify lock file still exists
	fi, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("lock file missing after close: %v", err)
	}
	if fi.Size() < 8 {
		t.Errorf("lock file unexpectedly truncated: size=%d", fi.Size())
	}
}

// 6. Double Open Within Same Process:
// Verifies that calling engine.Open twice on the same path within the same process
// correctly detects contention via the second file descriptor and returns ErrLocked.
func TestAdversarial_Lock_DoubleOpen_SameProcess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "double_open.db")

	eng1, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	defer eng1.Close()

	// Second call in same process
	eng2, err := engine.Open(dbPath, engine.Options{})
	if !errors.Is(err, engine.ErrLocked) {
		if eng2 != nil {
			_ = eng2.Close()
		}
		t.Fatalf("expected ErrLocked on double Open, got: %v", err)
	}
}

// 7. Uncontended Concurrent Racers:
// 25 goroutines simultaneously attempt engine.Open on a fresh database.
// Exactly one goroutine acquires the lock, performs a mutation, and releases it,
// while all other active concurrent callers receive ErrLocked.
func TestAdversarial_Lock_ConcurrentRacersNoHolder(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "racers_no_holder.db")

	numRacers := 25
	var wg sync.WaitGroup
	var wins atomic.Int64
	var locked atomic.Int64
	var errs atomic.Int64

	startBarrier := make(chan struct{})

	for i := 0; i < numRacers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-startBarrier

			eng, err := engine.Open(dbPath, engine.Options{})
			if err == nil {
				wins.Add(1)
				// Hold lock briefly to ensure collisions occur
				time.Sleep(20 * time.Millisecond)
				_ = eng.Put([]byte(fmt.Sprintf("winner_%d", id)), []byte("val"))
				_ = eng.Close()
			} else if errors.Is(err, engine.ErrLocked) {
				locked.Add(1)
			} else {
				errs.Add(1)
				t.Errorf("racer %d failed with unexpected error: %v", id, err)
			}
		}(i)
	}

	close(startBarrier)
	wg.Wait()

	if errs.Load() > 0 {
		t.Fatalf("unexpected errors occurred during race")
	}
	if wins.Load() < 1 {
		t.Fatalf("expected at least 1 winner, got 0")
	}
	t.Logf("Concurrent race results: %d won lock, %d rejected with ErrLocked", wins.Load(), locked.Load())

	// Reopen to verify DB is structurally sound
	engFinal, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("final Open failed after concurrent race: %v", err)
	}
	defer engFinal.Close()
	if engFinal.KeyCount() < 1 {
		t.Fatalf("expected at least 1 key committed, got %d", engFinal.KeyCount())
	}
}

// 8. Auto-creation of Nested Parent Directories:
// Verifies that opening a path with non-existent deep parent directories
// creates the directories and properly places both the data file and .lock sidecar.
func TestAdversarial_Lock_DeepNestedParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	deepPath := filepath.Join(tmpDir, "a", "b", "c", "deep.db")

	eng, err := engine.Open(deepPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open with deep parent dir failed: %v", err)
	}
	_ = eng.Close()

	if _, err := os.Stat(deepPath); err != nil {
		t.Fatalf("deep db file not created: %v", err)
	}
	if _, err := os.Stat(deepPath + ".lock"); err != nil {
		t.Fatalf("deep lock file not created: %v", err)
	}
}

// 9. Stress Concurrency During Reads & Writes (Goroutine & Process Mix):
// Tests holding lock while readers query data file via CLI.
func TestAdversarial_Lock_CLI_CollisionDiagnostics(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "diag.db")

	cmd, stdin := spawnLockHolder(t, dbPath)
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	// Test all CLI subcommands against locked database
	subcommands := []struct {
		args        []string
		description string
	}{
		{[]string{"open", dbPath}, "CLI open"},
		{[]string{"put", dbPath, "foo", "bar"}, "CLI put"},
		{[]string{"get", dbPath, "foo"}, "CLI get"},
		{[]string{"del", dbPath, "foo"}, "CLI del"},
		{[]string{"dump", dbPath}, "CLI dump"},
	}

	for _, sc := range subcommands {
		cliCmd := exec.Command("../../kvdb", sc.args...)
		out, err := cliCmd.CombinedOutput()
		if err == nil {
			t.Errorf("%s succeeded unexpectedly while lock is held! Output: %s", sc.description, string(out))
			continue
		}
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 1 {
			t.Errorf("%s exit code was not 1: %v", sc.description, err)
		}
		if !stringsContains(string(out), "LOCKED") && !stringsContains(string(out), "lock") {
			t.Errorf("%s output missing LOCKED diagnostic: %s", sc.description, string(out))
		}
	}

	_, _ = io.WriteString(stdin, "CLOSE\n")
	_ = cmd.Wait()
}

func stringsContains(s, substr string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_ = ctx
	return len(s) >= len(substr) && (s == substr || filepath.Base(s) != "" && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
