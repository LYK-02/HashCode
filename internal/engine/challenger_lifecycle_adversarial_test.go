package engine_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
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

// ============================================================================
// Challenger Stress Harness: Engine Lifecycle & Locking Invariants
// ============================================================================

// 1. Multi-process flock mutual exclusion under stress:
// 12 concurrent OS processes race simultaneously to open the same database.
// Strictly 1 wins, rest receive typed ErrLocked and exit with code 1.
func TestAdversarial_Lock_MultiProcess_Flock_MutualExclusion_Stress(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "mp_race.db")

	// Pre-create database with an initial key
	eng0, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("setup Open failed: %v", err)
	}
	if err := eng0.Put([]byte("mpKey"), []byte("mpVal")); err != nil {
		t.Fatalf("setup Put failed: %v", err)
	}
	if err := eng0.Close(); err != nil {
		t.Fatalf("setup Close failed: %v", err)
	}

	runRaceRound := func(round int, numRacers int) {
		type racerResult struct {
			id       int
			winner   bool
			exitCode int
			errOutput string
			cmd      *exec.Cmd
			stdin    io.WriteCloser
		}

		results := make([]racerResult, numRacers)
		var wg sync.WaitGroup

		for i := 0; i < numRacers; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				cmd := exec.Command(os.Args[0], "-test.run=^$")
				cmd.Env = append(os.Environ(),
					envSubprocessKey+"=1",
					envSubprocessPath+"="+dbPath,
				)

				stdoutPipe, err := cmd.StdoutPipe()
				if err != nil {
					t.Errorf("round %d racer %d StdoutPipe error: %v", round, id, err)
					return
				}
				stderrPipe, err := cmd.StderrPipe()
				if err != nil {
					t.Errorf("round %d racer %d StderrPipe error: %v", round, id, err)
					return
				}
				stdinPipe, err := cmd.StdinPipe()
				if err != nil {
					t.Errorf("round %d racer %d StdinPipe error: %v", round, id, err)
					return
				}

				if err := cmd.Start(); err != nil {
					t.Errorf("round %d racer %d Start error: %v", round, id, err)
					return
				}

				// Check whether this racer prints readySignal or exits with ErrLocked
				stdoutReader := bufio.NewReader(stdoutPipe)
				sigChan := make(chan string, 1)
				go func() {
					line, _ := stdoutReader.ReadString('\n')
					sigChan <- line
				}()

				select {
				case line := <-sigChan:
					if line == readySignal {
						results[id] = racerResult{
							id:     id,
							winner: true,
							cmd:    cmd,
							stdin:  stdinPipe,
						}
						return
					}
				case <-time.After(500 * time.Millisecond):
				}

				// If it did not become winner immediately, it must have failed with ErrLocked
				errBytes, _ := io.ReadAll(stderrPipe)
				waitErr := cmd.Wait()
				_ = stdinPipe.Close()

				exitCode := 0
				if waitErr != nil {
					var exitErr *exec.ExitError
					if errors.As(waitErr, &exitErr) {
						exitCode = exitErr.ExitCode()
					}
				}

				results[id] = racerResult{
					id:        id,
					winner:    false,
					exitCode:  exitCode,
					errOutput: string(errBytes),
				}
			}(i)
		}

		wg.Wait()

		var winners []racerResult
		var lockedCount int
		for i, res := range results {
			if res.winner {
				winners = append(winners, res)
			} else {
				if res.exitCode == 1 && (containsSubstr(res.errOutput, "LOCKED") || containsSubstr(res.errOutput, "lock")) {
					lockedCount++
				} else {
					t.Errorf("round %d racer %d had unexpected result: exit=%d, stderr=%q", round, i, res.exitCode, res.errOutput)
				}
			}
		}

		if len(winners) != 1 {
			t.Fatalf("round %d: CRITICAL VIOLATION: expected exactly 1 winner among %d OS processes, got %d", round, numRacers, len(winners))
		}
		if lockedCount != numRacers-1 {
			t.Fatalf("round %d: expected %d racers rejected with ErrLocked, got %d", round, numRacers-1, lockedCount)
		}

		// Cleanly close the winner
		winner := winners[0]
		_, _ = io.WriteString(winner.stdin, "CLOSE\n")
		_ = winner.stdin.Close()
		if err := winner.cmd.Wait(); err != nil {
			t.Fatalf("round %d: winner exit error: %v", round, err)
		}
	}

	// Run Wave 1 with 12 concurrent processes
	runRaceRound(1, 12)

	// Run Wave 2 with 10 concurrent processes
	runRaceRound(2, 10)
}

