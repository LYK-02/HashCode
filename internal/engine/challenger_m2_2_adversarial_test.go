package engine_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kvdb/internal/engine"
	"kvdb/internal/format"
)

// ============================================================================
// CHALLENGER M2-2 ADVERSARIAL TEST SUITE
// EMPIRICAL VALIDATION OF WRITE PROTOCOL, DURABILITY, & CONCURRENCY INVARIANTS
// ============================================================================

// TestChallenger_M2_2_Crash_Scenarios_A1_to_A7_Comprehensive validates the full
// Group A crash matrix (A1 to A7) with rigorous assertions on file integrity,
// truncation to validEOF, and recovery idempotency.
func TestChallenger_M2_2_Crash_Scenarios_A1_to_A7_Comprehensive(t *testing.T) {
	// A1: Kill before any bytes reach file -> key absent, previous keys intact
	t.Run("A1_NoBytesReachFile", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "a1.db")

		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		if err := eng.Put([]byte("prior_key"), []byte("prior_val")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
		validEOF := eng.ValidEOF()
		if err := eng.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		// Reopen: "kill before write" means file size is untouched
		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Reopen failed: %v", err)
		}
		defer eng2.Close()

		if eng2.ValidEOF() != validEOF {
			t.Fatalf("A1 validEOF mismatch: expected %d, got %d", validEOF, eng2.ValidEOF())
		}
		val, found, err := eng2.Get([]byte("prior_key"))
		if err != nil || !found || string(val) != "prior_val" {
			t.Fatalf("A1 prior key corrupted: found=%v, val=%q, err=%v", found, val, err)
		}
		_, foundNew, _ := eng2.Get([]byte("killed_key"))
		if foundNew {
			t.Fatal("A1 unwritten key should NOT be found")
		}
	})

	// A2: Kill after some bytes of record written (torn write) -> torn bytes ignored/truncated
	t.Run("A2_TornWritePartialBytes", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "a2.db")

		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		_ = eng.Put([]byte("stable_key"), []byte("stable_value"))
		validEOF := eng.ValidEOF()
		_ = eng.Close()

		// Inject torn frame header (e.g. 15 bytes of 28-byte frame header)
		f, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			t.Fatalf("OpenFile failed: %v", err)
		}
		tornHeaderBytes := []byte{0x55, 0x41, 0x42, 0x45, 0x01, 0x02, 0x03, 0x04, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		if _, err := f.Write(tornHeaderBytes); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		_ = f.Sync()
		_ = f.Close()

		// Reopen must detect torn tail, truncate to validEOF, and repair cleanly
		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("A2 reopen failed to repair: %v", err)
		}
		defer eng2.Close()

		if eng2.ValidEOF() != validEOF {
			t.Fatalf("A2 ValidEOF mismatch: expected %d, got %d", validEOF, eng2.ValidEOF())
		}
		val, found, err := eng2.Get([]byte("stable_key"))
		if err != nil || !found || string(val) != "stable_value" {
			t.Fatalf("A2 stable key corrupted: found=%v, val=%q", found, val)
		}

		// Verify physical file size was truncated back
		fi, err := os.Stat(dbPath)
		if err != nil || uint64(fi.Size()) != validEOF {
			t.Fatalf("A2 physical file size not truncated: expected %d, got %d", validEOF, fi.Size())
		}
	})

	// A3 & A4: Record fully written on disk before/after fsync -> durable and recoverable
	t.Run("A3_A4_FullRecordDurable", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "a3_a4.db")

		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		if err := eng.Put([]byte("durable_a3"), []byte("payload_a3")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
		_ = eng.Close()

		// Cold reopen
		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Reopen failed: %v", err)
		}
		defer eng2.Close()

		val, found, err := eng2.Get([]byte("durable_a3"))
		if err != nil || !found || string(val) != "payload_a3" {
			t.Fatalf("A3/A4 value corrupted: found=%v, val=%q, err=%v", found, val, err)
		}
	})

	// A5: Kill immediately after put returns success -> 100% durable
	t.Run("A5_KillAfterSuccessAck", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "a5.db")

		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		for i := 1; i <= 50; i++ {
			k := fmt.Sprintf("ack_k_%d", i)
			v := fmt.Sprintf("ack_v_%d", i)
			if err := eng.Put([]byte(k), []byte(v)); err != nil {
				t.Fatalf("Put %d failed: %v", i, err)
			}
		}
		_ = eng.Close()

		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Reopen failed: %v", err)
		}
		defer eng2.Close()

		if eng2.KeyCount() != 50 {
			t.Fatalf("KeyCount mismatch: expected 50, got %d", eng2.KeyCount())
		}
		for i := 1; i <= 50; i++ {
			k := fmt.Sprintf("ack_k_%d", i)
			expectedVal := fmt.Sprintf("ack_v_%d", i)
			val, found, err := eng2.Get([]byte(k))
			if err != nil || !found || string(val) != expectedVal {
				t.Fatalf("Key %s corrupted after ACK: found=%v, val=%q", k, found, val)
			}
		}
	})

	// A6: Kill during a delete -> key either present with old value or absent, never corrupted
	t.Run("A6_TornDelete_PreservesOldValue", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "a6.db")

		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		_ = eng.Put([]byte("del_target"), []byte("del_value_safe"))
		validEOF := eng.ValidEOF()
		_ = eng.Close()

		// Append partial delete frame bytes (12 bytes)
		f, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			t.Fatal(err)
		}
		tornDeleteBytes := []byte{0x55, 0x41, 0x42, 0x45, 0xAA, 0xBB, 0xCC, 0xDD, 0x05, 0x00, 0x00, 0x00}
		_, _ = f.Write(tornDeleteBytes)
		_ = f.Close()

		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Reopen after torn delete failed: %v", err)
		}
		defer eng2.Close()

		// Key MUST still be present with old value because delete frame was torn
		val, found, err := eng2.Get([]byte("del_target"))
		if err != nil || !found || string(val) != "del_value_safe" {
			t.Fatalf("A6 failed: del_target should remain with old value: found=%v, val=%q", found, val)
		}
		if eng2.ValidEOF() != validEOF {
			t.Fatalf("A6 ValidEOF mismatch: expected %d, got %d", validEOF, eng2.ValidEOF())
		}
	})

	// A7: Kill during 1MB put (partial write of multi-block payload) -> truncated cleanly
	t.Run("A7_1MB_TornWrite_TruncatedCleanly", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "a7.db")

		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		_ = eng.Put([]byte("prior_small"), []byte("small_data"))
		validEOF := eng.ValidEOF()
		_ = eng.Close()

		// Encode a 1MB frame
		oneMBVal := make([]byte, format.MaxValLen)
		for i := range oneMBVal {
			oneMBVal[i] = byte(i % 251)
		}
		ops := []format.Operation{{
			Type:  format.OpPut,
			Key:   []byte("huge_key"),
			Value: oneMBVal,
		}}
		fullFrame, err := format.EncodeFrame(2, ops)
		if err != nil {
			t.Fatalf("EncodeFrame failed: %v", err)
		}

		// Test truncation with various cut-off points of 1MB write: 32KB, 500KB, 1MB (missing trailer)
		cutoffs := []int{
			32 * 1024,
			500 * 1024,
			len(fullFrame) - format.FrameTrailerSize, // complete payload, missing trailer
			len(fullFrame) - 2,                      // partial trailer
		}

		for _, cutoff := range cutoffs {
			// Write cutoff bytes to file
			f, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write(fullFrame[:cutoff]); err != nil {
				t.Fatal(err)
			}
			_ = f.Sync()
			_ = f.Close()

			eng2, err := engine.Open(dbPath, engine.Options{})
			if err != nil {
				t.Fatalf("Reopen at cutoff %d failed: %v", cutoff, err)
			}

			if eng2.ValidEOF() != validEOF {
				t.Fatalf("ValidEOF mismatch at cutoff %d: expected %d, got %d", cutoff, validEOF, eng2.ValidEOF())
			}
			val, found, err := eng2.Get([]byte("prior_small"))
			if err != nil || !found || string(val) != "small_data" {
				t.Fatalf("Prior key corrupted at cutoff %d: %v, %q", cutoff, found, val)
			}
			_, foundHuge, _ := eng2.Get([]byte("huge_key"))
			if foundHuge {
				t.Fatalf("Huge key should not be present at cutoff %d", cutoff)
			}
			_ = eng2.Close()
		}

		// Finally, test full 1MB write succeeds and is durable
		engFinal, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err := engFinal.Put([]byte("huge_key"), oneMBVal); err != nil {
			t.Fatalf("Put 1MB failed: %v", err)
		}
		_ = engFinal.Close()

		// Reopen and verify complete 1MB
		engCheck, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer engCheck.Close()

		gotHuge, foundHuge, err := engCheck.Get([]byte("huge_key"))
		if err != nil || !foundHuge {
			t.Fatalf("1MB key missing on reopen: found=%v, err=%v", foundHuge, err)
		}
		if !bytes.Equal(gotHuge, oneMBVal) {
			t.Fatal("1MB payload content corrupted on reopen")
		}
	})
}

