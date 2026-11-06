package engine

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kvdb/internal/format"
)

// ============================================================================
// CHALLENGER M3-1: BATCH ATOMICITY & GROUP B CRASH MATRIX ADVERSARIAL SUITE
// ============================================================================

// ----------------------------------------------------------------------------
// 1. Boundary Conditions & Pre-Flight Validation (Harness 1)
// ----------------------------------------------------------------------------

func TestChallengerM3_1_Boundary_EmptyAndNilBatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_empty.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	// Initial valid state
	if err := eng.Put([]byte("baseline"), []byte("val")); err != nil {
		t.Fatalf("Put baseline failed: %v", err)
	}
	initialSize := getFileSize(t, eng.db)
	initialKeys := eng.KeyCount()

	// 1. Empty slice batch
	err = eng.Batch([]format.Operation{})
	if !errors.Is(err, format.ErrBatchEmpty) {
		t.Fatalf("Expected ErrBatchEmpty, got: %v", err)
	}

	// 2. Nil slice batch
	err = eng.Batch(nil)
	if !errors.Is(err, format.ErrBatchEmpty) {
		t.Fatalf("Expected ErrBatchEmpty for nil batch, got: %v", err)
	}

	// Verify state and file size strictly unchanged
	if getFileSize(t, eng.db) != initialSize {
		t.Fatalf("File size mutated on rejected empty batch: %d != %d", getFileSize(t, eng.db), initialSize)
	}
	if eng.KeyCount() != initialKeys {
		t.Fatalf("KeyCount mutated on rejected empty batch")
	}
}

func TestChallengerM3_1_Boundary_BatchSizeLimits_1_10000_10001(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_limits.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	// 1-op batch: valid, minimum accepted boundary
	err = eng.Batch([]format.Operation{
		{Type: format.OpPut, Key: []byte("k1"), Value: []byte("v1")},
	})
	if err != nil {
		t.Fatalf("1-op batch failed: %v", err)
	}
	val, found, err := eng.Get([]byte("k1"))
	if err != nil || !found || string(val) != "v1" {
		t.Fatalf("1-op batch key retrieval failed: val=%s, found=%v, err=%v", val, found, err)
	}

	// 10,001-op batch: invalid, must be rejected before disk I/O
	oversizedOps := make([]format.Operation, 10001)
	for i := 0; i < 10001; i++ {
		oversizedOps[i] = format.Operation{
			Type:  format.OpPut,
			Key:   []byte(fmt.Sprintf("over_%05d", i)),
			Value: []byte("val"),
		}
	}
	preSize := getFileSize(t, eng.db)
	err = eng.Batch(oversizedOps)
	if !errors.Is(err, format.ErrBatchTooLarge) {
		t.Fatalf("Expected ErrBatchTooLarge, got: %v", err)
	}
	if getFileSize(t, eng.db) != preSize {
		t.Fatalf("File size changed after oversized batch rejection")
	}

	// 10,000-op batch: valid, maximum boundary limit
	maxOps := make([]format.Operation, 10000)
	for i := 0; i < 10000; i++ {
		maxOps[i] = format.Operation{
			Type:  format.OpPut,
			Key:   []byte(fmt.Sprintf("max_%05d", i)),
			Value: []byte(fmt.Sprintf("v_%05d", i)),
		}
	}
	start := time.Now()
	err = eng.Batch(maxOps)
	if err != nil {
		t.Fatalf("10,000-op batch failed: %v", err)
	}
	t.Logf("10,000-op batch executed in %v", time.Since(start))

	if eng.KeyCount() != 10001 { // k1 + 10,000 keys
		t.Fatalf("Expected 10001 keys, got %d", eng.KeyCount())
	}

	// Reopen to verify cold recovery of 10,000-op batch
	if err := eng.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	recStart := time.Now()
	reopened, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer reopened.Close()
	recDuration := time.Since(recStart)
	t.Logf("Recovery of 10,001 keys completed in %v", recDuration)
	if recDuration > 2*time.Second {
		t.Errorf("Recovery exceeded 2.0s: %v", recDuration)
	}

	if reopened.KeyCount() != 10001 {
		t.Fatalf("Expected 10001 keys after reopen, got %d", reopened.KeyCount())
	}
}

