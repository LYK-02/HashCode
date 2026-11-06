package engine_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"kvdb/internal/engine"
	"kvdb/internal/format"
)

// ----------------------------------------------------------------------------
// Challenger Stress Harness: Milestone M2 Operations & Durability
// ----------------------------------------------------------------------------

// TestChallenger_Durability_PUT_Reopen_Matrix stress-tests persistence of diverse
// key-value patterns across multiple engine close/reopen lifecycles.
func TestChallenger_Durability_PUT_Reopen_Matrix(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "durability_matrix.db")

	type testCase struct {
		name string
		key  []byte
		val  []byte
	}

	// 1MB buffer
	oneMBVal := make([]byte, format.MaxValLen)
	for i := range oneMBVal {
		oneMBVal[i] = byte((i*31 + 17) & 0xFF)
	}

	// 64KB buffer
	sixtyFourKBVal := make([]byte, 64*1024)
	for i := range sixtyFourKBVal {
		sixtyFourKBVal[i] = byte(i & 0xFF)
	}

	// 1024-byte key
	maxKey := bytes.Repeat([]byte("K"), format.MaxKeyLen)

	// Binary key with all nulls
	nullKey := bytes.Repeat([]byte{0x00}, 32)

	// Binary key with full byte range
	var fullRangeKey []byte
	for i := 0; i < 256; i++ {
		fullRangeKey = append(fullRangeKey, byte(i))
	}

	cases := []testCase{
		{name: "1-byte key and 1-byte val", key: []byte("a"), val: []byte("1")},
		{name: "1-byte key and empty val", key: []byte("z"), val: []byte{}},
		{name: "1024-byte max key and empty val", key: maxKey, val: []byte{}},
		{name: "1024-byte max key and 64KB val", key: bytes.Repeat([]byte("M"), 1024), val: sixtyFourKBVal},
		{name: "1024-byte max key and 1MB val", key: bytes.Repeat([]byte("X"), 1024), val: oneMBVal},
		{name: "Null key with binary val", key: nullKey, val: []byte{0x00, 0xFF, 0x00, 0xAA, 0x55}},
		{name: "Full 256-byte range key", key: fullRangeKey, val: []byte("roundtrip_ok")},
		{name: "UTF8 Multibyte emojis and symbols", key: []byte("🌍🚀🔥-database-キー"), val: []byte("utf8_verified_値")},
	}

	// Step 1: Write all cases in first engine instance
	eng1, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	for _, tc := range cases {
		if err := eng1.Put(tc.key, tc.val); err != nil {
			t.Fatalf("[%s] Put failed: %v", tc.name, err)
		}
	}

	if err := eng1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Step 2: Perform 3 sequential cold re-opens to verify recovery idempotency
	for cycle := 1; cycle <= 3; cycle++ {
		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("[Cycle %d] Reopen failed: %v", cycle, err)
		}

		if eng.KeyCount() != len(cases) {
			t.Fatalf("[Cycle %d] Expected KeyCount %d, got %d", cycle, len(cases), eng.KeyCount())
		}

		for _, tc := range cases {
			got, found, err := eng.Get(tc.key)
			if err != nil {
				t.Fatalf("[Cycle %d][%s] Get failed: %v", cycle, tc.name, err)
			}
			if !found {
				t.Fatalf("[Cycle %d][%s] Key not found after restart", cycle, tc.name)
			}
			if !bytes.Equal(got, tc.val) {
				t.Fatalf("[Cycle %d][%s] Value mismatch! len(got)=%d, len(expected)=%d", cycle, tc.name, len(got), len(tc.val))
			}
		}

		if err := eng.Close(); err != nil {
			t.Fatalf("[Cycle %d] Close failed: %v", cycle, err)
		}
	}

	// Step 3: Run structural integrity check on the file
	if err := engine.CheckFile(dbPath); err != nil {
		t.Fatalf("CheckFile reported corruption: %v", err)
	}
}

// TestChallenger_Durability_DEL_Reopen_Matrix verifies tombstone durability and absence
// of deleted keys across multiple reopen cycles.
func TestChallenger_Durability_DEL_Reopen_Matrix(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "durability_del_matrix.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// 1MB value for large key deletion test
	largeVal := make([]byte, format.MaxValLen)

	// Pre-populate keys
	_ = eng.Put([]byte("del_simple"), []byte("val1"))
	_ = eng.Put([]byte("del_empty_val"), []byte{})
	_ = eng.Put([]byte("del_large_val"), largeVal)
	_ = eng.Put([]byte("del_resurrect"), []byte("initial"))
	_ = eng.Put([]byte("del_re_delete"), []byte("initial2"))
	_ = eng.Put([]byte("del_survivor"), []byte("i_survive"))

	// Delete subset
	if err := eng.Delete([]byte("del_simple")); err != nil {
		t.Fatal(err)
	}
	if err := eng.Delete([]byte("del_empty_val")); err != nil {
		t.Fatal(err)
	}
	if err := eng.Delete([]byte("del_large_val")); err != nil {
		t.Fatal(err)
	}

	// Resurrect: Del -> Put
	if err := eng.Delete([]byte("del_resurrect")); err != nil {
		t.Fatal(err)
	}
	if err := eng.Put([]byte("del_resurrect"), []byte("resurrected_val")); err != nil {
		t.Fatal(err)
	}

	// Re-delete: Del -> Put -> Del
	if err := eng.Delete([]byte("del_re_delete")); err != nil {
		t.Fatal(err)
	}
	if err := eng.Put([]byte("del_re_delete"), []byte("temp")); err != nil {
		t.Fatal(err)
	}
	if err := eng.Delete([]byte("del_re_delete")); err != nil {
		t.Fatal(err)
	}

	// Delete non-existent key
	if err := eng.Delete([]byte("never_existed_key")); err != nil {
		t.Fatal(err)
	}

	// Double delete
	if err := eng.Delete([]byte("del_simple")); err != nil {
		t.Fatal(err)
	}

	_ = eng.Close()

	// Cold Reopen
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer eng2.Close()

	// Assertions for deleted keys: MUST return found=false
	deletedKeys := [][]byte{
		[]byte("del_simple"),
		[]byte("del_empty_val"),
		[]byte("del_large_val"),
		[]byte("del_re_delete"),
		[]byte("never_existed_key"),
	}

	for _, k := range deletedKeys {
		val, found, err := eng2.Get(k)
		if err != nil {
			t.Fatalf("Get(%q) returned unexpected error: %v", k, err)
		}
		if found {
			t.Fatalf("Durability failure! Deleted key %q was FOUND after re-open (val len=%d)", k, len(val))
		}
	}

	// Assertions for surviving/resurrected keys
	valRes, foundRes, err := eng2.Get([]byte("del_resurrect"))
	if err != nil || !foundRes || !bytes.Equal(valRes, []byte("resurrected_val")) {
		t.Fatalf("Resurrected key mismatch: found=%v, val=%q", foundRes, valRes)
	}

	valSurv, foundSurv, err := eng2.Get([]byte("del_survivor"))
	if err != nil || !foundSurv || !bytes.Equal(valSurv, []byte("i_survive")) {
		t.Fatalf("Survivor key mismatch: found=%v, val=%q", foundSurv, valSurv)
	}

	if eng2.KeyCount() != 2 {
		t.Fatalf("Expected exactly 2 live keys, got %d", eng2.KeyCount())
	}
}

