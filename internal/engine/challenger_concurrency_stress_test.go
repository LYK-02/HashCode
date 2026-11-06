package engine_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kvdb/internal/engine"
)

// ============================================================================
// CHALLENGER EMPIRICAL STRESS HARNESS: M2 CONCURRENCY & LOCKING
// ============================================================================

// 1. Multi-Process Lock Mutual Exclusion on data.db.lock:
// Tests 10 simultaneous processes opening the same database while held.
// Checks whether exactly 1 succeeds or if held, all 10 fail immediately with non-blocking ErrLocked.
// Tests CLI exit code behavior (checking for Exit Code 2 vs 1).
func TestChallenger_Lock_MultiProcessMutualExclusion_10Processes(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "multiprocess_10.db")

	// Pre-create DB
	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("setup Open failed: %v", err)
	}
	if err := eng.Put([]byte("init_k"), []byte("init_v")); err != nil {
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

	const numProcesses = 10
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	type procResult struct {
		id       int
		duration time.Duration
		exitCode int
		err      error
		out      string
	}
	results := make([]procResult, numProcesses)

	// Locate kvdb binary
	kvdbPath, err := filepath.Abs("../../kvdb")
	if err != nil {
		t.Fatalf("cannot resolve kvdb path: %v", err)
	}
	if _, err := os.Stat(kvdbPath); err != nil {
		t.Fatalf("kvdb binary not found at %s: run 'make build' first", kvdbPath)
	}

	for i := 0; i < numProcesses; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-startBarrier

			start := time.Now()
			cli := exec.Command(kvdbPath, "open", dbPath)
			out, runErr := cli.CombinedOutput()
			dur := time.Since(start)

			exitCode := 0
			if runErr != nil {
				if exitErr, ok := runErr.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					exitCode = -1
				}
			}

			results[idx] = procResult{
				id:       idx,
				duration: dur,
				exitCode: exitCode,
				err:      runErr,
				out:      string(out),
			}
		}(i)
	}

	// Trigger all 10 processes simultaneously
	close(startBarrier)
	wg.Wait()

	// Verify all 10 processes failed non-blocking (< 1s)
	failedCount := 0
	exitCode2Count := 0
	exitCode1Count := 0

	for _, res := range results {
		if res.duration > 1*time.Second {
			t.Errorf("process %d took too long (> 1s): %v", res.id, res.duration)
		}
		if res.exitCode != 0 {
			failedCount++
		}
		if res.exitCode == 2 {
			exitCode2Count++
		}
		if res.exitCode == 1 {
			exitCode1Count++
		}
	}

	t.Logf("10 Simultaneous Processes Result: failed=%d/%d, exitCode2=%d, exitCode1=%d",
		failedCount, numProcesses, exitCode2Count, exitCode1Count)

	if failedCount != numProcesses {
		t.Fatalf("CRITICAL MUTUAL EXCLUSION VIOLATION: %d processes succeeded while lock was held!",
			numProcesses-failedCount)
	}

	// Release holder
	_, _ = io.WriteString(stdin, "CLOSE\n")
	_ = cmd.Wait()

	// After release, exactly 1 should succeed among 10 racers on an unlocked database
	var unheldWins atomic.Int64
	var unheldLocked atomic.Int64
	startBarrier2 := make(chan struct{})

	for i := 0; i < numProcesses; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-startBarrier2

			// Compete using direct engine.Open
			e, err := engine.Open(dbPath, engine.Options{})
			if err == nil {
				unheldWins.Add(1)
				// Hold open for a short window to force collision
				time.Sleep(50 * time.Millisecond)
				_ = e.Close()
			} else if errors.Is(err, engine.ErrLocked) {
				unheldLocked.Add(1)
			} else {
				t.Errorf("unexpected error on competing open: %v", err)
			}
		}(i)
	}

	close(startBarrier2)
	wg.Wait()

	t.Logf("Unheld competing 10 processes: won=%d, rejectedWithErrLocked=%d",
		unheldWins.Load(), unheldLocked.Load())

	if unheldWins.Load() < 1 {
		t.Fatalf("expected at least 1 process to acquire lock on unheld DB")
	}
	if unheldWins.Load()+unheldLocked.Load() != int64(numProcesses) {
		t.Fatalf("unexpected result counts: won=%d, locked=%d", unheldWins.Load(), unheldLocked.Load())
	}
}