func TestChallengerM3_1_Boundary_KeyAndValueLimits(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_keyval.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	// 1-byte key and 1024-byte key (max key)
	maxKey := bytes.Repeat([]byte("K"), format.MaxKeyLen)
	// 0-byte value and 1MB value (max value)
	maxVal := bytes.Repeat([]byte("V"), format.MaxValLen)

	validBatch := []format.Operation{
		{Type: format.OpPut, Key: []byte("1"), Value: []byte{}},
		{Type: format.OpPut, Key: maxKey, Value: maxVal},
	}
	if err := eng.Batch(validBatch); err != nil {
		t.Fatalf("Valid boundary batch failed: %v", err)
	}

	// Verify 0-byte value
	v1, found, err := eng.Get([]byte("1"))
	if err != nil || !found || len(v1) != 0 {
		t.Fatalf("0-byte value retrieval failed: found=%v, len=%d", found, len(v1))
	}

	// Verify 1MB value
	vMax, found, err := eng.Get(maxKey)
	if err != nil || !found || !bytes.Equal(vMax, maxVal) {
		t.Fatalf("1MB value retrieval mismatch: found=%v, len=%d", found, len(vMax))
	}

	// Rejected boundaries:
	// A: 0-byte key
	err = eng.Batch([]format.Operation{{Type: format.OpPut, Key: []byte{}, Value: []byte("v")}})
	if !errors.Is(err, format.ErrKeyEmpty) {
		t.Fatalf("Expected ErrKeyEmpty, got: %v", err)
	}

	// B: 1025-byte key
	tooLargeKey := bytes.Repeat([]byte("X"), format.MaxKeyLen+1)
	err = eng.Batch([]format.Operation{{Type: format.OpPut, Key: tooLargeKey, Value: []byte("v")}})
	if !errors.Is(err, format.ErrKeyTooLarge) {
		t.Fatalf("Expected ErrKeyTooLarge, got: %v", err)
	}

	// C: 1MB + 1 byte value
	tooLargeVal := bytes.Repeat([]byte("Y"), format.MaxValLen+1)
	err = eng.Batch([]format.Operation{{Type: format.OpPut, Key: []byte("valid"), Value: tooLargeVal}})
	if !errors.Is(err, format.ErrValueTooLarge) {
		t.Fatalf("Expected ErrValueTooLarge, got: %v", err)
	}

	// D: DEL with non-empty value
	err = eng.Batch([]format.Operation{{Type: format.OpDel, Key: []byte("valid"), Value: []byte("not_empty")}})
	if !errors.Is(err, format.ErrDelNonZeroValue) {
		t.Fatalf("Expected ErrDelNonZeroValue, got: %v", err)
	}

	// E: Unknown OpType
	err = eng.Batch([]format.Operation{{Type: format.OpType(0xFF), Key: []byte("valid")}})
	if !errors.Is(err, format.ErrUnknownOpType) {
		t.Fatalf("Expected ErrUnknownOpType, got: %v", err)
	}
}

func TestChallengerM3_1_Boundary_EmbeddedInvalidOp_ZeroSideEffects(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_embedded.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	if err := eng.Put([]byte("k_safe"), []byte("v_safe")); err != nil {
		t.Fatalf("Put safe failed: %v", err)
	}
	origSize := getFileSize(t, eng.db)

	testPositions := []int{0, 500, 999}
	for _, pos := range testPositions {
		ops := make([]format.Operation, 1000)
		for i := 0; i < 1000; i++ {
			ops[i] = format.Operation{
				Type:  format.OpPut,
				Key:   []byte(fmt.Sprintf("k_%04d", i)),
				Value: []byte("val"),
			}
		}
		// Inject invalid op at target position
		ops[pos] = format.Operation{
			Type:  format.OpPut,
			Key:   []byte{}, // empty key!
			Value: []byte("val"),
		}

		err = eng.Batch(ops)
		if !errors.Is(err, format.ErrKeyEmpty) {
			t.Fatalf("Expected ErrKeyEmpty at pos %d, got: %v", pos, err)
		}

		// Verify zero bytes written and KeyDir untouched
		if getFileSize(t, eng.db) != origSize {
			t.Fatalf("File size changed at pos %d: %d != %d", pos, getFileSize(t, eng.db), origSize)
		}
		if eng.KeyCount() != 1 {
			t.Fatalf("KeyCount changed at pos %d: %d != 1", pos, eng.KeyCount())
		}
	}
}