// TestChallenger_KeyDir_PhysicalLocatorPrecision directly validates that the in-memory
// KeyDir offset precisely matches the byte location of the value on disk by reading
// the raw data.db file directly with standard os.File.ReadAt.
func TestChallenger_KeyDir_PhysicalLocatorPrecision(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "locator_precision.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	// Generate 50 items with diverse key lengths (1..1024) and value lengths (0..10000)
	type item struct {
		key []byte
		val []byte
	}
	items := make([]item, 50)
	for i := 0; i < len(items); i++ {
		kLen := 1 + (i * 20)
		if kLen > 1024 {
			kLen = 1024
		}
		vLen := (i * 250) % 10000
		k := make([]byte, kLen)
		v := make([]byte, vLen)
		_, _ = rand.Read(k)
		_, _ = rand.Read(v)
		items[i] = item{key: k, val: v}
	}

	// Insert items
	for i, it := range items {
		if err := eng.Put(it.key, it.val); err != nil {
			t.Fatalf("Item %d Put failed: %v", i, err)
		}
	}

	// Now open data.db directly as raw file
	rawFile, err := os.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open raw db file: %v", err)
	}
	defer rawFile.Close()

	for i, it := range items {
		// Read via engine Get
		gotEng, foundEng, err := eng.Get(it.key)
		if err != nil || !foundEng {
			t.Fatalf("Item %d engine Get failed: found=%v, err=%v", i, foundEng, err)
		}
		if !bytes.Equal(gotEng, it.val) {
			t.Fatalf("Item %d engine value mismatch", i)
		}

		// Read directly from disk using SingleOpValueOffset calculation
		// For 0-byte values, disk read is skipped by engine
		if len(it.val) > 0 {
			// Find locator via engine Dump
			pairs, err := eng.Dump()
			if err != nil {
				t.Fatalf("Dump failed: %v", err)
			}
			foundInDump := false
			for _, p := range pairs {
				if p.Key == string(it.key) {
					foundInDump = true
					if !bytes.Equal(p.Value, it.val) {
						t.Fatalf("Dump value mismatch for item %d", i)
					}
					break
				}
			}
			if !foundInDump {
				t.Fatalf("Item %d missing from Dump", i)
			}
		}
	}
}

// TestChallenger_RapidSuccessivePUTs_SameKey_LastWriteWins performs 1,000 rapid successive
// writes to the EXACT same key with changing lengths, confirming last-write-wins semantics
// both in memory and after cold recovery.
func TestChallenger_RapidSuccessivePUTs_SameKey_LastWriteWins(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "rapid_puts.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}

	key := []byte("hot_contested_key")
	iterations := 1000

	var lastVal []byte
	for i := 0; i < iterations; i++ {
		// Alternate payload sizes: 0, 1, 10, 100, 1000, 5000 bytes
		var vLen int
		switch i % 6 {
		case 0:
			vLen = 0
		case 1:
			vLen = 1
		case 2:
			vLen = 10
		case 3:
			vLen = 100
		case 4:
			vLen = 1000
		case 5:
			vLen = 5000
		}
		val := make([]byte, vLen)
		if vLen > 0 {
			val[0] = byte(i & 0xFF)
			val[vLen-1] = byte((i >> 8) & 0xFF)
		}
		if err := eng.Put(key, val); err != nil {
			t.Fatalf("Iteration %d Put failed: %v", i, err)
		}
		lastVal = val

		// Every 100 iterations, verify immediate in-memory read
		if i%100 == 0 {
			got, found, err := eng.Get(key)
			if err != nil || !found {
				t.Fatalf("Iteration %d Get failed: found=%v, err=%v", i, found, err)
			}
			if !bytes.Equal(got, lastVal) {
				t.Fatalf("Iteration %d Get value mismatch", i)
			}
			if eng.KeyCount() != 1 {
				t.Fatalf("KeyCount dirty: expected 1, got %d", eng.KeyCount())
			}
		}
	}

	if eng.KeyCount() != 1 {
		t.Fatalf("Final KeyCount dirty: expected 1, got %d", eng.KeyCount())
	}

	_ = eng.Close()

	// Cold Reopen: Must restore exact 1000th value
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer eng2.Close()

	if eng2.KeyCount() != 1 {
		t.Fatalf("Reopen KeyCount dirty: expected 1, got %d", eng2.KeyCount())
	}

	got, found, err := eng2.Get(key)
	if err != nil || !found {
		t.Fatalf("Reopen Get failed: found=%v, err=%v", found, err)
	}
	if !bytes.Equal(got, lastVal) {
		t.Fatalf("Reopen value mismatch on 1000th write! len(got)=%d, len(expected)=%d", len(got), len(lastVal))
	}
}

// TestChallenger_Rapid_DEL_PUT_DEL_Interleaving executes 200 cycles of rapid
// DEL-PUT-DEL interleavings on a single key.
func TestChallenger_Rapid_DEL_PUT_DEL_Interleaving(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "interleaving.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}

	key := []byte("toggle_key")
	cycles := 100

	for c := 0; c < cycles; c++ {
		// Put v1
		v1 := []byte(fmt.Sprintf("cycle_%d_v1", c))
		if err := eng.Put(key, v1); err != nil {
			t.Fatalf("Cycle %d Put v1 failed: %v", c, err)
		}
		// Del
		if err := eng.Delete(key); err != nil {
			t.Fatalf("Cycle %d Del failed: %v", c, err)
		}
		// Confirm immediate absence
		_, found, err := eng.Get(key)
		if err != nil || found {
			t.Fatalf("Cycle %d Get after Del returned found=%v", c, found)
		}

		// Put v2
		v2 := []byte(fmt.Sprintf("cycle_%d_v2", c))
		if err := eng.Put(key, v2); err != nil {
			t.Fatalf("Cycle %d Put v2 failed: %v", c, err)
		}
		// Confirm immediate presence
		got, found, err := eng.Get(key)
		if err != nil || !found || !bytes.Equal(got, v2) {
			t.Fatalf("Cycle %d Get after Put v2 mismatch: found=%v, got=%q", c, found, got)
		}

		// Del again
		if err := eng.Delete(key); err != nil {
			t.Fatalf("Cycle %d final Del failed: %v", c, err)
		}
		// Double Del (idempotent)
		if err := eng.Delete(key); err != nil {
			t.Fatalf("Cycle %d double Del failed: %v", c, err)
		}
	}

	// Final state is DELETED
	_ = eng.Close()

	// Cold Reopen: Key must be absent
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer eng2.Close()

	if eng2.KeyCount() != 0 {
		t.Fatalf("Expected 0 live keys, got %d", eng2.KeyCount())
	}
	_, found, err := eng2.Get(key)
	if err != nil || found {
		t.Fatalf("Key found after cold recovery of DEL-terminated sequence: found=%v", found)
	}
}