// TestChallenger_M2_2_RapidOverwrites_SameKey_LastWriteWins performs 2,000 rapid
// overwrites to the exact same key with changing payload sizes and asserts
// that last-write-wins is strictly honored both in memory and after cold recovery.
func TestChallenger_M2_2_RapidOverwrites_SameKey_LastWriteWins(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "rapid_overwrites.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	key := []byte("hot_contested_key")
	iterations := 2000

	var lastVal []byte
	sizes := []int{0, 1, 17, 100, 512, 1024, 4096, 65536}

	for i := 0; i < iterations; i++ {
		sz := sizes[i%len(sizes)]
		val := make([]byte, sz)
		if sz > 0 {
			val[0] = byte(i & 0xFF)
			val[sz-1] = byte((i >> 8) & 0xFF)
			if sz > 2 {
				val[1] = 0xAA
			}
		}
		if err := eng.Put(key, val); err != nil {
			t.Fatalf("Iteration %d Put failed: %v", i, err)
		}
		lastVal = val

		// Periodic in-memory check
		if i%200 == 0 {
			got, found, err := eng.Get(key)
			if err != nil || !found {
				t.Fatalf("Iteration %d Get failed: found=%v, err=%v", i, found, err)
			}
			if !bytes.Equal(got, lastVal) {
				t.Fatalf("Iteration %d in-memory value mismatch", i)
			}
			if eng.KeyCount() != 1 {
				t.Fatalf("KeyCount should be 1, got %d", eng.KeyCount())
			}
		}
	}

	if eng.KeyCount() != 1 {
		t.Fatalf("Final KeyCount should be 1, got %d", eng.KeyCount())
	}
	_ = eng.Close()

	// Cold Reopen: must recover exact 2000th value
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer eng2.Close()

	if eng2.KeyCount() != 1 {
		t.Fatalf("Reopened KeyCount should be 1, got %d", eng2.KeyCount())
	}

	gotAfterReopen, foundAfterReopen, err := eng2.Get(key)
	if err != nil || !foundAfterReopen {
		t.Fatalf("Reopened Get failed: found=%v, err=%v", foundAfterReopen, err)
	}
	if !bytes.Equal(gotAfterReopen, lastVal) {
		t.Fatalf("Reopened value mismatch! len(got)=%d, len(expected)=%d", len(gotAfterReopen), len(lastVal))
	}

	// Verify Dump returns exactly 1 item
	pairs, err := eng2.Dump()
	if err != nil {
		t.Fatalf("Dump failed: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("Dump returned %d pairs, expected 1", len(pairs))
	}
	if pairs[0].Key != string(key) || !bytes.Equal(pairs[0].Value, lastVal) {
		t.Fatal("Dump content mismatch")
	}

	// Structural file check
	if err := engine.CheckFile(dbPath); err != nil {
		t.Fatalf("CheckFile failed: %v", err)
	}
}

// TestChallenger_M2_2_RapidInterleaving_PUT_DEL_Chronology executes 500 cycles of
// interleaved operations across multiple keys, verifying resurrection, double delete,
// and deleting non-existent keys.
func TestChallenger_M2_2_RapidInterleaving_PUT_DEL_Chronology(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "interleaving_500.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// 1. Delete non-existent key: must succeed idempotently
	if err := eng.Delete([]byte("phantom_key")); err != nil {
		t.Fatalf("Delete of missing key returned error: %v", err)
	}
	_, foundMissing, _ := eng.Get([]byte("phantom_key"))
	if foundMissing {
		t.Fatal("Phantom key should not be found")
	}

	// 2. Double delete
	_ = eng.Put([]byte("target_double"), []byte("v1"))
	if err := eng.Delete([]byte("target_double")); err != nil {
		t.Fatal(err)
	}
	if err := eng.Delete([]byte("target_double")); err != nil {
		t.Fatalf("Second delete failed: %v", err)
	}

	// 3. Resurrection: DEL -> PUT
	_ = eng.Put([]byte("target_resurrect"), []byte("old_v"))
	_ = eng.Delete([]byte("target_resurrect"))
	if err := eng.Put([]byte("target_resurrect"), []byte("resurrected_v")); err != nil {
		t.Fatal(err)
	}

	// 4. Interleaving across 5 keys for 200 cycles
	numKeys := 5
	cycles := 200
	expectedState := make(map[string][]byte) // nil indicates deleted

	for c := 0; c < cycles; c++ {
		keyStr := fmt.Sprintf("toggle_k_%d", c%numKeys)
		keyBytes := []byte(keyStr)

		if c%3 == 0 {
			// Delete
			if err := eng.Delete(keyBytes); err != nil {
				t.Fatalf("Cycle %d Delete failed: %v", c, err)
			}
			expectedState[keyStr] = nil
		} else {
			// Put
			valBytes := []byte(fmt.Sprintf("val_c_%d", c))
			if err := eng.Put(keyBytes, valBytes); err != nil {
				t.Fatalf("Cycle %d Put failed: %v", c, err)
			}
			expectedState[keyStr] = valBytes
		}
	}

	_ = eng.Close()

	// Cold Reopen: Must restore exact chronological state
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer eng2.Close()

	// Verify target_resurrect
	gotRes, foundRes, err := eng2.Get([]byte("target_resurrect"))
	if err != nil || !foundRes || string(gotRes) != "resurrected_v" {
		t.Fatalf("Resurrected key mismatch: found=%v, val=%q", foundRes, gotRes)
	}

	// Verify target_double is absent
	_, foundDouble, _ := eng2.Get([]byte("target_double"))
	if foundDouble {
		t.Fatal("Double deleted key should be absent")
	}

	// Verify all toggled keys
	for kStr, expectedVal := range expectedState {
		kBytes := []byte(kStr)
		gotVal, found, err := eng2.Get(kBytes)
		if err != nil {
			t.Fatalf("Get(%s) error: %v", kStr, err)
		}
		if expectedVal == nil {
			if found {
				t.Fatalf("Key %s should be DELETED, but found=true with val %q", kStr, gotVal)
			}
		} else {
			if !found {
				t.Fatalf("Key %s should be PRESENT, but found=false", kStr)
			}
			if !bytes.Equal(gotVal, expectedVal) {
				t.Fatalf("Key %s value mismatch: expected %q, got %q", kStr, expectedVal, gotVal)
			}
		}
	}
}

// TestChallenger_M2_2_Concurrency_D1_to_D4_RaceStress exercises concurrent read/write
// operations under the Go race detector with strict correctness validation.
func TestChallenger_M2_2_Concurrency_D1_to_D4_RaceStress(t *testing.T) {
	// D1 & D2: Disjoint & Hot Key Concurrent Writers
	t.Run("D1_D2_Concurrent_Writers", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "d1_d2.db")

		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatal(err)
		}

		numWriters := 8
		opsPerWriter := 50
		var wg sync.WaitGroup

		for w := 0; w < numWriters; w++ {
			wg.Add(1)
			go func(writerID int) {
				defer wg.Done()
				for i := 0; i < opsPerWriter; i++ {
					// D1: Disjoint key
					kDisjoint := fmt.Sprintf("disjoint_w%d_i%d", writerID, i)
					vDisjoint := fmt.Sprintf("val_w%d_i%d", writerID, i)
					if err := eng.Put([]byte(kDisjoint), []byte(vDisjoint)); err != nil {
						t.Errorf("D1 Put failed: %v", err)
						return
					}

					// D2: Same hot key
					vHot := fmt.Sprintf("hot_val_w%02d_i%04d", writerID, i)
					if err := eng.Put([]byte("contested_hot_key"), []byte(vHot)); err != nil {
						t.Errorf("D2 Put failed: %v", err)
						return
					}
				}
			}(w)
		}

		wg.Wait()
		_ = eng.Close()

		// Reopen and verify
		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Reopen failed: %v", err)
		}
		defer eng2.Close()

		// All disjoint keys must be present
		for w := 0; w < numWriters; w++ {
			for i := 0; i < opsPerWriter; i++ {
				k := fmt.Sprintf("disjoint_w%d_i%d", w, i)
				expected := fmt.Sprintf("val_w%d_i%d", w, i)
				got, found, err := eng2.Get([]byte(k))
				if err != nil || !found || string(got) != expected {
					t.Fatalf("Disjoint key %s mismatch: found=%v, got=%q", k, found, string(got))
				}
			}
		}

		// Hot key must be present with valid uncorrupted format
		gotHot, foundHot, err := eng2.Get([]byte("contested_hot_key"))
		if err != nil || !foundHot {
			t.Fatalf("Hot key missing: found=%v, err=%v", foundHot, err)
		}
		if len(gotHot) != len("hot_val_w00_i0000") {
			t.Fatalf("Torn hot key value length: %d", len(gotHot))
		}
	})

	// D3 & D4: Continuous Readers and Heavy Oscillating Writers (Zero Torn Reads)
	t.Run("D3_D4_Oscillating_NoTornReads", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "d3_d4.db")

		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer eng.Close()

		payloadSize := 4096
		payloadA := bytes.Repeat([]byte{0xAA}, payloadSize)
		payloadB := bytes.Repeat([]byte{0x55}, payloadSize)

		if err := eng.Put([]byte("flip_key"), payloadA); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
		defer cancel()

		var wg sync.WaitGroup

		// 2 Writers alternating A and B
		for w := 0; w < 2; w++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				flip := false
				for {
					select {
					case <-ctx.Done():
						return
					default:
						var p []byte
						if flip {
							p = payloadA
						} else {
							p = payloadB
						}
						_ = eng.Put([]byte("flip_key"), p)
						flip = !flip
					}
				}
			}(w)
		}

		// 6 Readers continuously asserting that every byte matches either 0xAA or 0x55 uniformly
		for r := 0; r < 6; r++ {
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
						if len(val) != payloadSize {
							t.Errorf("Torn read length: expected %d, got %d", payloadSize, len(val))
							return
						}

						// Uniformity check: every byte MUST match the first byte
						expectedByte := val[0]
						if expectedByte != 0xAA && expectedByte != 0x55 {
							t.Errorf("Torn read invalid first byte: 0x%02X", expectedByte)
							return
						}
						for idx, b := range val {
							if b != expectedByte {
								t.Fatalf("TORN READ DETECTED! Byte %d is 0x%02X, expected 0x%02X", idx, b, expectedByte)
							}
						}
					}
				}
			}()
		}

		wg.Wait()
	})
}

