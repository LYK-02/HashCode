package engine

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kvdb/internal/format"
)

func TestBatch_EmptyBatch_Rejected(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_empty.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	err = eng.Batch([]format.Operation{})
	if err == nil {
		t.Fatalf("Expected error on empty batch")
	}
	if !errors.Is(err, format.ErrBatchEmpty) {
		t.Errorf("Expected ErrBatchEmpty, got: %v", err)
	}
}

func TestBatch_TooLargeBatch_Rejected(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_large.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	ops := make([]format.Operation, 10001)
	for i := 0; i < 10001; i++ {
		ops[i] = format.Operation{Type: format.OpPut, Key: []byte("k"), Value: []byte("v")}
	}

	err = eng.Batch(ops)
	if err == nil {
		t.Fatalf("Expected error on 10,001 ops batch")
	}
	if !errors.Is(err, format.ErrBatchTooLarge) {
		t.Errorf("Expected ErrBatchTooLarge, got: %v", err)
	}
}

func TestBatch_SingleOp_PutAndDel(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_single.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	// 1-op PUT
	err = eng.Batch([]format.Operation{
		{Type: format.OpPut, Key: []byte("single_key"), Value: []byte("single_val")},
	})
	if err != nil {
		t.Fatalf("1-op batch PUT failed: %v", err)
	}

	val, found, err := eng.Get([]byte("single_key"))
	if err != nil || !found || string(val) != "single_val" {
		t.Fatalf("Get single_key failed: val=%s, found=%v, err=%v", val, found, err)
	}

	// 1-op DEL
	err = eng.Batch([]format.Operation{
		{Type: format.OpDel, Key: []byte("single_key")},
	})
	if err != nil {
		t.Fatalf("1-op batch DEL failed: %v", err)
	}

	_, found, err = eng.Get([]byte("single_key"))
	if err != nil || found {
		t.Fatalf("single_key should be deleted, found=%v", found)
	}
}

func TestBatch_Mixed10Ops_CommittedDurable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_mixed.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Pre-populate some keys
	_ = eng.Put([]byte("pre_1"), []byte("v1"))
	_ = eng.Put([]byte("pre_2"), []byte("v2"))

	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("b_k1"), Value: []byte("val1")},
		{Type: format.OpPut, Key: []byte("b_k2"), Value: []byte("val2")},
		{Type: format.OpDel, Key: []byte("pre_1")},
		{Type: format.OpPut, Key: []byte("b_k3"), Value: []byte("val3")},
		{Type: format.OpPut, Key: []byte("b_k4"), Value: []byte("val4")},
		{Type: format.OpPut, Key: []byte("b_k5"), Value: []byte("val5")},
		{Type: format.OpDel, Key: []byte("pre_2")},
		{Type: format.OpPut, Key: []byte("b_k6"), Value: []byte("val6")},
		{Type: format.OpPut, Key: []byte("b_k7"), Value: []byte("val7")},
		{Type: format.OpPut, Key: []byte("b_k8"), Value: []byte("val8")},
	}

	if err := eng.Batch(ops); err != nil {
		t.Fatalf("Batch failed: %v", err)
	}

	_ = eng.Close()

	// Re-open and verify durability
	eng2, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Re-open failed: %v", err)
	}
	defer eng2.Close()

	// pre_1 and pre_2 must be deleted
	_, found, _ := eng2.Get([]byte("pre_1"))
	if found {
		t.Errorf("pre_1 should be deleted")
	}
	_, found, _ = eng2.Get([]byte("pre_2"))
	if found {
		t.Errorf("pre_2 should be deleted")
	}

	// b_k1 through b_k8 must exist
	for i := 1; i <= 8; i++ {
		k := fmt.Sprintf("b_k%d", i)
		expectedVal := fmt.Sprintf("val%d", i)
		v, f, err := eng2.Get([]byte(k))
		if err != nil || !f || string(v) != expectedVal {
			t.Errorf("Key %s: expected %s, got %s (found=%v, err=%v)", k, expectedVal, v, f, err)
		}
	}
}

func TestBatch_10000Ops_Accepted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 10,000 op batch test in short mode")
	}
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_10k.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	ops := make([]format.Operation, 10000)
	for i := 0; i < 10000; i++ {
		ops[i] = format.Operation{
			Type:  format.OpPut,
			Key:   []byte(fmt.Sprintf("k10k_%06d", i)),
			Value: []byte(fmt.Sprintf("v10k_%06d", i)),
		}
	}

	start := time.Now()
	if err := eng.Batch(ops); err != nil {
		t.Fatalf("10,000 batch failed: %v", err)
	}
	elapsed := time.Since(start)
	t.Logf("10,000 op batch committed in %v", elapsed)

	if eng.KeyCount() != 10000 {
		t.Errorf("Expected 10,000 keys, got %d", eng.KeyCount())
	}

	_ = eng.Close()

	// Reopen and verify recovery time
	recStart := time.Now()
	eng2, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Re-open 10k db failed: %v", err)
	}
	defer eng2.Close()
	recElapsed := time.Since(recStart)
	t.Logf("10,000 keys recovered in %v", recElapsed)

	if recElapsed > 2*time.Second {
		t.Errorf("Recovery exceeded 2.0s: %v", recElapsed)
	}

	// Spot check first and last keys
	vFirst, fFirst, _ := eng2.Get([]byte("k10k_000000"))
	if !fFirst || string(vFirst) != "v10k_000000" {
		t.Errorf("First key mismatch: %s", vFirst)
	}
	vLast, fLast, _ := eng2.Get([]byte("k10k_009999"))
	if !fLast || string(vLast) != "v10k_009999" {
		t.Errorf("Last key mismatch: %s", vLast)
	}
}