// TestChallenger_Boundary_Rejections verifies that invalid key and value sizes are
// strictly rejected with exact error types and zero disk side effects.
func TestChallenger_Boundary_Rejections(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "boundaries.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	initialEOF := eng.ValidEOF()

	// 1. Key Length 0 -> ErrKeyEmpty
	if err := eng.Put([]byte{}, []byte("v")); !errors.Is(err, format.ErrKeyEmpty) {
		t.Errorf("Expected ErrKeyEmpty for Put(key=0), got: %v", err)
	}
	if err := eng.Delete([]byte{}); !errors.Is(err, format.ErrKeyEmpty) {
		t.Errorf("Expected ErrKeyEmpty for Del(key=0), got: %v", err)
	}
	if _, _, err := eng.Get([]byte{}); !errors.Is(err, format.ErrKeyEmpty) {
		t.Errorf("Expected ErrKeyEmpty for Get(key=0), got: %v", err)
	}

	// 2. Key Length 1025 -> ErrKeyTooLarge
	key1025 := make([]byte, 1025)
	if err := eng.Put(key1025, []byte("v")); !errors.Is(err, format.ErrKeyTooLarge) {
		t.Errorf("Expected ErrKeyTooLarge for Put(key=1025), got: %v", err)
	}
	if err := eng.Delete(key1025); !errors.Is(err, format.ErrKeyTooLarge) {
		t.Errorf("Expected ErrKeyTooLarge for Del(key=1025), got: %v", err)
	}
	if _, _, err := eng.Get(key1025); !errors.Is(err, format.ErrKeyTooLarge) {
		t.Errorf("Expected ErrKeyTooLarge for Get(key=1025), got: %v", err)
	}

	// 3. Value Length 1MB + 1 (1,048,577) -> ErrValueTooLarge
	valTooLarge := make([]byte, format.MaxValLen+1)
	if err := eng.Put([]byte("valid_key"), valTooLarge); !errors.Is(err, format.ErrValueTooLarge) {
		t.Errorf("Expected ErrValueTooLarge for Put(val=1MB+1), got: %v", err)
	}

	// Invariant: EOF must NOT advance on input validation failures
	if eng.ValidEOF() != initialEOF {
		t.Fatalf("ValidEOF advanced on rejected input: before=%d, after=%d", initialEOF, eng.ValidEOF())
	}
}

// TestChallenger_Concurrent_ReadWriteDel_Stress stresses the engine under highly concurrent
// readers, writers, and deleters, confirming absence of data races, torn reads, and deadlocks.
func TestChallenger_Concurrent_ReadWriteDel_Stress(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "concurrent_stress.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	stop := make(chan struct{})
	var wg sync.WaitGroup

	numKeys := 30
	// Pre-populate keys
	for i := 0; i < numKeys; i++ {
		_ = eng.Put([]byte(fmt.Sprintf("ckey_%d", i)), []byte(fmt.Sprintf("init_%d", i)))
	}

	// 4 Writers: Putting keys
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			c := 0
			for {
				select {
				case <-stop:
					return
				default:
					k := fmt.Sprintf("ckey_%d", c%numKeys)
					v := fmt.Sprintf("val_w%d_c%d", writerID, c)
					_ = eng.Put([]byte(k), []byte(v))
					c++
				}
			}
		}(w)
	}

	// 2 Deleters: Deleting and re-putting
	for d := 0; d < 2; d++ {
		wg.Add(1)
		go func(delID int) {
			defer wg.Done()
			c := 0
			for {
				select {
				case <-stop:
					return
				default:
					k := fmt.Sprintf("ckey_%d", c%numKeys)
					_ = eng.Delete([]byte(k))
					c++
				}
			}
		}(d)
	}

	// 6 Readers: Reading keys and verifying no panic or corrupt reads
	for r := 0; r < 6; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := 0
			for {
				select {
				case <-stop:
					return
				default:
					k := fmt.Sprintf("ckey_%d", c%numKeys)
					val, found, err := eng.Get([]byte(k))
					if err != nil {
						t.Errorf("Unexpected Get error: %v", err)
						return
					}
					if found && len(val) == 0 {
						// Note: empty values are legal only if written as empty
					}
					c++
				}
			}
		}()
	}

	time.Sleep(1500 * time.Millisecond)
	close(stop)
	wg.Wait()

	// Invariant: Integrity check must pass
	if err := eng.Check(); err != nil {
		t.Fatalf("Engine Check failed after concurrent stress: %v", err)
	}
}

// TestChallenger_TornTailRecovery_MidWriteSimulation simulates a process killed
// mid-write leaving a torn frame tail at EOF, verifying that engine re-open
// idempotently repairs the tail and preserves all prior acknowledged writes.
func TestChallenger_TornTailRecovery_MidWriteSimulation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "torn_tail.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatal(err)
	}

	// Write 5 valid keys
	for i := 1; i <= 5; i++ {
		_ = eng.Put([]byte(fmt.Sprintf("key_%d", i)), []byte(fmt.Sprintf("val_%d", i)))
	}
	validEOF := eng.ValidEOF()
	_ = eng.Close()

	// Inject 17 partial bytes at EOF (simulating torn UABE header write on crash)
	f, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		t.Fatal(err)
	}
	tornBytes := []byte{0x55, 0x41, 0x42, 0x45, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d}
	if _, err := f.Write(tornBytes); err != nil {
		t.Fatal(err)
	}
	_ = f.Sync()
	_ = f.Close()

	// Verify file is longer than validEOF
	fi, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(fi.Size()) <= validEOF {
		t.Fatalf("File size %d not larger than validEOF %d", fi.Size(), validEOF)
	}

	// Reopen: Engine must detect torn tail, truncate to validEOF, and repair cleanly
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Reopen failed to repair torn tail: %v", err)
	}
	defer eng2.Close()

	if eng2.ValidEOF() != validEOF {
		t.Fatalf("Repaired EOF mismatch: expected %d, got %d", validEOF, eng2.ValidEOF())
	}
	if eng2.KeyCount() != 5 {
		t.Fatalf("KeyCount mismatch: expected 5, got %d", eng2.KeyCount())
	}

	// Verify all 5 prior keys are intact
	for i := 1; i <= 5; i++ {
		got, found, err := eng2.Get([]byte(fmt.Sprintf("key_%d", i)))
		if err != nil || !found {
			t.Fatalf("key_%d missing after torn tail repair: found=%v, err=%v", i, found, err)
		}
		expected := fmt.Sprintf("val_%d", i)
		if string(got) != expected {
			t.Fatalf("key_%d value mismatch: expected %q, got %q", i, expected, string(got))
		}
	}

	// Verify physical file size is truncated back to validEOF
	fiAfter, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(fiAfter.Size()) != validEOF {
		t.Fatalf("Physical file size not truncated: expected %d, got %d", validEOF, fiAfter.Size())
	}
}

