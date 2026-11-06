package engine_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"kvdb/internal/engine"
	"kvdb/internal/format"
)

func TestRecovery_1MillionKeys_Under2Seconds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 1M keys scale test in short mode")
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bench_1m.db")

	opts := engine.Options{MaxBytes: 1024 * 1024 * 1024} // 1 GB
	db, err := engine.Open(dbPath, opts)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	const totalKeys = 1_000_000
	const batchSize = 10_000 // 100 batches of 10k ops = 1,000,000 keys
	totalBatches := totalKeys / batchSize

	t.Logf("Writing %d keys in %d batches of %d...", totalKeys, totalBatches, batchSize)
	writeStart := time.Now()

	for b := 0; b < totalBatches; b++ {
		ops := make([]format.Operation, batchSize)
		for i := 0; i < batchSize; i++ {
			keyIdx := b*batchSize + i
			k := fmt.Sprintf("k%08d", keyIdx)
			ops[i] = format.Operation{
				Type:  format.OpPut,
				Key:   []byte(k),
				Value: []byte("val"),
			}
		}
		if err := db.Batch(ops); err != nil {
			t.Fatalf("Batch %d failed: %v", b, err)
		}
	}
	writeDuration := time.Since(writeStart)
	t.Logf("Finished writing 1M keys in %v", writeDuration)

	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Now measure recovery time upon Open()
	t.Log("Beginning Recovery Benchmark for 1,000,000 keys...")
	recStart := time.Now()
	dbRecovered, err := engine.Open(dbPath, opts)
	recDuration := time.Since(recStart)

	if err != nil {
		t.Fatalf("Open/Recovery of 1M keys failed: %v", err)
	}
	defer dbRecovered.Close()

	t.Logf(">>> 1,000,000 keys recovery completed in %v <<<", recDuration)

	// Verify sample keys
	val0, exists, err := dbRecovered.Get([]byte("k00000000"))
	if err != nil || !exists || string(val0) != "val" {
		t.Fatalf("Sample key 0 mismatch: exists=%v val=%s err=%v", exists, string(val0), err)
	}

	valEnd, exists, err := dbRecovered.Get([]byte(fmt.Sprintf("k%08d", totalKeys-1)))
	if err != nil || !exists || string(valEnd) != "val" {
		t.Fatalf("Sample key %d mismatch: exists=%v val=%s err=%v", totalKeys-1, exists, string(valEnd), err)
	}

	// Assert under 2.0s
	if recDuration > 2*time.Second {
		t.Fatalf("FAILURE: Recovery took %v, which exceeds the mandatory 2.0s limit!", recDuration)
	}
	t.Logf("SUCCESS: Recovery was %v (< 2.0s)", recDuration)
}