// TestChallenger_M2_2_BoundaryValues_DeepMatrix stress-tests:
// - 0-byte value (distinct from NOTFOUND)
// - 1MB maximum value
// - Binary payloads containing all 256 byte values
// - Key length boundaries (1 byte, 1024 bytes, empty key, 1025-byte key)
func TestChallenger_M2_2_BoundaryValues_DeepMatrix(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "boundary_deep.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// 1. Zero-byte value: distinct from NOTFOUND
	zeroKey := []byte("zero_len_value_key")
	if err := eng.Put(zeroKey, []byte{}); err != nil {
		t.Fatalf("Put zero-byte val failed: %v", err)
	}

	gotZero, foundZero, err := eng.Get(zeroKey)
	if err != nil {
		t.Fatalf("Get zeroKey error: %v", err)
	}
	if !foundZero {
		t.Fatal("Zero-byte value MUST return found=true (distinct from NOTFOUND)")
	}
	if gotZero == nil || len(gotZero) != 0 {
		t.Fatalf("Expected non-nil 0-byte slice, got: %v", gotZero)
	}

	// 2. Non-existent key: returns found=false, nil slice
	gotNonExistent, foundNonExistent, err := eng.Get([]byte("does_not_exist_at_all"))
	if err != nil {
		t.Fatal(err)
	}
	if foundNonExistent || gotNonExistent != nil {
		t.Fatalf("Non-existent key must return found=false, nil; got found=%v, val=%v", foundNonExistent, gotNonExistent)
	}

	// 3. 1MB Value Boundary (1,048,576 bytes)
	oneMBVal := make([]byte, format.MaxValLen)
	_, _ = rand.Read(oneMBVal)
	oneMBKey := []byte("max_allowed_val_key")
	if err := eng.Put(oneMBKey, oneMBVal); err != nil {
		t.Fatalf("Put 1MB failed: %v", err)
	}

	// 1MB + 1 Byte: strictly rejected with ErrValueTooLarge
	oneMBPlusOne := make([]byte, format.MaxValLen+1)
	err = eng.Put([]byte("overflow_val"), oneMBPlusOne)
	if !errors.Is(err, format.ErrValueTooLarge) {
		t.Fatalf("Expected ErrValueTooLarge, got: %v", err)
	}

	// 4. Binary payloads containing all 256 byte values
	all256Bytes := make([]byte, 256)
	for i := 0; i < 256; i++ {
		all256Bytes[i] = byte(i)
	}

	// Binary key containing all 256 byte values
	binKey := append([]byte("bin_key_"), all256Bytes...)
	// Binary value containing 4 cycles of all 256 byte values (1024 bytes)
	binVal := bytes.Repeat(all256Bytes, 4)

	if err := eng.Put(binKey, binVal); err != nil {
		t.Fatalf("Put binary key/val failed: %v", err)
	}

	// Key with null bytes (\x00) inside
	nullKey := []byte("prefix\x00middle\x00suffix")
	nullVal := []byte("\x00\x00\x00payload\x00\x00")
	if err := eng.Put(nullKey, nullVal); err != nil {
		t.Fatalf("Put null-embedded key failed: %v", err)
	}

	// 5. Key length boundaries
	// MinKeyLen = 1
	singleByteKey := []byte("x")
	if err := eng.Put(singleByteKey, []byte("single_byte_key_val")); err != nil {
		t.Fatalf("Put 1-byte key failed: %v", err)
	}

	// MaxKeyLen = 1024
	maxKey := bytes.Repeat([]byte("K"), format.MaxKeyLen)
	if err := eng.Put(maxKey, []byte("max_key_val")); err != nil {
		t.Fatalf("Put 1024-byte key failed: %v", err)
	}

	// Key length 0: rejected
	if err := eng.Put([]byte{}, []byte("v")); !errors.Is(err, format.ErrKeyEmpty) {
		t.Fatalf("Expected ErrKeyEmpty, got: %v", err)
	}

	// Key length 1025: rejected
	tooLargeKey := make([]byte, format.MaxKeyLen+1)
	if err := eng.Put(tooLargeKey, []byte("v")); !errors.Is(err, format.ErrKeyTooLarge) {
		t.Fatalf("Expected ErrKeyTooLarge, got: %v", err)
	}

	_ = eng.Close()

	// Cold Reopen: Assert all boundary items survive recovery
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer eng2.Close()

	// Verify 0-byte value
	gotZeroReopen, foundZeroReopen, err := eng2.Get(zeroKey)
	if err != nil || !foundZeroReopen || len(gotZeroReopen) != 0 {
		t.Fatalf("Zero-byte key corrupted on reopen: found=%v, len=%d", foundZeroReopen, len(gotZeroReopen))
	}

	// Verify 1MB value
	gotOneMB, foundOneMB, err := eng2.Get(oneMBKey)
	if err != nil || !foundOneMB || !bytes.Equal(gotOneMB, oneMBVal) {
		t.Fatal("1MB value corrupted on reopen")
	}

	// Verify binary key/value
	gotBinVal, foundBin, err := eng2.Get(binKey)
	if err != nil || !foundBin || !bytes.Equal(gotBinVal, binVal) {
		t.Fatal("Binary key/val corrupted on reopen")
	}

	// Verify null-embedded key/val
	gotNullVal, foundNull, err := eng2.Get(nullKey)
	if err != nil || !foundNull || !bytes.Equal(gotNullVal, nullVal) {
		t.Fatal("Null-embedded key/val corrupted on reopen")
	}

	// Verify 1-byte key
	got1B, found1B, _ := eng2.Get(singleByteKey)
	if !found1B || string(got1B) != "single_byte_key_val" {
		t.Fatal("1-byte key corrupted on reopen")
	}

	// Verify 1024-byte key
	gotMaxK, foundMaxK, _ := eng2.Get(maxKey)
	if !foundMaxK || string(gotMaxK) != "max_key_val" {
		t.Fatal("1024-byte key corrupted on reopen")
	}

	// Dump test: Verify all items present and sorted
	pairs, err := eng2.Dump()
	if err != nil {
		t.Fatalf("Dump failed: %v", err)
	}
	expectedCount := 6 // zeroKey, oneMBKey, binKey, nullKey, singleByteKey, maxKey
	if len(pairs) != expectedCount {
		t.Fatalf("Dump count mismatch: expected %d, got %d", expectedCount, len(pairs))
	}
}