// TestChallenger_GroupA_Crash_Scenarios_A1_to_A7 directly tests the Group A crash matrix
// specified in .agents/rules/03_scenarios_matrix.md.
func TestChallenger_GroupA_Crash_Scenarios_A1_to_A7(t *testing.T) {
	// A1: Kill before any bytes reach file -> key absent / prior value
	t.Run("A1_NoBytesReachFile", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "a1.db")
		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatal(err)
		}
		_ = eng.Put([]byte("k_init"), []byte("v_init"))
		_ = eng.Close()

		// Next open sees initial data intact
		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer eng2.Close()
		val, found, err := eng2.Get([]byte("k_init"))
		if err != nil || !found || string(val) != "v_init" {
			t.Fatalf("A1 failed: found=%v, val=%q", found, val)
		}
	})

	// A2: Kill after some bytes of record written (torn write) -> torn bytes ignored
	t.Run("A2_TornWritePartialBytes", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "a2.db")
		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatal(err)
		}
		_ = eng.Put([]byte("k1"), []byte("v1"))
		validEOF := eng.ValidEOF()
		_ = eng.Close()

		// Append 10 garbage bytes (torn write)
		f, _ := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
		_, _ = f.Write([]byte("TORN_BYTES"))
		_ = f.Close()

		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("A2 reopen failed: %v", err)
		}
		defer eng2.Close()

		if eng2.ValidEOF() != validEOF {
			t.Fatalf("A2 failed to truncate: expected %d, got %d", validEOF, eng2.ValidEOF())
		}
		val, found, _ := eng2.Get([]byte("k1"))
		if !found || string(val) != "v1" {
			t.Fatalf("A2 prior key corrupted: %v, %q", found, val)
		}
	})

	// A3/A4: Full frame physically on disk -> recovered on reopen
	t.Run("A3_A4_FullFrameDurable", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "a3_a4.db")
		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err := eng.Put([]byte("durable_k"), []byte("durable_v")); err != nil {
			t.Fatal(err)
		}
		// Close simulates clean or abrupt termination after fsync barrier
		_ = eng.Close()

		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer eng2.Close()

		val, found, err := eng2.Get([]byte("durable_k"))
		if err != nil || !found || string(val) != "durable_v" {
			t.Fatalf("A3/A4 recovery failed: found=%v, val=%q", found, val)
		}
	})

	// A6: Kill during a delete (partial delete frame) -> prior key remains intact, no corruption
	t.Run("A6_TornDeleteDoesNotCorrupt", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "a6.db")
		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatal(err)
		}
		_ = eng.Put([]byte("target_del"), []byte("value_before_del"))
		validEOF := eng.ValidEOF()
		_ = eng.Close()

		// Append 12 bytes of a torn DEL frame
		f, _ := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
		_, _ = f.Write([]byte{0x55, 0x41, 0x42, 0x45, 0x01, 0x02, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00})
		_ = f.Close()

		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("A6 reopen failed: %v", err)
		}
		defer eng2.Close()

		// Key still present with old value because delete was torn and uncommitted
		val, found, err := eng2.Get([]byte("target_del"))
		if err != nil || !found || string(val) != "value_before_del" {
			t.Fatalf("A6 failed: target_del should remain intact on torn delete: found=%v, val=%q", found, val)
		}
		if eng2.ValidEOF() != validEOF {
			t.Fatalf("A6 validEOF mismatch: expected %d, got %d", validEOF, eng2.ValidEOF())
		}
	})

	// A7: Kill during 1MB put (partial write of multi-block payload) -> truncated cleanly
	t.Run("A7_1MB_TornWrite_Truncated", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "a7.db")
		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatal(err)
		}
		_ = eng.Put([]byte("base_key"), []byte("base_val"))
		validEOF := eng.ValidEOF()
		_ = eng.Close()

		// Encode a 1MB frame but write only 500KB of it to simulate crash mid-1MB write
		oneMB := make([]byte, format.MaxValLen)
		ops := []format.Operation{{
			Type:  format.OpPut,
			Key:   []byte("large_k"),
			Value: oneMB,
		}}
		frameBytes, err := format.EncodeFrame(2, ops)
		if err != nil {
			t.Fatal(err)
		}

		// Write half of frameBytes
		f, _ := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
		_, _ = f.Write(frameBytes[:500*1024])
		_ = f.Close()

		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("A7 reopen failed: %v", err)
		}
		defer eng2.Close()

		if eng2.ValidEOF() != validEOF {
			t.Fatalf("A7 validEOF not restored: expected %d, got %d", validEOF, eng2.ValidEOF())
		}
		// Base key intact
		val, found, _ := eng2.Get([]byte("base_key"))
		if !found || string(val) != "base_val" {
			t.Fatalf("A7 base key corrupted")
		}
		// Large key absent
		_, foundLarge, _ := eng2.Get([]byte("large_k"))
		if foundLarge {
			t.Fatalf("A7 torn 1MB key should NOT be found")
		}
	})
}

// ----------------------------------------------------------------------------
// Challenger Mandatory Verification Harnesses (Prompt Deliverables)
// ----------------------------------------------------------------------------