func TestBatch_IntraBatch_DuplicatePuts_LastWins(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_dup_puts.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("k"), Value: []byte("v1")},
		{Type: format.OpPut, Key: []byte("k"), Value: []byte("v2")},
		{Type: format.OpPut, Key: []byte("k"), Value: []byte("v3_winner")},
	}

	if err := eng.Batch(ops); err != nil {
		t.Fatalf("Batch failed: %v", err)
	}

	val, found, err := eng.Get([]byte("k"))
	if err != nil || !found || string(val) != "v3_winner" {
		t.Errorf("Expected v3_winner, got %s (found=%v, err=%v)", val, found, err)
	}
}

func TestBatch_IntraBatch_PutThenDel_KeyAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_put_del.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("k"), Value: []byte("transient")},
		{Type: format.OpDel, Key: []byte("k")},
	}

	if err := eng.Batch(ops); err != nil {
		t.Fatalf("Batch failed: %v", err)
	}

	_, found, err := eng.Get([]byte("k"))
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if found {
		t.Errorf("Key should be absent after PUT then DEL in same batch")
	}
}

func TestBatch_IntraBatch_DelThenPut_KeyPresent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_del_put.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	// Prior value exists
	_ = eng.Put([]byte("k"), []byte("old"))

	ops := []format.Operation{
		{Type: format.OpDel, Key: []byte("k")},
		{Type: format.OpPut, Key: []byte("k"), Value: []byte("new_winner")},
	}

	if err := eng.Batch(ops); err != nil {
		t.Fatalf("Batch failed: %v", err)
	}

	val, found, err := eng.Get([]byte("k"))
	if err != nil || !found || string(val) != "new_winner" {
		t.Errorf("Expected new_winner, got %s (found=%v, err=%v)", val, found, err)
	}
}

func TestBatch_IntraBatch_PutDelPut_FinalWins(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_put_del_put.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("k"), Value: []byte("first")},
		{Type: format.OpDel, Key: []byte("k")},
		{Type: format.OpPut, Key: []byte("k"), Value: []byte("final_third")},
	}

	if err := eng.Batch(ops); err != nil {
		t.Fatalf("Batch failed: %v", err)
	}

	val, found, err := eng.Get([]byte("k"))
	if err != nil || !found || string(val) != "final_third" {
		t.Errorf("Expected final_third, got %s (found=%v, err=%v)", val, found, err)
	}
}

func TestBatch_Validation_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_val_err.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	// Invalid OpType
	err = eng.Batch([]format.Operation{
		{Type: 99, Key: []byte("k"), Value: []byte("v")},
	})
	if !errors.Is(err, format.ErrUnknownOpType) {
		t.Errorf("Expected ErrUnknownOpType, got: %v", err)
	}

	// Key Empty
	err = eng.Batch([]format.Operation{
		{Type: format.OpPut, Key: []byte(""), Value: []byte("v")},
	})
	if !errors.Is(err, format.ErrKeyEmpty) {
		t.Errorf("Expected ErrKeyEmpty, got: %v", err)
	}

	// Key Too Large (> 1024)
	err = eng.Batch([]format.Operation{
		{Type: format.OpPut, Key: bytes.Repeat([]byte("a"), 1025), Value: []byte("v")},
	})
	if !errors.Is(err, format.ErrKeyTooLarge) {
		t.Errorf("Expected ErrKeyTooLarge, got: %v", err)
	}

	// Value Too Large (> 1MB)
	err = eng.Batch([]format.Operation{
		{Type: format.OpPut, Key: []byte("k"), Value: bytes.Repeat([]byte("a"), 1048577)},
	})
	if !errors.Is(err, format.ErrValueTooLarge) {
		t.Errorf("Expected ErrValueTooLarge, got: %v", err)
	}

	// Del with non-zero value
	err = eng.Batch([]format.Operation{
		{Type: format.OpDel, Key: []byte("k"), Value: []byte("non_empty")},
	})
	if !errors.Is(err, format.ErrDelNonZeroValue) {
		t.Errorf("Expected ErrDelNonZeroValue, got: %v", err)
	}
}

