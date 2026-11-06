package engine_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"kvdb/internal/engine"
	"kvdb/internal/format"
)

func TestEngine_Compaction_ShrinksFileAndIncrementsGen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "compact_basic.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	if eng.CompactionGen() != 0 {
		t.Fatalf("expected initial compaction generation 0, got %d", eng.CompactionGen())
	}

	// Insert 100 keys
	for i := 0; i < 100; i++ {
		k := []byte(fmt.Sprintf("key_%03d", i))
		v := []byte(fmt.Sprintf("val_initial_%03d", i))
		if err := eng.Put(k, v); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Overwrite 50 keys 5 times to generate garbage
	for round := 1; round <= 5; round++ {
		for i := 0; i < 50; i++ {
			k := []byte(fmt.Sprintf("key_%03d", i))
			v := []byte(fmt.Sprintf("val_overwrite_round_%d_%03d", round, i))
			if err := eng.Put(k, v); err != nil {
				t.Fatalf("Put overwrite failed: %v", err)
			}
		}
	}

	// Delete 30 keys (keys 50..79)
	for i := 50; i < 80; i++ {
		k := []byte(fmt.Sprintf("key_%03d", i))
		if err := eng.Delete(k); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
	}

	fiBefore, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("Stat before failed: %v", err)
	}
	sizeBefore := fiBefore.Size()

	// Trigger compaction
	if err := eng.Compact(); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	if eng.CompactionGen() != 1 {
		t.Fatalf("expected compaction generation 1, got %d", eng.CompactionGen())
	}

	fiAfter, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("Stat after failed: %v", err)
	}
	sizeAfter := fiAfter.Size()

	if sizeAfter >= sizeBefore {
		t.Fatalf("Compaction failed to shrink file: before=%d, after=%d", sizeBefore, sizeAfter)
	}
	t.Logf("Compaction shrunk file from %d bytes to %d bytes (%.1f%% reduction)",
		sizeBefore, sizeAfter, float64(sizeBefore-sizeAfter)/float64(sizeBefore)*100)

	// Verify all 70 remaining live keys
	for i := 0; i < 50; i++ {
		k := []byte(fmt.Sprintf("key_%03d", i))
		expectedVal := []byte(fmt.Sprintf("val_overwrite_round_5_%03d", i))
		val, found, err := eng.Get(k)
		if err != nil || !found {
			t.Fatalf("Key %s missing after compaction", k)
		}
		if !bytes.Equal(val, expectedVal) {
			t.Fatalf("Value mismatch for %s: got %s, want %s", k, val, expectedVal)
		}
	}

	// Deleted keys must NOT be found
	for i := 50; i < 80; i++ {
		k := []byte(fmt.Sprintf("key_%03d", i))
		_, found, err := eng.Get(k)
		if err != nil {
			t.Fatalf("Get error for %s: %v", k, err)
		}
		if found {
			t.Fatalf("Deleted key %s unexpectedly found after compaction", k)
		}
	}

	// Keys 80..99 must still have initial values
	for i := 80; i < 100; i++ {
		k := []byte(fmt.Sprintf("key_%03d", i))
		expectedVal := []byte(fmt.Sprintf("val_initial_%03d", i))
		val, found, err := eng.Get(k)
		if err != nil || !found {
			t.Fatalf("Key %s missing after compaction", k)
		}
		if !bytes.Equal(val, expectedVal) {
			t.Fatalf("Value mismatch for %s: got %s, want %s", k, val, expectedVal)
		}
	}

	// Subsequent write after compaction must succeed
	if err := eng.Put([]byte("new_key_post_compact"), []byte("new_val")); err != nil {
		t.Fatalf("Put post compaction failed: %v", err)
	}

	// Close and reopen to verify durability
	if err := eng.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reopened, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Reopen failed: %v", err)
	}
	defer reopened.Close()

	if reopened.CompactionGen() != 1 {
		t.Fatalf("expected reopened compaction generation 1, got %d", reopened.CompactionGen())
	}
	if reopened.KeyCount() != 71 {
		t.Fatalf("expected 71 live keys after reopen, got %d", reopened.KeyCount())
	}
	val, found, err := reopened.Get([]byte("new_key_post_compact"))
	if err != nil || !found || !bytes.Equal(val, []byte("new_val")) {
		t.Fatalf("Post compaction write did not survive reopen")
	}
}

func TestEngine_Compaction_CrashBeforeRename_CleansOrphanTemp(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "compact_crash.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	for i := 0; i < 10; i++ {
		_ = eng.Put([]byte(fmt.Sprintf("k%d", i)), []byte(fmt.Sprintf("v%d", i)))
	}
	_ = eng.Close()

	// Simulate a crash during compaction by manually leaving a .compact.tmp file
	tempPath := dbPath + ".compact.tmp"
	if err := os.WriteFile(tempPath, []byte("partial garbage from killed compaction"), 0666); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	// Reopening database must automatically discard orphan temp file
	reopened, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed with orphan temp present: %v", err)
	}
	defer reopened.Close()

	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("Open failed to clean up orphan compaction temp file %s", tempPath)
	}

	if reopened.KeyCount() != 10 {
		t.Fatalf("expected 10 keys from intact original database, got %d", reopened.KeyCount())
	}
}

func TestEngine_Compaction_EmptyAndAllDeleted(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "compact_empty.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer eng.Close()

	// Compact empty DB
	if err := eng.Compact(); err != nil {
		t.Fatalf("Compact empty DB failed: %v", err)
	}

	fi, _ := os.Stat(dbPath)
	if fi.Size() != format.HeaderSize {
		t.Fatalf("Compacted empty DB size must be exactly %d bytes, got %d", format.HeaderSize, fi.Size())
	}

	// Insert and delete a key
	_ = eng.Put([]byte("temp_k"), []byte("temp_v"))
	_ = eng.Delete([]byte("temp_k"))

	// Compact all-deleted DB
	if err := eng.Compact(); err != nil {
		t.Fatalf("Compact all-deleted DB failed: %v", err)
	}

	fi2, _ := os.Stat(dbPath)
	if fi2.Size() != format.HeaderSize {
		t.Fatalf("Compacted all-deleted DB size must be exactly %d bytes, got %d", format.HeaderSize, fi2.Size())
	}
	if eng.CompactionGen() != 2 {
		t.Fatalf("expected compaction gen 2, got %d", eng.CompactionGen())
	}
}