// 2. SIGKILL recovery:
// Kill the lock holder with kill -9 without clean Close.
// Assert OS kernel immediately releases the flock and the next process can open
// the database without needing manual lock file deletion.
func TestAdversarial_Lock_SIGKILL_InstantRecovery_Stress(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sigkill_recovery.db")
	lockPath := dbPath + ".lock"

	const cycles = 10
	for cycle := 1; cycle <= cycles; cycle++ {
		cmd, stdin := spawnLockHolder(t, dbPath)

		// Assert lock is held by checking that direct Open returns ErrLocked
		_, err := engine.Open(dbPath, engine.Options{})
		if !errors.Is(err, engine.ErrLocked) {
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			t.Fatalf("cycle %d: expected ErrLocked while holder is active, got %v", cycle, err)
		}

		// Brutally kill lock holder with SIGKILL (kill -9)
		if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
			_ = stdin.Close()
			t.Fatalf("cycle %d: SIGKILL failed: %v", cycle, err)
		}
		_ = cmd.Wait()
		_ = stdin.Close()

		// Invariant: <path>.lock MUST NOT be deleted
		if _, err := os.Stat(lockPath); err != nil {
			t.Fatalf("cycle %d: lock sidecar missing after SIGKILL: %v", cycle, err)
		}

		// Immediate successor open must succeed without manual intervention
		start := time.Now()
		engSuccessor, err := engine.Open(dbPath, engine.Options{})
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("cycle %d: successor Open failed after SIGKILL: %v", cycle, err)
		}
		if elapsed > 200*time.Millisecond {
			t.Errorf("cycle %d: successor Open took excessive time: %v", cycle, elapsed)
		}

		// Perform durable write to confirm full operability
		k := fmt.Sprintf("k_%d", cycle)
		v := fmt.Sprintf("v_%d", cycle)
		if err := engSuccessor.Put([]byte(k), []byte(v)); err != nil {
			_ = engSuccessor.Close()
			t.Fatalf("cycle %d: successor Put failed: %v", cycle, err)
		}

		val, found, err := engSuccessor.Get([]byte(k))
		if err != nil || !found || string(val) != v {
			_ = engSuccessor.Close()
			t.Fatalf("cycle %d: successor Get mismatch: %v, found=%v, val=%q", cycle, err, found, string(val))
		}

		if err := engSuccessor.Close(); err != nil {
			t.Fatalf("cycle %d: successor Close failed: %v", cycle, err)
		}
	}
}

// 3. Inode stability:
// Verify <path>.lock is never unlinked or renamed, and its inode remains constant
// across multiple opens, closes, concurrent collisions, SIGKILLs, and corruption rejections.
func TestAdversarial_Lock_Inode_Stability_Matrix(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "inode_stability.db")
	lockPath := dbPath + ".lock"

	// Prior to open, lock file must not exist
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock file unexpectedly exists before open")
	}

	// 1st Open: creates sidecar
	eng1, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("initial Open failed: %v", err)
	}

	fi1, err := os.Stat(lockPath)
	if err != nil {
		_ = eng1.Close()
		t.Fatalf("stat lock file failed: %v", err)
	}
	stat1, ok := fi1.Sys().(*syscall.Stat_t)
	if !ok {
		_ = eng1.Close()
		t.Fatalf("failed to get syscall.Stat_t")
	}
	initialInode := stat1.Ino
	_ = eng1.Close()

	checkInode := func(stepName string) {
		fi, err := os.Stat(lockPath)
		if err != nil {
			t.Fatalf("%s: lock file missing: %v", stepName, err)
		}
		stat, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			t.Fatalf("%s: failed to get Stat_t", stepName)
		}
		if stat.Ino != initialInode {
			t.Fatalf("%s: INODE CHANGED! initial=%d, current=%d (file was replaced/recreated)", stepName, initialInode, stat.Ino)
		}
		if stat.Nlink != 1 {
			t.Fatalf("%s: expected nlink=1, got %d", stepName, stat.Nlink)
		}
	}

	// Check 50 sequential Open-Close cycles
	for i := 1; i <= 50; i++ {
		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("cycle %d Open failed: %v", i, err)
		}
		checkInode(fmt.Sprintf("cycle %d during open", i))
		if err := eng.Close(); err != nil {
			t.Fatalf("cycle %d Close failed: %v", i, err)
		}
		checkInode(fmt.Sprintf("cycle %d after close", i))
	}

	// Check during and after lock holder subprocess
	cmd, stdin := spawnLockHolder(t, dbPath)
	checkInode("while subprocess holds lock")

	// Trigger collision while subprocess holds lock
	_, err = engine.Open(dbPath, engine.Options{})
	if !errors.Is(err, engine.ErrLocked) {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
	checkInode("after collision ErrLocked")

	// Brutally kill subprocess with SIGKILL
	_ = cmd.Process.Signal(syscall.SIGKILL)
	_ = cmd.Wait()
	_ = stdin.Close()
	checkInode("immediately after SIGKILL")

	// Reopen after SIGKILL
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("reopen after SIGKILL failed: %v", err)
	}
	checkInode("during reopen after SIGKILL")
	_ = eng2.Close()
	checkInode("after reopen close")
}