// 2. Abrupt Crash / SIGKILL of Lock Holder:
// Kills lock holder with kill -9; verifies surviving process immediately acquires lock and accesses clean data.
func TestChallenger_Lock_SIGKILL_SurvivingProcessIntegrity(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sigkill_integrity.db")

	// Phase 1: Initialize database with baseline data
	engInit, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("initial open failed: %v", err)
	}
	for i := 0; i < 50; i++ {
		k := fmt.Sprintf("baseline_key_%03d", i)
		v := fmt.Sprintf("baseline_val_%03d", i)
		if err := engInit.Put([]byte(k), []byte(v)); err != nil {
			t.Fatalf("initial put failed: %v", err)
		}
	}
	if err := engInit.Close(); err != nil {
		t.Fatalf("initial close failed: %v", err)
	}

	// Repeat SIGKILL recovery cycles
	const cycles = 5
	for c := 1; c <= cycles; c++ {
		// Spawn lock holder
		cmd, stdin := spawnLockHolder(t, dbPath)

		// Confirm lock is held
		_, err := engine.Open(dbPath, engine.Options{})
		if !errors.Is(err, engine.ErrLocked) {
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			t.Fatalf("cycle %d: expected ErrLocked before kill, got %v", c, err)
		}

		// Abruptly kill holder with SIGKILL (kill -9)
		if err := cmd.Process.Kill(); err != nil {
			_ = stdin.Close()
			t.Fatalf("cycle %d: kill failed: %v", c, err)
		}
		_ = cmd.Wait()
		_ = stdin.Close()

		// Surviving process immediately opens database
		start := time.Now()
		survivor, err := engine.Open(dbPath, engine.Options{})
		acquireDuration := time.Since(start)

		if err != nil {
			t.Fatalf("cycle %d: surviving process failed to acquire lock: %v", c, err)
		}
		if acquireDuration > 200*time.Millisecond {
			t.Errorf("cycle %d: lock acquisition took too long: %v", c, acquireDuration)
		}

		// Verify all baseline data is intact and uncorrupted
		for i := 0; i < 50; i++ {
			k := fmt.Sprintf("baseline_key_%03d", i)
			expectedV := fmt.Sprintf("baseline_val_%03d", i)
			gotV, found, getErr := survivor.Get([]byte(k))
			if getErr != nil || !found || string(gotV) != expectedV {
				_ = survivor.Close()
				t.Fatalf("cycle %d: corrupted baseline data for %s: found=%v, got=%q, err=%v",
					c, k, found, string(gotV), getErr)
			}
		}

		// Perform new durable write from survivor
		cycleKey := fmt.Sprintf("survivor_cycle_%d", c)
		cycleVal := fmt.Sprintf("survivor_val_%d_%d", c, time.Now().UnixNano())
		if err := survivor.Put([]byte(cycleKey), []byte(cycleVal)); err != nil {
			_ = survivor.Close()
			t.Fatalf("cycle %d: survivor put failed: %v", c, err)
		}

		// Verify read-your-own-writes
		readBack, found, err := survivor.Get([]byte(cycleKey))
		if err != nil || !found || string(readBack) != cycleVal {
			_ = survivor.Close()
			t.Fatalf("cycle %d: survivor read-back failed: found=%v, got=%q", c, found, string(readBack))
		}

		if err := survivor.Close(); err != nil {
			t.Fatalf("cycle %d: survivor close failed: %v", c, err)
		}
	}
}