// TestChallenger_M2_2_TornTail_EveryByteOffset_Stress tests torn write recovery across
// every single byte offset of a frame to prove that no matter which byte the process is
// killed at during an append, the engine recovers to the previous valid state.
func TestChallenger_M2_2_TornTail_EveryByteOffset_Stress(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "every_byte_offset.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Write 3 established keys
	_ = eng.Put([]byte("key1"), []byte("val1"))
	_ = eng.Put([]byte("key2"), []byte("val2"))
	_ = eng.Put([]byte("key3"), []byte("val3"))
	durableEOF := eng.ValidEOF()
	_ = eng.Close()

	// Encode a realistic frame for key4
	ops := []format.Operation{{
		Type:  format.OpPut,
		Key:   []byte("key4_uncommitted"),
		Value: []byte("payload_that_gets_torn_midway"),
	}}
	frameBytes, err := format.EncodeFrame(4, ops)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	// Test tearing the write at EVERY byte offset from 1 to len(frameBytes)-1
	for tearOffset := 1; tearOffset < len(frameBytes); tearOffset++ {
		// Append tearOffset bytes to data.db
		f, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			t.Fatalf("[offset %d] OpenFile failed: %v", tearOffset, err)
		}
		if _, err := f.Write(frameBytes[:tearOffset]); err != nil {
			t.Fatalf("[offset %d] Write failed: %v", tearOffset, err)
		}
		_ = f.Sync()
		_ = f.Close()

		// Reopen must truncate the torn bytes and restore durableEOF
		engReopen, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("[offset %d] Reopen failed to recover torn tail: %v", tearOffset, err)
		}

		if engReopen.ValidEOF() != durableEOF {
			t.Fatalf("[offset %d] ValidEOF not restored: expected %d, got %d", tearOffset, durableEOF, engReopen.ValidEOF())
		}
		if engReopen.KeyCount() != 3 {
			t.Fatalf("[offset %d] KeyCount dirty: expected 3, got %d", tearOffset, engReopen.KeyCount())
		}

		// Verify key4 is NOT present
		_, found4, _ := engReopen.Get([]byte("key4_uncommitted"))
		if found4 {
			t.Fatalf("[offset %d] Uncommitted torn key4 was found!", tearOffset)
		}

		_ = engReopen.Close()

		// Verify physical file size matches durableEOF
		fi, err := os.Stat(dbPath)
		if err != nil || uint64(fi.Size()) != durableEOF {
			t.Fatalf("[offset %d] File size not physically truncated: expected %d, got %d", tearOffset, durableEOF, fi.Size())
		}
	}

	// Now append a valid write to prove the file is healthy and can continue taking writes
	engHealthy, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Final open failed: %v", err)
	}
	defer engHealthy.Close()

	if err := engHealthy.Put([]byte("key4_committed"), []byte("valid_after_torn_stresses")); err != nil {
		t.Fatalf("Put after torn tail stress failed: %v", err)
	}
	if engHealthy.KeyCount() != 4 {
		t.Fatalf("Expected 4 keys, got %d", engHealthy.KeyCount())
	}
	if err := engHealthy.Check(); err != nil {
		t.Fatalf("Integrity check failed: %v", err)
	}
}