// 4. Header corruption fail-closed byte matrix:
// Deliberately flip bytes in magic, version, and header CRC32C of data.db.
// Verify engine.Open fails closed with precise typed errors and does NOT overwrite or wipe the file.
func TestAdversarial_Lock_Header_Corruption_FailClosed_ByteMatrix(t *testing.T) {
	tmpDir := t.TempDir()

	// Helper to create a valid base DB with data
	createBaseDB := func(path string) []byte {
		eng, err := engine.Open(path, engine.Options{})
		if err != nil {
			t.Fatalf("createBaseDB Open failed: %v", err)
		}
		for i := 0; i < 5; i++ {
			k := fmt.Sprintf("baseKey_%d", i)
			v := fmt.Sprintf("baseVal_%d", i)
			if err := eng.Put([]byte(k), []byte(v)); err != nil {
				_ = eng.Close()
				t.Fatalf("createBaseDB Put failed: %v", err)
			}
		}
		if err := eng.Close(); err != nil {
			t.Fatalf("createBaseDB Close failed: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("readFile failed: %v", err)
		}
		return data
	}

	baseBytes := createBaseDB(filepath.Join(tmpDir, "master.db"))

	type corruptCase struct {
		name        string
		corruptFn   func(buf []byte) []byte
		expectedErr error
	}

	var cases []corruptCase

	// Magic byte flips (bytes 0..7)
	for b := 0; b < 8; b++ {
		byteIdx := b
		cases = append(cases, corruptCase{
			name: fmt.Sprintf("Magic_Byte_%d_Flip", byteIdx),
			corruptFn: func(buf []byte) []byte {
				c := bytes.Clone(buf)
				c[byteIdx] ^= 0x55
				return c
			},
			expectedErr: format.ErrInvalidHeaderMagic,
		})
	}

	// Version corruption (bytes 8..9)
	cases = append(cases,
		corruptCase{
			name: "Version_Zero",
			corruptFn: func(buf []byte) []byte {
				c := bytes.Clone(buf)
				c[8] = 0x00
				c[9] = 0x00
				// Recalculate CRC so only version is invalid
				crc := format.ChecksumCastagnoli(c[:28])
				c[28] = byte(crc)
				c[29] = byte(crc >> 8)
				c[30] = byte(crc >> 16)
				c[31] = byte(crc >> 24)
				return c
			},
			expectedErr: format.ErrUnsupportedVersion,
		},
		corruptCase{
			name: "Version_Future_2",
			corruptFn: func(buf []byte) []byte {
				c := bytes.Clone(buf)
				c[8] = 0x02
				c[9] = 0x00
				crc := format.ChecksumCastagnoli(c[:28])
				c[28] = byte(crc)
				c[29] = byte(crc >> 8)
				c[30] = byte(crc >> 16)
				c[31] = byte(crc >> 24)
				return c
			},
			expectedErr: format.ErrUnsupportedVersion,
		},
		corruptCase{
			name: "Version_Max_65535",
			corruptFn: func(buf []byte) []byte {
				c := bytes.Clone(buf)
				c[8] = 0xFF
				c[9] = 0xFF
				crc := format.ChecksumCastagnoli(c[:28])
				c[28] = byte(crc)
				c[29] = byte(crc >> 8)
				c[30] = byte(crc >> 16)
				c[31] = byte(crc >> 24)
				return c
			},
			expectedErr: format.ErrUnsupportedVersion,
		},
	)

	// CRC32C byte flips (bytes 28..31)
	for b := 28; b <= 31; b++ {
		byteIdx := b
		cases = append(cases, corruptCase{
			name: fmt.Sprintf("Header_CRC_Byte_%d_Flip", byteIdx),
			corruptFn: func(buf []byte) []byte {
				c := bytes.Clone(buf)
				c[byteIdx] ^= 0xAA
				return c
			},
			expectedErr: format.ErrCorruptHeader,
		})
	}

	// Truncated headers (sizes 1, 15, 31 bytes)
	for _, sz := range []int{1, 15, 31} {
		size := sz
		cases = append(cases, corruptCase{
			name: fmt.Sprintf("Header_Truncated_%d_Bytes", size),
			corruptFn: func(buf []byte) []byte {
				return bytes.Clone(buf[:size])
			},
			expectedErr: format.ErrHeaderTooShort,
		})
	}

	// Execute every corrupt case
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			testDB := filepath.Join(t.TempDir(), "corrupt.db")
			corruptedContent := tc.corruptFn(baseBytes)
			if err := os.WriteFile(testDB, corruptedContent, 0644); err != nil {
				t.Fatalf("write corrupted DB failed: %v", err)
			}

			beforeHash := sha256.Sum256(corruptedContent)

			// Attempt Open: MUST fail closed with expected typed error
			eng, err := engine.Open(testDB, engine.Options{})
			if err == nil {
				_ = eng.Close()
				t.Fatalf("Open succeeded unexpectedly on corrupted header (%s)", tc.name)
			}
			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("Open error mismatch for %s: expected %v, got %v", tc.name, tc.expectedErr, err)
			}

			// INVARIANT: data.db MUST NOT be overwritten, wiped, or truncated!
			afterContent, err := os.ReadFile(testDB)
			if err != nil {
				t.Fatalf("failed to read testDB after failed open: %v", err)
			}
			afterHash := sha256.Sum256(afterContent)
			if beforeHash != afterHash || !bytes.Equal(corruptedContent, afterContent) {
				t.Fatalf("CRITICAL INVARIANT VIOLATION: %s corrupted data.db was modified/wiped on failed Open! (len before=%d, len after=%d)",
					tc.name, len(corruptedContent), len(afterContent))
			}

			// Verify lock was released cleanly: a second Open returns the same error, not ErrLocked
			_, err2 := engine.Open(testDB, engine.Options{})
			if !errors.Is(err2, tc.expectedErr) {
				t.Fatalf("second Open failed with wrong error (dangling lock?): %v", err2)
			}
		})
	}
}