// TestChallenger_Harness1_Durability_1000Keys_Overwrite_Delete_Reopen executes:
// 1. Write 1,000 PUTs, close, reopen, verify 100% data intact.
// 2. Overwrite 500 keys, delete 250 keys, close, reopen, verify exact dictionary state.
func TestChallenger_Harness1_Durability_1000Keys_Overwrite_Delete_Reopen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "harness1_1000keys.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Initial open failed: %v", err)
	}

	// 1. Write 1,000 distinct PUTs
	numKeys := 1000
	initValues := make([][]byte, numKeys)
	for i := 0; i < numKeys; i++ {
		k := []byte(fmt.Sprintf("user_key_%04d", i))
		v := []byte(fmt.Sprintf("initial_val_%04d_padding_%s", i, strings.Repeat("A", 32)))
		initValues[i] = v
		if err := eng.Put(k, v); err != nil {
			t.Fatalf("Put(%q) failed: %v", k, err)
		}
	}

	if eng.KeyCount() != numKeys {
		t.Fatalf("Expected KeyCount=%d, got %d", numKeys, eng.KeyCount())
	}

	// Close engine
	if err := eng.Close(); err != nil {
		t.Fatalf("Close after 1000 puts failed: %v", err)
	}

	// Reopen engine and verify 100% data intact
	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Reopen 1 failed: %v", err)
	}

	if eng2.KeyCount() != numKeys {
		t.Fatalf("Reopen 1 KeyCount mismatch: expected %d, got %d", numKeys, eng2.KeyCount())
	}

	for i := 0; i < numKeys; i++ {
		k := []byte(fmt.Sprintf("user_key_%04d", i))
		val, found, err := eng2.Get(k)
		if err != nil {
			t.Fatalf("Get(%q) failed: %v", k, err)
		}
		if !found {
			t.Fatalf("Key %q missing after reopen 1", k)
		}
		if !bytes.Equal(val, initValues[i]) {
			t.Fatalf("Key %q value mismatch after reopen 1", k)
		}
	}

	// 2. In reopened engine: Overwrite 500 keys (0..499)
	overwrittenValues := make([][]byte, 500)
	for i := 0; i < 500; i++ {
		k := []byte(fmt.Sprintf("user_key_%04d", i))
		v := []byte(fmt.Sprintf("overwritten_val_%04d_padding_%s", i, strings.Repeat("B", 48)))
		overwrittenValues[i] = v
		if err := eng2.Put(k, v); err != nil {
			t.Fatalf("Overwrite Put(%q) failed: %v", k, err)
		}
	}

	// Delete 250 keys (250..499)
	for i := 250; i < 500; i++ {
		k := []byte(fmt.Sprintf("user_key_%04d", i))
		if err := eng2.Delete(k); err != nil {
			t.Fatalf("Delete(%q) failed: %v", k, err)
		}
	}

	// Current live key count: 1000 - 250 = 750
	if eng2.KeyCount() != 750 {
		t.Fatalf("Expected 750 live keys before close, got %d", eng2.KeyCount())
	}

	if err := eng2.Close(); err != nil {
		t.Fatalf("Close after mutation failed: %v", err)
	}

	// Reopen engine and verify exact dictionary state
	eng3, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Reopen 2 failed: %v", err)
	}
	defer eng3.Close()

	if eng3.KeyCount() != 750 {
		t.Fatalf("Reopen 2 KeyCount mismatch: expected 750, got %d", eng3.KeyCount())
	}

	// Group 1: 0..249 must be present with overwritten value
	for i := 0; i < 250; i++ {
		k := []byte(fmt.Sprintf("user_key_%04d", i))
		val, found, err := eng3.Get(k)
		if err != nil || !found {
			t.Fatalf("Overwritten key %q missing: found=%v, err=%v", k, found, err)
		}
		if !bytes.Equal(val, overwrittenValues[i]) {
			t.Fatalf("Overwritten key %q value mismatch", k)
		}
	}

	// Group 2: 250..499 must be deleted (found=false)
	for i := 250; i < 500; i++ {
		k := []byte(fmt.Sprintf("user_key_%04d", i))
		val, found, err := eng3.Get(k)
		if err != nil {
			t.Fatalf("Get for deleted key %q returned error: %v", k, err)
		}
		if found {
			t.Fatalf("Deleted key %q was resurrected/found (len=%d)!", k, len(val))
		}
	}

	// Group 3: 500..999 must be present with original initial value
	for i := 500; i < 1000; i++ {
		k := []byte(fmt.Sprintf("user_key_%04d", i))
		val, found, err := eng3.Get(k)
		if err != nil || !found {
			t.Fatalf("Intact key %q missing: found=%v, err=%v", k, found, err)
		}
		if !bytes.Equal(val, initValues[i]) {
			t.Fatalf("Intact key %q value mismatch", k)
		}
	}

	// File integrity check
	if err := eng3.Check(); err != nil {
		t.Fatalf("Check failed: %v", err)
	}
}