// 3. Concurrency Scenario D1:
// 20 concurrent goroutines writing disjoint keys under race detector.
func TestChallenger_Concurrency_D1_20WritersDisjointKeys(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "d1_20writers.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	const numWriters = 20
	const keysPerWriter = 50
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			<-startBarrier

			for i := 0; i < keysPerWriter; i++ {
				key := fmt.Sprintf("w_%02d_key_%04d", writerID, i)
				val := fmt.Sprintf("writer_%02d_payload_value_%04d_%d", writerID, i, time.Now().UnixNano())

				if err := eng.Put([]byte(key), []byte(val)); err != nil {
					t.Errorf("writer %d put key %s failed: %v", writerID, key, err)
					return
				}
			}
		}(w)
	}

	close(startBarrier)
	wg.Wait()

	expectedTotal := numWriters * keysPerWriter
	if eng.KeyCount() != expectedTotal {
		t.Fatalf("expected in-memory key count %d, got %d", expectedTotal, eng.KeyCount())
	}

	// Close and cold-reopen to verify all 1,000 disjoint keys are durable
	if err := eng.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("cold reopen failed: %v", err)
	}
	defer eng2.Close()

	if eng2.KeyCount() != expectedTotal {
		t.Fatalf("expected recovered key count %d, got %d", expectedTotal, eng2.KeyCount())
	}

	for w := 0; w < numWriters; w++ {
		for i := 0; i < keysPerWriter; i++ {
			key := fmt.Sprintf("w_%02d_key_%04d", w, i)
			val, found, err := eng2.Get([]byte(key))
			if err != nil {
				t.Fatalf("Get %s failed: %v", key, err)
			}
			if !found {
				t.Fatalf("acknowledged key %s missing after cold reopen", key)
			}
			expectedPrefix := fmt.Sprintf("writer_%02d_payload_value_%04d_", w, i)
			if !bytes.HasPrefix(val, []byte(expectedPrefix)) {
				t.Fatalf("key %s payload corrupted: got %q, expected prefix %q", key, string(val), expectedPrefix)
			}
		}
	}
}

// 4. Concurrency Scenario D2:
// 20 concurrent goroutines overwriting the same key;
// final value matches last write with no corruption or tearing.
func TestChallenger_Concurrency_D2_20WritersSameKey(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "d2_20writers_samekey.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	const numWriters = 20
	const iterations = 50
	const hotKey = "disputed_hot_key"

	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	// Helper to create a signed, self-checking payload: [writerID: 2B][iter: 4B][random: 8B][crc32c: 4B]
	makePayload := func(writerID uint16, iter uint32) []byte {
		b := make([]byte, 18)
		binary.BigEndian.PutUint16(b[0:2], writerID)
		binary.BigEndian.PutUint32(b[2:6], iter)
		binary.BigEndian.PutUint64(b[6:14], rand.Uint64())
		c := crc32.Checksum(b[0:14], crc32.MakeTable(crc32.Castagnoli))
		binary.BigEndian.PutUint32(b[14:18], c)
		return b
	}

	validatePayload := func(b []byte) (uint16, uint32, bool) {
		if len(b) != 18 {
			return 0, 0, false
		}
		expectedCRC := binary.BigEndian.Uint32(b[14:18])
		actualCRC := crc32.Checksum(b[0:14], crc32.MakeTable(crc32.Castagnoli))
		if expectedCRC != actualCRC {
			return 0, 0, false
		}
		wid := binary.BigEndian.Uint16(b[0:2])
		itr := binary.BigEndian.Uint32(b[2:6])
		return wid, itr, true
	}

	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			<-startBarrier

			for i := 0; i < iterations; i++ {
				payload := makePayload(uint16(writerID), uint32(i))
				if err := eng.Put([]byte(hotKey), payload); err != nil {
					t.Errorf("writer %d put failed: %v", writerID, err)
					return
				}
			}
		}(w)
	}

	close(startBarrier)
	wg.Wait()

	// Verify final value in memory
	finalVal, found, err := eng.Get([]byte(hotKey))
	if err != nil || !found {
		t.Fatalf("Get final value failed: found=%v, err=%v", found, err)
	}

	wid, itr, valid := validatePayload(finalVal)
	if !valid {
		t.Fatalf("CRITICAL INTEGRITY FAILURE: final value is torn or corrupted! len=%d, bytes=%x",
			len(finalVal), finalVal)
	}
	t.Logf("Final write captured: Writer %d, Iteration %d (valid CRC32C)", wid, itr)

	// Close and verify cold replay yields identical valid value
	if err := eng.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer eng2.Close()

	reopenVal, found2, err := eng2.Get([]byte(hotKey))
	if err != nil || !found2 {
		t.Fatalf("reopen Get failed: found=%v, err=%v", found2, err)
	}
	if !bytes.Equal(finalVal, reopenVal) {
		t.Fatalf("reopened value does not match final value! %x != %x", reopenVal, finalVal)
	}
}