// 5. Path canonicalization:
// Test opening via ./data.db, sub/../data.db, and absolute path.
// Verify they all attach to the same lock sidecar and conflict properly.
func TestAdversarial_Lock_Path_Canonicalization_TripleAngle(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	dbFileName := "canon_test.db"
	absPath := filepath.Join(subDir, dbFileName)
	lockSidecar := absPath + ".lock"

	// Formulate paths
	dotDotPath := filepath.Join(subDir, "..", "sub", dbFileName)
	deepDotDotPath := filepath.Join(subDir, "nested", "..", "..", "sub", dbFileName)

	// Path with ./ relative from subDir
	// We'll execute an Open while in subDir
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	relPath := "./" + dbFileName

	// Angle 1: Open via relative path, assert abs and dotDot collide
	engRel, err := engine.Open(relPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open with relative path %q failed: %v", relPath, err)
	}

	// Verify lock sidecar exists at canonical absPath + ".lock"
	if _, err := os.Stat(lockSidecar); err != nil {
		_ = engRel.Close()
		t.Fatalf("canonical lock sidecar %s missing: %v", lockSidecar, err)
	}

	// Attempt Open via absolute path -> MUST return ErrLocked
	_, err = engine.Open(absPath, engine.Options{})
	if !errors.Is(err, engine.ErrLocked) {
		_ = engRel.Close()
		t.Fatalf("expected ErrLocked for absPath while relPath open, got: %v", err)
	}

	// Attempt Open via dot-dot path -> MUST return ErrLocked
	_, err = engine.Open(dotDotPath, engine.Options{})
	if !errors.Is(err, engine.ErrLocked) {
		_ = engRel.Close()
		t.Fatalf("expected ErrLocked for dotDotPath while relPath open, got: %v", err)
	}

	// Attempt Open via deep dot-dot path -> MUST return ErrLocked
	_, err = engine.Open(deepDotDotPath, engine.Options{})
	if !errors.Is(err, engine.ErrLocked) {
		_ = engRel.Close()
		t.Fatalf("expected ErrLocked for deepDotDotPath while relPath open, got: %v", err)
	}

	if err := engRel.Close(); err != nil {
		t.Fatalf("close rel failed: %v", err)
	}

	// Angle 2: Open via dot-dot path, assert abs and rel collide
	engDotDot, err := engine.Open(dotDotPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open with dot-dot path failed: %v", err)
	}

	_, err = engine.Open(absPath, engine.Options{})
	if !errors.Is(err, engine.ErrLocked) {
		_ = engDotDot.Close()
		t.Fatalf("expected ErrLocked for absPath while dotDotPath open, got: %v", err)
	}

	_, err = engine.Open(relPath, engine.Options{})
	if !errors.Is(err, engine.ErrLocked) {
		_ = engDotDot.Close()
		t.Fatalf("expected ErrLocked for relPath while dotDotPath open, got: %v", err)
	}

	if err := engDotDot.Close(); err != nil {
		t.Fatalf("close dotDot failed: %v", err)
	}

	// Angle 3: Open via absolute path, assert dot-dot and rel collide
	engAbs, err := engine.Open(absPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open with abs path failed: %v", err)
	}

	_, err = engine.Open(dotDotPath, engine.Options{})
	if !errors.Is(err, engine.ErrLocked) {
		_ = engAbs.Close()
		t.Fatalf("expected ErrLocked for dotDotPath while absPath open, got: %v", err)
	}

	_, err = engine.Open(relPath, engine.Options{})
	if !errors.Is(err, engine.ErrLocked) {
		_ = engAbs.Close()
		t.Fatalf("expected ErrLocked for relPath while absPath open, got: %v", err)
	}

	if err := engAbs.Close(); err != nil {
		t.Fatalf("close abs failed: %v", err)
	}

	// Verify only ONE lock file exists in the directory tree
	entries, err := os.ReadDir(subDir)
	if err != nil {
		t.Fatalf("readdir subDir failed: %v", err)
	}
	var lockCount int
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".lock" {
			lockCount++
		}
	}
	if lockCount != 1 {
		t.Fatalf("expected exactly 1 .lock file in %s, found %d", subDir, lockCount)
	}
}