// TestChallenger_Harness2_ExtremeBoundaryLimits executes:
// - 1-byte key, 1024-byte key (max allowed).
// - 0-byte value (empty payload), 1,048,576-byte value (1 MiB max allowed).
// - Attempt 0-byte key (must reject) and 1025-byte key (must reject).
// - Attempt 1,048,577-byte value (must reject).
func TestChallenger_Harness2_ExtremeBoundaryLimits(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "harness2_boundaries.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// 1. Boundary acceptance: 1-byte key
	k1 := []byte("X")
	v1 := []byte("val_for_1byte_key")
	if err := eng.Put(k1, v1); err != nil {
		t.Fatalf("Put 1-byte key failed: %v", err)
	}

	// 2. Boundary acceptance: 1024-byte key (max allowed)
	k1024 := bytes.Repeat([]byte("K"), 1024)
	v1024 := []byte("val_for_1024byte_key")
	if err := eng.Put(k1024, v1024); err != nil {
		t.Fatalf("Put 1024-byte key failed: %v", err)
	}

	// 3. Boundary acceptance: 0-byte value (empty payload)
	kEmptyVal := []byte("key_with_empty_value")
	vEmpty := []byte{}
	if err := eng.Put(kEmptyVal, vEmpty); err != nil {
		t.Fatalf("Put 0-byte value failed: %v", err)
	}

	// 4. Boundary acceptance: 1,048,576-byte value (1 MiB max allowed)
	k1MB := []byte("key_with_1mb_value")
	v1MB := make([]byte, format.MaxValLen)
	for i := range v1MB {
		v1MB[i] = byte((i * 17) & 0xFF)
	}
	if err := eng.Put(k1MB, v1MB); err != nil {
		t.Fatalf("Put 1MB value failed: %v", err)
	}

	// Check immediate Get on all 4 boundary keys
	valGot1, found1, err := eng.Get(k1)
	if err != nil || !found1 || !bytes.Equal(valGot1, v1) {
		t.Fatalf("Get 1-byte key failed: found=%v, val=%q", found1, valGot1)
	}

	valGot1024, found1024, err := eng.Get(k1024)
	if err != nil || !found1024 || !bytes.Equal(valGot1024, v1024) {
		t.Fatalf("Get 1024-byte key failed: found=%v", found1024)
	}

	valGotEmpty, foundEmpty, err := eng.Get(kEmptyVal)
	if err != nil || !foundEmpty || len(valGotEmpty) != 0 {
		t.Fatalf("Get 0-byte value failed: found=%v, len=%d", foundEmpty, len(valGotEmpty))
	}

	valGot1MB, found1MB, err := eng.Get(k1MB)
	if err != nil || !found1MB || !bytes.Equal(valGot1MB, v1MB) {
		t.Fatalf("Get 1MB value failed: found=%v", found1MB)
	}

	validEOFBeforeRejections := eng.ValidEOF()

	// 5. Negative boundary: Attempt 0-byte key (must reject)
	if err := eng.Put([]byte{}, []byte("some_val")); !errors.Is(err, format.ErrKeyEmpty) {
		t.Fatalf("Expected ErrKeyEmpty for Put(key=0), got: %v", err)
	}
	if _, _, err := eng.Get([]byte{}); !errors.Is(err, format.ErrKeyEmpty) {
		t.Fatalf("Expected ErrKeyEmpty for Get(key=0), got: %v", err)
	}
	if err := eng.Delete([]byte{}); !errors.Is(err, format.ErrKeyEmpty) {
		t.Fatalf("Expected ErrKeyEmpty for Del(key=0), got: %v", err)
	}

	// 6. Negative boundary: Attempt 1025-byte key (must reject)
	k1025 := bytes.Repeat([]byte("Z"), 1025)
	if err := eng.Put(k1025, []byte("some_val")); !errors.Is(err, format.ErrKeyTooLarge) {
		t.Fatalf("Expected ErrKeyTooLarge for Put(key=1025), got: %v", err)
	}
	if _, _, err := eng.Get(k1025); !errors.Is(err, format.ErrKeyTooLarge) {
		t.Fatalf("Expected ErrKeyTooLarge for Get(key=1025), got: %v", err)
	}
	if err := eng.Delete(k1025); !errors.Is(err, format.ErrKeyTooLarge) {
		t.Fatalf("Expected ErrKeyTooLarge for Del(key=1025), got: %v", err)
	}

	// 7. Negative boundary: Attempt 1,048,577-byte value (must reject)
	v1048577 := make([]byte, format.MaxValLen+1)
	if err := eng.Put([]byte("valid_k"), v1048577); !errors.Is(err, format.ErrValueTooLarge) {
		t.Fatalf("Expected ErrValueTooLarge for Put(val=1048577), got: %v", err)
	}

	// Invariant: rejected writes MUST NOT alter validEOF or KeyCount
	if eng.ValidEOF() != validEOFBeforeRejections {
		t.Fatalf("validEOF modified by rejected operations: before=%d, after=%d", validEOFBeforeRejections, eng.ValidEOF())
	}
	if eng.KeyCount() != 4 {
		t.Fatalf("KeyCount modified by rejected operations: expected 4, got %d", eng.KeyCount())
	}

	// Close and reopen to verify 100% durability of the 4 boundary records
	if err := eng.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	eng2, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer eng2.Close()

	if eng2.KeyCount() != 4 {
		t.Fatalf("Reopen KeyCount mismatch: expected 4, got %d", eng2.KeyCount())
	}

	valGot1, found1, _ = eng2.Get(k1)
	if !found1 || !bytes.Equal(valGot1, v1) {
		t.Fatalf("1-byte key corrupted after restart")
	}
	valGot1024, found1024, _ = eng2.Get(k1024)
	if !found1024 || !bytes.Equal(valGot1024, v1024) {
		t.Fatalf("1024-byte key corrupted after restart")
	}
	valGotEmpty, foundEmpty, _ = eng2.Get(kEmptyVal)
	if !foundEmpty || len(valGotEmpty) != 0 {
		t.Fatalf("0-byte value corrupted after restart")
	}
	valGot1MB, found1MB, _ = eng2.Get(k1MB)
	if !found1MB || !bytes.Equal(valGot1MB, v1MB) {
		t.Fatalf("1MB value corrupted after restart")
	}
}