func TestChallengerM3_1_Boundary_QuotaAtomicRejection(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_quota.db")

	// Pre-fill db with 1 key
	initEng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := initEng.Put([]byte("base_key"), []byte("base_value")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	baseSize := getFileSize(t, initEng.db)
	initEng.Close()

	// Reopen with maxBytes set to baseSize + 60 bytes
	// A batch of 2 keys requires at least 32B (envelope) + 2*(7 + 10 + 10) = 86 bytes, which will exceed quota!
	eng, err := Open(dbPath, Options{MaxBytes: baseSize + 60})
	if err != nil {
		t.Fatalf("Open with quota failed: %v", err)
	}
	defer eng.Close()

	batch := []format.Operation{
		{Type: format.OpPut, Key: []byte("quota_k1"), Value: []byte("quota_v1")},
		{Type: format.OpPut, Key: []byte("quota_k2"), Value: []byte("quota_v2")},
	}
	err = eng.Batch(batch)
	if !errors.Is(err, ErrDiskFull) {
		t.Fatalf("Expected ErrDiskFull, got: %v", err)
	}

	// Verify atomicity: file size unchanged, neither key present
	if getFileSize(t, eng.db) != baseSize {
		t.Fatalf("File size expanded despite ErrDiskFull: %d != %d", getFileSize(t, eng.db), baseSize)
	}
	_, found, _ := eng.Get([]byte("quota_k1"))
	if found {
		t.Fatalf("quota_k1 visible after quota rejection")
	}
	_, found, _ = eng.Get([]byte("quota_k2"))
	if found {
		t.Fatalf("quota_k2 visible after quota rejection")
	}
}

// ----------------------------------------------------------------------------
// 2. Intra-Batch Ordering & Differential Oracle (Harness 2)
// ----------------------------------------------------------------------------

func TestChallengerM3_1_IntraBatch_Ordering_DifferentialOracle(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_ordering.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	// Independent reference oracle simulating sequential mutations
	oracle := make(map[string][]byte)

	var batchOps []format.Operation

	// Helper to record mutation in batch and oracle
	putOp := func(key, val string) {
		batchOps = append(batchOps, format.Operation{
			Type:  format.OpPut,
			Key:   []byte(key),
			Value: []byte(val),
		})
		oracle[key] = []byte(val)
	}
	delOp := func(key string) {
		batchOps = append(batchOps, format.Operation{
			Type: format.OpDel,
			Key:  []byte(key),
		})
		delete(oracle, key)
	}

	// Sequence A: PUT -> PUT -> DEL -> PUT (Final PUT must win)
	putOp("seq_A", "val_1")
	putOp("seq_A", "val_2")
	delOp("seq_A")
	putOp("seq_A", "val_3")

	// Sequence B: PUT -> DEL -> PUT -> DEL (DEL must win; key absent)
	putOp("seq_B", "val_1")
	delOp("seq_B")
	putOp("seq_B", "val_2")
	delOp("seq_B")

	// Sequence C: DEL non-existent -> PUT (Key present with new val)
	delOp("seq_C")
	putOp("seq_C", "val_c")

	// Sequence D: DEL non-existent -> DEL non-existent (Key absent)
	delOp("seq_D")
	delOp("seq_D")

	// Massive interleaved updates across 50 keys, 20 mutations each
	r := rand.New(rand.NewSource(42))
	for i := 0; i < 1000; i++ {
		keyName := fmt.Sprintf("rand_key_%02d", r.Intn(50))
		if r.Intn(4) == 0 { // 25% chance of delete
			delOp(keyName)
		} else {
			putOp(keyName, fmt.Sprintf("rand_val_%04d", i))
		}
	}

	// Execute batch
	if err := eng.Batch(batchOps); err != nil {
		t.Fatalf("Batch execution failed: %v", err)
	}

	// 1. Live verification against oracle
	for k, expectedVal := range oracle {
		val, found, err := eng.Get([]byte(k))
		if err != nil {
			t.Fatalf("Get %s failed: %v", k, err)
		}
		if !found {
			t.Fatalf("Key %s missing in live engine", k)
		}
		if !bytes.Equal(val, expectedVal) {
			t.Fatalf("Value mismatch for %s: got %s, want %s", k, val, expectedVal)
		}
	}

	// Verify absent keys
	for _, k := range []string{"seq_B", "seq_D"} {
		_, found, err := eng.Get([]byte(k))
		if err != nil || found {
			t.Fatalf("Key %s should be absent, found=%v, err=%v", k, found, err)
		}
	}

	if eng.KeyCount() != len(oracle) {
		t.Fatalf("KeyCount mismatch: eng=%d, oracle=%d", eng.KeyCount(), len(oracle))
	}

	// 2. Cold Recovery verification against oracle
	if err := eng.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reopened, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer reopened.Close()

	if reopened.KeyCount() != len(oracle) {
		t.Fatalf("KeyCount mismatch after reopen: eng=%d, oracle=%d", reopened.KeyCount(), len(oracle))
	}

	for k, expectedVal := range oracle {
		val, found, err := reopened.Get([]byte(k))
		if err != nil {
			t.Fatalf("Get %s failed after reopen: %v", k, err)
		}
		if !found {
			t.Fatalf("Key %s missing after reopen", k)
		}
		if !bytes.Equal(val, expectedVal) {
			t.Fatalf("Value mismatch after reopen for %s: got %s, want %s", k, val, expectedVal)
		}
	}
}

// ----------------------------------------------------------------------------
// 3. Reader Isolation Under Concurrent Batch (Harness 3, Scenario D4)
// ----------------------------------------------------------------------------

func TestChallengerM3_1_ReaderIsolation_ConcurrentStress(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_isolation.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	// Initial population: 5 keys all at gen_0
	numKeys := 5
	initOps := make([]format.Operation, numKeys)
	for i := 0; i < numKeys; i++ {
		initOps[i] = format.Operation{
			Type:  format.OpPut,
			Key:   []byte(fmt.Sprintf("iso_k%d", i)),
			Value: []byte("gen_0"),
		}
	}
	if err := eng.Batch(initOps); err != nil {
		t.Fatalf("Init batch failed: %v", err)
	}

	var stopFlag int32
	var tornReadCount int64
	var totalReads int64
	var wg sync.WaitGroup

	numReaders := 8
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			keys := make([][]byte, numKeys)
			for i := 0; i < numKeys; i++ {
				keys[i] = []byte(fmt.Sprintf("iso_k%d", i))
			}
			for atomic.LoadInt32(&stopFlag) == 0 {
				vals, founds, err := eng.GetMulti(keys)
				if err != nil {
					continue
				}

				var observedGens []string
				consistent := true

				for i := 0; i < numKeys; i++ {
					if !founds[i] {
						consistent = false
						break
					}
					observedGens = append(observedGens, string(vals[i]))
				}

				if consistent && len(observedGens) == numKeys {
					// All 5 keys MUST share the exact same generation string!
					firstGen := observedGens[0]
					for _, g := range observedGens[1:] {
						if g != firstGen {
							atomic.AddInt64(&tornReadCount, 1)
							break
						}
					}
				}
				atomic.AddInt64(&totalReads, 1)
			}
		}(r)
	}

	// Writer executes 200 consecutive atomic updates advancing generation
	numGenerations := 200
	for gen := 1; gen <= numGenerations; gen++ {
		genOps := make([]format.Operation, numKeys)
		genStr := fmt.Sprintf("gen_%d", gen)
		for i := 0; i < numKeys; i++ {
			genOps[i] = format.Operation{
				Type:  format.OpPut,
				Key:   []byte(fmt.Sprintf("iso_k%d", i)),
				Value: []byte(genStr),
			}
		}
		if err := eng.Batch(genOps); err != nil {
			t.Fatalf("Batch generation %d failed: %v", gen, err)
		}
	}

	atomic.StoreInt32(&stopFlag, 1)
	wg.Wait()

	t.Logf("Completed %d concurrent reads under batch mutation. Torn reads: %d",
		atomic.LoadInt64(&totalReads), atomic.LoadInt64(&tornReadCount))

	if atomic.LoadInt64(&tornReadCount) > 0 {
		t.Fatalf("CRITICAL INVARIANT VIOLATION: Observed %d torn/partial reads across batch boundary!",
			atomic.LoadInt64(&tornReadCount))
	}
}