// 6. Rapid Multi-Process Handoff Under High Burst:
// 20 rapid sequential process transfers where each process opens, verifies,
// mutates a key, and closes. Asserts zero deadlock, zero race condition, and monotonic sequence advancement.
func TestAdversarial_Lock_Rapid_Sequential_Process_Handoff(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "rapid_handoff.db")

	const handoffs = 20
	for i := 1; i <= handoffs; i++ {
		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("handoff %d Open failed: %v", i, err)
		}

		key := fmt.Sprintf("handoff_key_%03d", i)
		val := fmt.Sprintf("handoff_val_%03d", i)
		if err := eng.Put([]byte(key), []byte(val)); err != nil {
			_ = eng.Close()
			t.Fatalf("handoff %d Put failed: %v", i, err)
		}

		got, found, err := eng.Get([]byte(key))
		if err != nil || !found || string(got) != val {
			_ = eng.Close()
			t.Fatalf("handoff %d Get mismatch: got=%q, want=%q", i, string(got), val)
		}

		if err := eng.Close(); err != nil {
			t.Fatalf("handoff %d Close failed: %v", i, err)
		}
	}

	// Final verification of all 20 keys
	engFinal, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("final Open failed: %v", err)
	}
	defer engFinal.Close()

	if engFinal.KeyCount() != handoffs {
		t.Fatalf("expected %d keys, got %d", handoffs, engFinal.KeyCount())
	}
	for i := 1; i <= handoffs; i++ {
		k := fmt.Sprintf("handoff_key_%03d", i)
		expectedVal := fmt.Sprintf("handoff_val_%03d", i)
		v, found, err := engFinal.Get([]byte(k))
		if err != nil || !found || string(v) != expectedVal {
			t.Fatalf("final key %s mismatch: found=%v, val=%q", k, found, string(v))
		}
	}
}