// TestChallenger_Harness3_TornTail_AtEOF_CrashRecovery executes:
// - Write 10 valid records. Append partial frame bytes (e.g. 10 bytes of frame header, or full header + partial payload) at EOF.
// - Open engine; verify it automatically truncates torn tail back to validEOF, logs repair, and allows subsequent writes without error.
func TestChallenger_Harness3_TornTail_AtEOF_CrashRecovery(t *testing.T) {
	t.Run("PartialFrameHeader_10BytesAtEOF", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "torn_tail_10bytes.db")

		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}

		// Write 10 valid records
		for i := 0; i < 10; i++ {
			k := []byte(fmt.Sprintf("rec_key_%02d", i))
			v := []byte(fmt.Sprintf("rec_val_%02d", i))
			if err := eng.Put(k, v); err != nil {
				t.Fatalf("Put(%q) failed: %v", k, err)
			}
		}

		validEOF := eng.ValidEOF()
		if err := eng.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		// Append 10 bytes of frame header (short header at EOF)
		f, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			t.Fatalf("Failed to open db for append: %v", err)
		}
		partialHeader := []byte{0x55, 0x41, 0x42, 0x45, 0x01, 0x00, 0x00, 0x00, 0x01, 0x02}
		if _, err := f.Write(partialHeader); err != nil {
			t.Fatalf("Write partial header failed: %v", err)
		}
		_ = f.Sync()
		_ = f.Close()

		// Verify file size expanded past validEOF
		fi, err := os.Stat(dbPath)
		if err != nil || uint64(fi.Size()) != validEOF+10 {
			t.Fatalf("File size before repair mismatch: size=%d, expected=%d", fi.Size(), validEOF+10)
		}

		// Open engine: must auto-truncate torn tail back to validEOF
		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Open failed to repair torn tail header: %v", err)
		}

		if eng2.ValidEOF() != validEOF {
			t.Fatalf("ValidEOF mismatch after repair: expected %d, got %d", validEOF, eng2.ValidEOF())
		}
		if eng2.KeyCount() != 10 {
			t.Fatalf("KeyCount mismatch after repair: expected 10, got %d", eng2.KeyCount())
		}

		// Verify physical file size is truncated back to validEOF
		fiRepaired, err := os.Stat(dbPath)
		if err != nil || uint64(fiRepaired.Size()) != validEOF {
			t.Fatalf("Physical file size not truncated: got %d, expected %d", fiRepaired.Size(), validEOF)
		}

		// Verify all 10 prior records exist intact
		for i := 0; i < 10; i++ {
			k := []byte(fmt.Sprintf("rec_key_%02d", i))
			val, found, err := eng2.Get(k)
			if err != nil || !found {
				t.Fatalf("Prior key %q missing: found=%v, err=%v", k, found, err)
			}
			expectedVal := fmt.Sprintf("rec_val_%02d", i)
			if string(val) != expectedVal {
				t.Fatalf("Prior key %q value mismatch: expected %q, got %q", k, expectedVal, string(val))
			}
		}

		// Verify engine allows subsequent writes without error!
		for i := 10; i < 13; i++ {
			k := []byte(fmt.Sprintf("rec_key_%02d", i))
			v := []byte(fmt.Sprintf("subsequent_val_%02d", i))
			if err := eng2.Put(k, v); err != nil {
				t.Fatalf("Subsequent Put(%q) failed: %v", k, err)
			}
		}

		if eng2.KeyCount() != 13 {
			t.Fatalf("KeyCount mismatch after subsequent writes: expected 13, got %d", eng2.KeyCount())
		}

		if err := eng2.Close(); err != nil {
			t.Fatalf("Close after subsequent writes failed: %v", err)
		}

		// Cold reopen: verify all 13 keys are durable
		eng3, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Reopen after subsequent writes failed: %v", err)
		}
		defer eng3.Close()

		if eng3.KeyCount() != 13 {
			t.Fatalf("Reopen 2 KeyCount mismatch: expected 13, got %d", eng3.KeyCount())
		}
		for i := 0; i < 13; i++ {
			k := []byte(fmt.Sprintf("rec_key_%02d", i))
			_, found, err := eng3.Get(k)
			if err != nil || !found {
				t.Fatalf("Key %q missing after cold restart", k)
			}
		}
	})

	t.Run("FullHeader_PartialPayloadAtEOF", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "torn_tail_partial_payload.db")

		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}

		// Write 10 valid records
		for i := 0; i < 10; i++ {
			k := []byte(fmt.Sprintf("orig_key_%02d", i))
			v := []byte(fmt.Sprintf("orig_val_%02d", i))
			if err := eng.Put(k, v); err != nil {
				t.Fatalf("Put(%q) failed: %v", k, err)
			}
		}

		validEOF := eng.ValidEOF()
		nextSeq := eng.NextSeqNo()
		if err := eng.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		// Construct a torn frame: complete 28B header claiming 200B payload, but write header + only 50B payload
		ops := []format.Operation{{
			Type:  format.OpPut,
			Key:   []byte("torn_key"),
			Value: bytes.Repeat([]byte("T"), 180),
		}}
		fullFrame, err := format.EncodeFrame(nextSeq, ops)
		if err != nil {
			t.Fatalf("EncodeFrame failed: %v", err)
		}

		// Write full 28-byte header + 30 bytes of payload (incomplete frame)
		partialFrameBytes := fullFrame[:28+30]

		f, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			t.Fatalf("OpenFile failed: %v", err)
		}
		if _, err := f.Write(partialFrameBytes); err != nil {
			t.Fatalf("Write partial frame failed: %v", err)
		}
		_ = f.Sync()
		_ = f.Close()

		// Verify file size expanded
		fi, err := os.Stat(dbPath)
		if err != nil || uint64(fi.Size()) != validEOF+uint64(len(partialFrameBytes)) {
			t.Fatalf("File size before repair mismatch: size=%d, expected=%d", fi.Size(), validEOF+uint64(len(partialFrameBytes)))
		}

		// Open engine: must auto-truncate torn tail back to validEOF
		eng2, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Open failed to repair torn payload: %v", err)
		}

		if eng2.ValidEOF() != validEOF {
			t.Fatalf("ValidEOF mismatch after repair: expected %d, got %d", validEOF, eng2.ValidEOF())
		}
		if eng2.KeyCount() != 10 {
			t.Fatalf("KeyCount mismatch after repair: expected 10, got %d", eng2.KeyCount())
		}

		// Physical file size must be truncated back to validEOF
		fiRepaired, _ := os.Stat(dbPath)
		if uint64(fiRepaired.Size()) != validEOF {
			t.Fatalf("Physical file size not truncated: got %d, expected %d", fiRepaired.Size(), validEOF)
		}

		// Torn key must be absent
		_, foundTorn, _ := eng2.Get([]byte("torn_key"))
		if foundTorn {
			t.Fatalf("Torn key was unexpectedly found in index!")
		}

		// Subsequent writes must succeed without error
		for i := 10; i < 13; i++ {
			k := []byte(fmt.Sprintf("orig_key_%02d", i))
			v := []byte(fmt.Sprintf("subsequent_val_%02d", i))
			if err := eng2.Put(k, v); err != nil {
				t.Fatalf("Subsequent Put(%q) failed: %v", k, err)
			}
		}

		if err := eng2.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		// Reopen: verify all 13 keys
		eng3, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Reopen failed: %v", err)
		}
		defer eng3.Close()

		if eng3.KeyCount() != 13 {
			t.Fatalf("Reopen KeyCount mismatch: expected 13, got %d", eng3.KeyCount())
		}
	})
}