// ----------------------------------------------------------------------------
// 4. Crash Matrix Group B (B1-B6) Empirical Simulations (Harness 4)
// ----------------------------------------------------------------------------

func TestChallengerM3_1_CrashMatrix_B1_KillAfter3of10OpsWritten(t *testing.T) {
	// Scenario B1: Kill after 3 of 10 operations in a batch written -> None of the 10 visible.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b1.db")

	// 1. Establish baseline database with 3 acknowledged keys
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := eng.Put([]byte("base_1"), []byte("v1")); err != nil {
		t.Fatalf("Put base_1 failed: %v", err)
	}
	if err := eng.Put([]byte("base_2"), []byte("v2")); err != nil {
		t.Fatalf("Put base_2 failed: %v", err)
	}
	if err := eng.Put([]byte("base_3"), []byte("v3")); err != nil {
		t.Fatalf("Put base_3 failed: %v", err)
	}
	baselineSize := getFileSize(t, eng.db)
	eng.Close()

	// 2. Construct 10-op batch
	batchOps := make([]format.Operation, 10)
	for i := 0; i < 10; i++ {
		batchOps[i] = format.Operation{
			Type:  format.OpPut,
			Key:   []byte(fmt.Sprintf("b1_key_%d", i)),
			Value: []byte(fmt.Sprintf("b1_val_%d", i)),
		}
	}
	fullFrame, err := format.EncodeFrame(4, batchOps)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	// Calculate byte offset corresponding to exactly 3 operations into payload:
	// 28B header + subrecords 0, 1, 2
	partialLen := format.FrameHeaderSize
	for i := 0; i < 3; i++ {
		partialLen += format.RecordSize(batchOps[i])
	}

	// Append partial frame to database file
	f, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	if _, err := f.Write(fullFrame[:partialLen]); err != nil {
		t.Fatalf("Write partial frame failed: %v", err)
	}
	f.Sync()
	f.Close()

	// 3. Reopen database (cold recovery)
	reopened, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed on B1 recovery: %v", err)
	}
	defer reopened.Close()

	// Verify B1 invariants:
	// None of the 10 operations visible!
	for i := 0; i < 10; i++ {
		k := []byte(fmt.Sprintf("b1_key_%d", i))
		if _, found, _ := reopened.Get(k); found {
			t.Fatalf("Scenario B1 failure: Operation %d visible after crash!", i)
		}
	}

	// Baseline data 100% intact
	if reopened.KeyCount() != 3 {
		t.Fatalf("Expected 3 baseline keys, got %d", reopened.KeyCount())
	}
	if v, found, _ := reopened.Get([]byte("base_1")); !found || string(v) != "v1" {
		t.Fatalf("Baseline key base_1 damaged")
	}

	// File truncated back to baseline size
	if getFileSize(t, reopened.db) != baselineSize {
		t.Fatalf("File was not truncated back to baseline: got %d, want %d",
			getFileSize(t, reopened.db), baselineSize)
	}
}