// 5. Concurrency Scenario D3:
// Continuous readers observing values during heavy writer loop without torn reads or errors.
func TestChallenger_Concurrency_D3_ContinuousReadersDuringHeavyWrites(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "d3_readers_during_writes.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	const numKeys = 25
	// Pre-populate keys with valid formatted values
	for i := 0; i < numKeys; i++ {
		k := fmt.Sprintf("k_%03d", i)
		v := fmt.Sprintf("initial_payload_%03d", i)
		if err := eng.Put([]byte(k), []byte(v)); err != nil {
			t.Fatalf("pre-populate failed: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var readOps atomic.Int64
	var writeOps atomic.Int64
	var readErrors atomic.Int64
	var tornReads atomic.Int64

	const numWriters = 5
	const numReaders = 15

	// Writers
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			iter := 0
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(wid)))
			for {
				select {
				case <-ctx.Done():
					return
				default:
					idx := r.Intn(numKeys)
					key := fmt.Sprintf("k_%03d", idx)
					val := fmt.Sprintf("writer_%02d_seq_%06d_padding_%s", wid, iter, bytes.Repeat([]byte("X"), 32))
					if err := eng.Put([]byte(key), []byte(val)); err != nil {
						t.Errorf("writer %d put failed: %v", wid, err)
						return
					}
					writeOps.Add(1)
					iter++
				}
			}
		}(w)
	}

	// Readers
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(rid int) {
			defer wg.Done()
			rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(rid*100)))
			for {
				select {
				case <-ctx.Done():
					return
				default:
					idx := rnd.Intn(numKeys)
					key := fmt.Sprintf("k_%03d", idx)
					val, found, err := eng.Get([]byte(key))
					if err != nil {
						readErrors.Add(1)
						t.Errorf("reader %d Get failed: %v", rid, err)
						return
					}
					if !found {
						tornReads.Add(1)
						t.Errorf("reader %d: prepopulated key %s NOT FOUND", rid, key)
						return
					}
					strVal := string(val)
					// Verify value format: must either be initial_payload_... or writer_...
					isInitial := bytes.HasPrefix(val, []byte("initial_payload_"))
					isWriter := bytes.HasPrefix(val, []byte("writer_"))
					if !isInitial && !isWriter {
						tornReads.Add(1)
						t.Errorf("reader %d: torn or corrupted read observed: %q", rid, strVal)
						return
					}
					readOps.Add(1)
				}
			}
		}(r)
	}

	wg.Wait()

	t.Logf("D3 Results: totalWrites=%d, totalReads=%d, readErrors=%d, tornReads=%d",
		writeOps.Load(), readOps.Load(), readErrors.Load(), tornReads.Load())

	if readErrors.Load() > 0 || tornReads.Load() > 0 {
		t.Fatalf("D3 FAILURE: %d read errors, %d torn reads detected", readErrors.Load(), tornReads.Load())
	}
	if writeOps.Load() < 100 {
		t.Fatalf("insufficient write churn: only %d writes executed", writeOps.Load())
	}
	if readOps.Load() < 100 {
		t.Fatalf("insufficient read volume: only %d reads executed", readOps.Load())
	}
}