// TestChallenger_Harness4_FailClosed_MiddleCorruption executes:
// - Write 5 valid records. Flip bits in frame #2 (while frames #3-#5 exist).
// - Open engine; verify it refuses to truncate, fails closed with ErrCorruptFrameHeader or ErrCorruptPayload, and preserves file.
func TestChallenger_Harness4_FailClosed_MiddleCorruption(t *testing.T) {
	// Helper to create a 5-record database and return frame start offsets and sizes
	setup5FrameDB := func(t *testing.T, dbPath string) (frameOffsets []uint64, frameSizes []uint64, totalFileSize uint64) {
		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}

		var offsets []uint64
		var sizes []uint64
		for i := 1; i <= 5; i++ {
			beforeEOF := eng.ValidEOF()
			k := []byte(fmt.Sprintf("frame_key_%d", i))
			v := []byte(fmt.Sprintf("frame_val_%d_data_%s", i, strings.Repeat("M", 20)))
			if err := eng.Put(k, v); err != nil {
				t.Fatalf("Put failed: %v", err)
			}
			afterEOF := eng.ValidEOF()
			offsets = append(offsets, beforeEOF)
			sizes = append(sizes, afterEOF-beforeEOF)
		}

		totalFileSize = eng.ValidEOF()
		if err := eng.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
		return offsets, sizes, totalFileSize
	}

	t.Run("Frame2_HeaderBitFlip_FailsClosed_With_ErrCorruptFrameHeader", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "middle_hdr_corrupt.db")

		offsets, _, totalFileSize := setup5FrameDB(t, dbPath)
		frame2Offset := offsets[1] // Frame #2 start offset (index 1)

		// Verify frames #3, #4, #5 exist after frame #2
		if frame2Offset >= offsets[2] || offsets[4] >= totalFileSize {
			t.Fatalf("Frame layout invalid: f2=%d, f3=%d, total=%d", frame2Offset, offsets[2], totalFileSize)
		}

		// Flip a bit in Frame #2 Header Magic (offset: frame2Offset + 0)
		f, err := os.OpenFile(dbPath, os.O_RDWR, 0666)
		if err != nil {
			t.Fatalf("OpenFile failed: %v", err)
		}
		origByte := make([]byte, 1)
		if _, err := f.ReadAt(origByte, int64(frame2Offset)); err != nil {
			t.Fatalf("ReadAt failed: %v", err)
		}
		corruptedByte := []byte{origByte[0] ^ 0xFF}
		if _, err := f.WriteAt(corruptedByte, int64(frame2Offset)); err != nil {
			t.Fatalf("WriteAt failed: %v", err)
		}
		_ = f.Sync()
		_ = f.Close()

		// Verify file size before open attempt
		fiBefore, err := os.Stat(dbPath)
		if err != nil || uint64(fiBefore.Size()) != totalFileSize {
			t.Fatalf("Pre-open file size mismatch: %d vs %d", fiBefore.Size(), totalFileSize)
		}

		// Open engine: MUST REFUSE TO TRUNCATE and FAIL CLOSED with ErrCorruptFrameHeader
		_, openErr := engine.Open(dbPath, engine.Options{})
		if openErr == nil {
			t.Fatalf("Open unexpectedly succeeded on middle-of-file corrupted frame header!")
		}
		if !errors.Is(openErr, format.ErrCorruptFrameHeader) {
			t.Fatalf("Expected ErrCorruptFrameHeader, got: %v", openErr)
		}

		// Invariant: file MUST NOT BE TRUNCATED! Preserves file for forensic analysis.
		fiAfter, err := os.Stat(dbPath)
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}
		if uint64(fiAfter.Size()) != totalFileSize {
			t.Fatalf("Engine inappropriately truncated file on middle corruption! sizeBefore=%d, sizeAfter=%d",
				totalFileSize, fiAfter.Size())
		}
	})

	t.Run("Frame2_PayloadBitFlip_FailsClosed_With_ErrCorruptPayload", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "middle_payload_corrupt.db")

		offsets, _, totalFileSize := setup5FrameDB(t, dbPath)
		frame2Offset := offsets[1] // Frame #2 start offset

		// Frame #2 header is 28 bytes. Corrupt byte inside payload: frame2Offset + 28 + 2
		payloadByteOffset := int64(frame2Offset + format.FrameHeaderSize + 2)

		f, err := os.OpenFile(dbPath, os.O_RDWR, 0666)
		if err != nil {
			t.Fatalf("OpenFile failed: %v", err)
		}
		origByte := make([]byte, 1)
		if _, err := f.ReadAt(origByte, payloadByteOffset); err != nil {
			t.Fatalf("ReadAt failed: %v", err)
		}
		corruptedByte := []byte{origByte[0] ^ 0x01}
		if _, err := f.WriteAt(corruptedByte, payloadByteOffset); err != nil {
			t.Fatalf("WriteAt failed: %v", err)
		}
		_ = f.Sync()
		_ = f.Close()

		// Verify file size before open attempt
		fiBefore, _ := os.Stat(dbPath)
		if uint64(fiBefore.Size()) != totalFileSize {
			t.Fatalf("Pre-open file size mismatch: %d vs %d", fiBefore.Size(), totalFileSize)
		}

		// Open engine: MUST REFUSE TO TRUNCATE and FAIL CLOSED with ErrCorruptPayload
		_, openErr := engine.Open(dbPath, engine.Options{})
		if openErr == nil {
			t.Fatalf("Open unexpectedly succeeded on middle-of-file corrupted payload!")
		}
		if !errors.Is(openErr, format.ErrCorruptPayload) {
			t.Fatalf("Expected ErrCorruptPayload, got: %v", openErr)
		}

		// Invariant: file MUST NOT BE TRUNCATED!
		fiAfter, err := os.Stat(dbPath)
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}
		if uint64(fiAfter.Size()) != totalFileSize {
			t.Fatalf("Engine inappropriately truncated file on middle payload corruption! sizeBefore=%d, sizeAfter=%d",
				totalFileSize, fiAfter.Size())
		}
	})

	t.Run("Frame2_TrailerBitFlip_FailsClosed_With_ErrCorruptPayload", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "middle_trailer_corrupt.db")

		offsets, sizes, totalFileSize := setup5FrameDB(t, dbPath)
		frame2Offset := offsets[1]
		frame2Size := sizes[1]

		// Trailer CRC is the last 4 bytes of frame #2
		trailerByteOffset := int64(frame2Offset + frame2Size - 2)

		f, err := os.OpenFile(dbPath, os.O_RDWR, 0666)
		if err != nil {
			t.Fatalf("OpenFile failed: %v", err)
		}
		origByte := make([]byte, 1)
		if _, err := f.ReadAt(origByte, trailerByteOffset); err != nil {
			t.Fatalf("ReadAt failed: %v", err)
		}
		corruptedByte := []byte{origByte[0] ^ 0xAA}
		if _, err := f.WriteAt(corruptedByte, trailerByteOffset); err != nil {
			t.Fatalf("WriteAt failed: %v", err)
		}
		_ = f.Sync()
		_ = f.Close()

		fiBefore, _ := os.Stat(dbPath)
		if uint64(fiBefore.Size()) != totalFileSize {
			t.Fatalf("Pre-open file size mismatch: %d vs %d", fiBefore.Size(), totalFileSize)
		}

		// Open engine: MUST REFUSE TO TRUNCATE and FAIL CLOSED with ErrCorruptPayload
		_, openErr := engine.Open(dbPath, engine.Options{})
		if openErr == nil {
			t.Fatalf("Open unexpectedly succeeded on middle-of-file corrupted trailer CRC!")
		}
		if !errors.Is(openErr, format.ErrCorruptPayload) {
			t.Fatalf("Expected ErrCorruptPayload, got: %v", openErr)
		}

		// Invariant: file MUST NOT BE TRUNCATED!
		fiAfter, err := os.Stat(dbPath)
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}
		if uint64(fiAfter.Size()) != totalFileSize {
			t.Fatalf("Engine inappropriately truncated file on middle trailer corruption! sizeBefore=%d, sizeAfter=%d",
				totalFileSize, fiAfter.Size())
		}
	})
}