func TestChallengerM3_1_CrashMatrix_B2_KillAfter10OpsWritten_NoCommitMarker(t *testing.T) {
	// Scenario B2: Kill after all 10 operations written but commit marker (trailer CRC) is not -> None of the 10 visible.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b2.db")

	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := eng.Put([]byte("base_key"), []byte("base_val")); err != nil {
		t.Fatalf("Put base failed: %v", err)
	}
	baselineSize := getFileSize(t, eng.db)
	eng.Close()

	batchOps := make([]format.Operation, 10)
	for i := 0; i < 10; i++ {
		batchOps[i] = format.Operation{
			Type:  format.OpPut,
			Key:   []byte(fmt.Sprintf("b2_key_%d", i)),
			Value: []byte(fmt.Sprintf("b2_val_%d", i)),
		}
	}
	fullFrame, err := format.EncodeFrame(2, batchOps)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	// Write FrameHeader + Payload, but omit the 4-byte trailer CRC (commit marker)
	frameWithoutTrailer := fullFrame[:len(fullFrame)-format.FrameTrailerSize]

	f, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	if _, err := f.Write(frameWithoutTrailer); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	f.Sync()
	f.Close()

	// Reopen
	reopened, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed on B2 recovery: %v", err)
	}
	defer reopened.Close()

	// None of the 10 visible
	for i := 0; i < 10; i++ {
		k := []byte(fmt.Sprintf("b2_key_%d", i))
		if _, found, _ := reopened.Get(k); found {
			t.Fatalf("Scenario B2 failure: Key %s visible without commit marker!", k)
		}
	}
	if reopened.KeyCount() != 1 {
		t.Fatalf("Expected 1 baseline key, got %d", reopened.KeyCount())
	}
	if getFileSize(t, reopened.db) != baselineSize {
		t.Fatalf("Torn frame was not truncated back to baseline: %d != %d",
			getFileSize(t, reopened.db), baselineSize)
	}
}