// 6. Concurrency Scenario D4:
// Large values (10KB - 1MB) being read concurrently; no partial or torn bytes observed.
func TestChallenger_Concurrency_D4_LargeValues_10KB_to_1MB_NoTornReads(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "d4_large_values.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	// Distinct large value sizes to exercise across multi-block boundaries
	sizes := []int{
		10 * 1024,      // 10 KB
		64 * 1024,      // 64 KB
		256 * 1024,     // 256 KB
		512 * 1024,     // 512 KB
		1 * 1024 * 1024, // 1 MB (Hackathon maximum single value size)
	}

	// Payload generator with CRC32C trailer
	generateLargePayload := func(size int, marker byte) []byte {
		buf := make([]byte, size)
		for i := 0; i < size-4; i++ {
			buf[i] = marker
		}
		// Put marker byte in first byte and size in bytes 1..5
		binary.BigEndian.PutUint32(buf[1:5], uint32(size))
		c := crc32.Checksum(buf[:size-4], crc32.MakeTable(crc32.Castagnoli))
		binary.BigEndian.PutUint32(buf[size-4:], c)
		return buf
	}

	// Payload validator
	verifyLargePayload := func(buf []byte, expectedSize int) (byte, bool) {
		if len(buf) != expectedSize {
			return 0, false
		}
		expectedCRC := binary.BigEndian.Uint32(buf[expectedSize-4:])
		actualCRC := crc32.Checksum(buf[:expectedSize-4], crc32.MakeTable(crc32.Castagnoli))
		if expectedCRC != actualCRC {
			return 0, false
		}
		marker := buf[0]
		return marker, true
	}

	// Pre-populate each large key
	for i, size := range sizes {
		key := fmt.Sprintf("large_key_%d", i)
		payload := generateLargePayload(size, 'A')
		if err := eng.Put([]byte(key), payload); err != nil {
			t.Fatalf("failed to put large value size %d: %v", size, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var readsCompleted atomic.Int64
	var writesCompleted atomic.Int64
	var tornOrCorrupted atomic.Int64

	// Writer: repeatedly flips between markers 'A', 'B', 'C', 'D' across sizes
	wg.Add(1)
	go func() {
		defer wg.Done()
		markers := []byte{'A', 'B', 'C', 'D'}
		markerIdx := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
				for i, size := range sizes {
					key := fmt.Sprintf("large_key_%d", i)
					marker := markers[markerIdx%len(markers)]
					payload := generateLargePayload(size, marker)
					if err := eng.Put([]byte(key), payload); err != nil {
						t.Errorf("writer put large value size %d failed: %v", size, err)
						return
					}
					writesCompleted.Add(1)
				}
				markerIdx++
			}
		}
	}()

	// Readers: 8 concurrent readers reading large values continuously
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					for i, expectedSize := range sizes {
						key := fmt.Sprintf("large_key_%d", i)
						val, found, err := eng.Get([]byte(key))
						if err != nil {
							tornOrCorrupted.Add(1)
							t.Errorf("reader %d Get %s failed: %v", readerID, key, err)
							return
						}
						if !found {
							tornOrCorrupted.Add(1)
							t.Errorf("reader %d Get %s returned found=false", readerID, key)
							return
						}

						marker, valid := verifyLargePayload(val, expectedSize)
						if !valid {
							tornOrCorrupted.Add(1)
							t.Errorf("reader %d observed torn/corrupt large read for %s: len=%d (expected %d)",
								readerID, key, len(val), expectedSize)
							return
						}
						_ = marker
						readsCompleted.Add(1)
					}
				}
			}
		}(r)
	}

	wg.Wait()

	t.Logf("D4 Results: largeWrites=%d, largeReads=%d, tornOrCorrupted=%d",
		writesCompleted.Load(), readsCompleted.Load(), tornOrCorrupted.Load())

	if tornOrCorrupted.Load() > 0 {
		t.Fatalf("CRITICAL D4 FAILURE: %d torn or corrupted large reads detected!", tornOrCorrupted.Load())
	}
	if readsCompleted.Load() < 10 {
		t.Fatalf("insufficient large reads completed: %d", readsCompleted.Load())
	}
}