func TestBatch_Quota_AtomicRejection(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_quota.db")

	// Limit engine to 100 bytes (DB header takes 32 bytes, leaves 68 bytes)
	eng, err := Open(dbPath, Options{MaxBytes: 100})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	// Pre-populate 1 small key that fits
	err = eng.Put([]byte("p"), []byte("v"))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	priorEOF := eng.ValidEOF()

	// Batch that exceeds quota
	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("k1"), Value: bytes.Repeat([]byte("X"), 50)},
		{Type: format.OpPut, Key: []byte("k2"), Value: bytes.Repeat([]byte("Y"), 50)},
	}

	err = eng.Batch(ops)
	if !errors.Is(err, ErrDiskFull) {
		t.Fatalf("Expected ErrDiskFull, got: %v", err)
	}

	// Prior state must be completely untouched!
	if eng.ValidEOF() != priorEOF {
		t.Errorf("ValidEOF modified after rejected batch: expected %d, got %d", priorEOF, eng.ValidEOF())
	}
	if eng.KeyCount() != 1 {
		t.Errorf("KeyCount modified after rejected batch: expected 1, got %d", eng.KeyCount())
	}
	_, found, _ := eng.Get([]byte("k1"))
	if found {
		t.Errorf("k1 should not exist after rejected batch")
	}
}

func TestBatch_CrashRecovery_GroupB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_crash_b1_b6.db")

	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	_ = eng.Put([]byte("prior_k"), []byte("prior_v"))
	validSize := eng.ValidEOF()
	_ = eng.Close()

	// Open raw file to simulate torn write during batch append
	f, err := os.OpenFile(dbPath, os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}

	// Construct batch of 10 operations
	ops := make([]format.Operation, 10)
	for i := 0; i < 10; i++ {
		ops[i] = format.Operation{
			Type:  format.OpPut,
			Key:   []byte(fmt.Sprintf("batch_k%d", i)),
			Value: []byte(fmt.Sprintf("batch_v%d", i)),
		}
	}
	frameBytes, _ := format.EncodeFrame(2, ops)

	// Simulate Scenario B1/B2: partial frame bytes written (say only header + 2 subrecords)
	tornBytes := frameBytes[:format.FrameHeaderSize+20]
	_, _ = f.WriteAt(tornBytes, int64(validSize))
	_ = f.Sync()
	_ = f.Close()

	// Reopen engine -> recovery must truncate torn tail and restore 0 batch operations
	engRecovered, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open after crash failed: %v", err)
	}
	defer engRecovered.Close()

	if engRecovered.ValidEOF() != validSize {
		t.Errorf("ValidEOF mismatch: expected %d, got %d", validSize, engRecovered.ValidEOF())
	}

	// Prior key must be intact
	v, found, err := engRecovered.Get([]byte("prior_k"))
	if err != nil || !found || string(v) != "prior_v" {
		t.Errorf("Prior key corrupted or missing: %s, found=%v", v, found)
	}

	// ZERO of the 10 batch operations must be visible
	for i := 0; i < 10; i++ {
		k := fmt.Sprintf("batch_k%d", i)
		_, found, _ := engRecovered.Get([]byte(k))
		if found {
			t.Errorf("Batch key %s should NOT be visible after torn write", k)
		}
	}
}

func TestBatch_ClosedAndPoisoned(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_closed.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	_ = eng.Close()

	err = eng.Batch([]format.Operation{
		{Type: format.OpPut, Key: []byte("k"), Value: []byte("v")},
	})
	if !errors.Is(err, ErrClosed) {
		t.Errorf("Expected ErrClosed, got: %v", err)
	}
}

func TestBatch_ReaderIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "b_iso.db")
	eng, err := Open(dbPath, Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Reader goroutine continually checking for partial batch states
	var readerObservedPartial bool
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				// Batch puts k1, k2, k3 together. Reader must see either 0 or all 3.
				_, f1, _ := eng.Get([]byte("iso_k1"))
				_, f2, _ := eng.Get([]byte("iso_k2"))
				_, f3, _ := eng.Get([]byte("iso_k3"))

				count := 0
				if f1 {
					count++
				}
				if f2 {
					count++
				}
				if f3 {
					count++
				}

				if count > 0 && count < 3 {
					readerObservedPartial = true
					return
				}
			}
		}
	}()

	// Perform 100 batches
	for b := 0; b < 100; b++ {
		ops := []format.Operation{
			{Type: format.OpPut, Key: []byte("iso_k1"), Value: []byte(fmt.Sprintf("v1_%d", b))},
			{Type: format.OpPut, Key: []byte("iso_k2"), Value: []byte(fmt.Sprintf("v2_%d", b))},
			{Type: format.OpPut, Key: []byte("iso_k3"), Value: []byte(fmt.Sprintf("v3_%d", b))},
		}
		if err := eng.Batch(ops); err != nil {
			t.Fatalf("Batch %d failed: %v", b, err)
		}
	}

	close(stop)
	wg.Wait()

	if readerObservedPartial {
		t.Fatalf("Dirty read observed: reader saw partial subset of atomic batch!")
	}
}