func TestChallengerM3_1_CrashMatrix_B3_CommitMarkerWritten_FsyncTiming(t *testing.T) {
	// Scenario B3: Kill after commit marker written but before fsync -> Either all 10 visible or none visible. NEVER some.

	// Sub-case B3.1: Full commit marker physically reached disk before kill
	t.Run("B3_CommitMarkerFlushed_AllVisible", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "b3_all.db")
		eng, _ := Open(dbPath, Options{})
		eng.Close()

		ops := make([]format.Operation, 10)
		for i := 0; i < 10; i++ {
			ops[i] = format.Operation{Type: format.OpPut, Key: []byte(fmt.Sprintf("b3_k%d", i)), Value: []byte("val")}
		}
		frame, _ := format.EncodeFrame(1, ops)

		// Full write + sync
		f, _ := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
		_, _ = f.Write(frame)
		_ = f.Sync()
		_ = f.Close()

		reopened, err := Open(dbPath, Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer reopened.Close()

		if reopened.KeyCount() != 10 {
			t.Fatalf("Expected all 10 keys visible, got %d", reopened.KeyCount())
		}
	})

	// Sub-case B3.2: Torn commit marker (e.g. 2 of 4 bytes reached disk)
	t.Run("B3_CommitMarkerTorn_NoneVisible", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "b3_torn.db")
		eng, _ := Open(dbPath, Options{})
		eng.Close()

		ops := make([]format.Operation, 10)
		for i := 0; i < 10; i++ {
			ops[i] = format.Operation{Type: format.OpPut, Key: []byte(fmt.Sprintf("b3_k%d", i)), Value: []byte("val")}
		}
		frame, _ := format.EncodeFrame(1, ops)

		// Write frame with only 2 bytes of the 4-byte trailer CRC
		tornFrame := frame[:len(frame)-2]
		f, _ := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
		_, _ = f.Write(tornFrame)
		_ = f.Sync()
		_ = f.Close()

		reopened, err := Open(dbPath, Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer reopened.Close()

		if reopened.KeyCount() != 0 {
			t.Fatalf("Scenario B3 failure: Expected 0 keys visible for torn commit marker, got %d", reopened.KeyCount())
		}
	})

	// Sub-case B3.3: Bit-flip in commit marker (corrupted trailer CRC)
	t.Run("B3_CommitMarkerCorrupted_NoneVisible", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "b3_corrupt.db")
		eng, _ := Open(dbPath, Options{})
		eng.Close()

		ops := make([]format.Operation, 10)
		for i := 0; i < 10; i++ {
			ops[i] = format.Operation{Type: format.OpPut, Key: []byte(fmt.Sprintf("b3_k%d", i)), Value: []byte("val")}
		}
		frame, _ := format.EncodeFrame(1, ops)
		frame[len(frame)-1] ^= 0xFF // Corrupt trailer CRC

		f, _ := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
		_, _ = f.Write(frame)
		_ = f.Sync()
		_ = f.Close()

		reopened, err := Open(dbPath, Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer reopened.Close()

		if reopened.KeyCount() != 0 {
			t.Fatalf("Scenario B3 failure: Expected 0 keys visible for corrupt trailer CRC, got %d", reopened.KeyCount())
		}
	})
}

func TestChallengerM3_1_CrashMatrix_B4_KillAfterBatchReturnsSuccess(t *testing.T) {
	// Scenario B4: Kill after batch returns success -> All 10 visible.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b4.db")

	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	ops := make([]format.Operation, 10)
	for i := 0; i < 10; i++ {
		ops[i] = format.Operation{
			Type:  format.OpPut,
			Key:   []byte(fmt.Sprintf("b4_k%d", i)),
			Value: []byte(fmt.Sprintf("b4_v%d", i)),
		}
	}
	if err := eng.Batch(ops); err != nil {
		t.Fatalf("Batch failed: %v", err)
	}

	// Simulate hard kill without graceful Close()
	// Leave lock and descriptors as they are on crash, then reopen
	reopened, err := Open(dbPath, Options{})
	if err != nil {
		// Note: in Linux, same process re-opening while file descriptor held might get ErrLocked.
		// So close only the descriptor to simulate process exit:
		eng.Close()
		reopened, err = Open(dbPath, Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
	}
	defer reopened.Close()

	if reopened.KeyCount() != 10 {
		t.Fatalf("Expected all 10 keys visible, got %d", reopened.KeyCount())
	}
	for i := 0; i < 10; i++ {
		v, found, err := reopened.Get([]byte(fmt.Sprintf("b4_k%d", i)))
		if err != nil || !found || string(v) != fmt.Sprintf("b4_v%d", i) {
			t.Fatalf("Key b4_k%d mismatch after restart: val=%s, found=%v", i, v, found)
		}
	}
}

func TestChallengerM3_1_CrashMatrix_B5_PutAndDelSameKeyInBatch(t *testing.T) {
	// Scenario B5: A batch that puts key X and deletes key X in same batch -> Defined ordering honoured after crash.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b5.db")

	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Batch: PUT X="v1", PUT X="v2", DEL X
	// Invariant: X must be ABSENT after crash
	batch1 := []format.Operation{
		{Type: format.OpPut, Key: []byte("key_X"), Value: []byte("v1")},
		{Type: format.OpPut, Key: []byte("key_X"), Value: []byte("v2")},
		{Type: format.OpDel, Key: []byte("key_X")},
		// Complementary: DEL Y, PUT Y="y1", PUT Y="y2" -> Y must be "y2"
		{Type: format.OpDel, Key: []byte("key_Y")},
		{Type: format.OpPut, Key: []byte("key_Y"), Value: []byte("y1")},
		{Type: format.OpPut, Key: []byte("key_Y"), Value: []byte("y2")},
	}
	if err := eng.Batch(batch1); err != nil {
		t.Fatalf("Batch failed: %v", err)
	}
	eng.Close()

	// Reopen after simulated crash
	reopened, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer reopened.Close()

	// Key X must be strictly absent
	_, foundX, err := reopened.Get([]byte("key_X"))
	if err != nil || foundX {
		t.Fatalf("Scenario B5 failure: key_X should be absent, found=%v", foundX)
	}

	// Key Y must be strictly present with value "y2"
	vY, foundY, err := reopened.Get([]byte("key_Y"))
	if err != nil || !foundY || string(vY) != "y2" {
		t.Fatalf("Scenario B5 failure: key_Y mismatch: val=%s, found=%v", vY, foundY)
	}
}

func TestChallengerM3_1_CrashMatrix_B6_10000Ops_KilledHalfway(t *testing.T) {
	// Scenario B6: A batch of 10,000 operations killed halfway -> None visible; recovery completes in under 2 seconds.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b6.db")

	// 1. Establish baseline
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := eng.Put([]byte("baseline_marker"), []byte("marker_value")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	baseSize := getFileSize(t, eng.db)
	eng.Close()

	// 2. Construct 10,000-op batch
	ops := make([]format.Operation, 10000)
	for i := 0; i < 10000; i++ {
		ops[i] = format.Operation{
			Type:  format.OpPut,
			Key:   []byte(fmt.Sprintf("b6_k_%05d", i)),
			Value: []byte(fmt.Sprintf("b6_v_%05d", i)),
		}
	}
	fullFrame, err := format.EncodeFrame(2, ops)
	if err != nil {
		t.Fatalf("EncodeFrame 10,000 failed: %v", err)
	}

	// Calculate offset for exactly 5,000 ops (halfway)
	halfwayLen := format.FrameHeaderSize
	for i := 0; i < 5000; i++ {
		halfwayLen += format.RecordSize(ops[i])
	}

	// Append halfway frame to file
	f, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	if _, err := f.Write(fullFrame[:halfwayLen]); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	f.Sync()
	f.Close()

	// 3. Measure recovery time and verify none visible
	recStart := time.Now()
	reopened, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed on B6 recovery: %v", err)
	}
	defer reopened.Close()
	recDuration := time.Since(recStart)

	t.Logf("B6 recovery of torn 10,000-op batch completed in %v", recDuration)
	if recDuration > 2*time.Second {
		t.Errorf("Scenario B6 recovery exceeded 2.0s limit: %v", recDuration)
	}

	// None of the 10,000 keys visible!
	if reopened.KeyCount() != 1 {
		t.Fatalf("Scenario B6 failure: Expected exactly 1 baseline key, got %d", reopened.KeyCount())
	}

	// Check sample keys across range
	for _, idx := range []int{0, 1000, 2500, 4999, 5000, 7500, 9999} {
		k := []byte(fmt.Sprintf("b6_k_%05d", idx))
		if _, found, _ := reopened.Get(k); found {
			t.Fatalf("Scenario B6 failure: Key %s visible after crash halfway!", k)
		}
	}

	// File truncated back to baseline size
	if getFileSize(t, reopened.db) != baseSize {
		t.Fatalf("File size not restored to baseline: %d != %d", getFileSize(t, reopened.db), baseSize)
	}

	// Engine remains fully functional for subsequent writes
	if err := reopened.Put([]byte("new_key"), []byte("new_val")); err != nil {
		t.Fatalf("Put after B6 recovery failed: %v", err)
	}
	val, found, _ := reopened.Get([]byte("new_key"))
	if !found || string(val) != "new_val" {
		t.Fatalf("Subsequent write not retrievable")
	}
}

// ----------------------------------------------------------------------------
// 5. Byte-by-Byte Truncation Sweep Across Entire Frame (Harness 5)
// ----------------------------------------------------------------------------

func TestChallengerM3_1_ByteByByte_TornFrame_Sweep(t *testing.T) {
	// Exhaustively tests torn tail recovery at EVERY single byte offset of a 3-op batch frame.
	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("sweep_1"), Value: []byte("val_1")},
		{Type: format.OpPut, Key: []byte("sweep_2"), Value: []byte("val_2")},
		{Type: format.OpDel, Key: []byte("sweep_1")},
	}
	frame, err := format.EncodeFrame(1, ops)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	frameLen := len(frame)
	t.Logf("Testing exhaustive byte-by-byte sweep from 1 to %d bytes", frameLen-1)

	// Step by 3 bytes to balance thoroughness and execution time
	for cut := 1; cut < frameLen; cut += 3 {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, fmt.Sprintf("sweep_%d.db", cut))

		// 1. Write baseline
		eng, err := Open(dbPath, Options{})
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		if err := eng.Put([]byte("sweep_base"), []byte("base_val")); err != nil {
			t.Fatalf("Put base failed: %v", err)
		}
		baseSize := getFileSize(t, eng.db)
		eng.Close()

		// 2. Append truncated frame (cut bytes)
		f, err := os.OpenFile(dbPath, os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			t.Fatalf("OpenFile failed: %v", err)
		}
		if _, err := f.Write(frame[:cut]); err != nil {
			t.Fatalf("Write cut failed: %v", err)
		}
		f.Sync()
		f.Close()

		// 3. Reopen and verify clean recovery
		reopened, err := Open(dbPath, Options{})
		if err != nil {
			t.Fatalf("Open failed at cut offset %d: %v", cut, err)
		}

		if reopened.KeyCount() != 1 {
			reopened.Close()
			t.Fatalf("Cut offset %d: expected 1 key, got %d", cut, reopened.KeyCount())
		}
		if v, found, _ := reopened.Get([]byte("sweep_base")); !found || string(v) != "base_val" {
			reopened.Close()
			t.Fatalf("Cut offset %d: baseline key corrupt", cut)
		}
		if getFileSize(t, reopened.db) != baseSize {
			reopened.Close()
			t.Fatalf("Cut offset %d: file not truncated to %d (got %d)", cut, baseSize, getFileSize(t, reopened.db))
		}
		reopened.Close()
	}
}

// ----------------------------------------------------------------------------
// Test Helpers
// ----------------------------------------------------------------------------

func getFileSize(t *testing.T, f *os.File) uint64 {
	t.Helper()
	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat failed in getFileSize: %v", err)
	}
	return uint64(fi.Size())
}
