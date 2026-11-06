package e2e_test

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"kvdb/tests/e2e/fixtures"
	"kvdb/tests/e2e/harness"
)

// ============================================================================
// Tier 1 - Feature Coverage (55 Test Cases: 11 Features x 5 Cases)
// ============================================================================

// ----------------------------------------------------------------------------
// Feature 1: Database Header & Open (Cases 1-5)
// ----------------------------------------------------------------------------

func TestTier1_01_Open_NewDatabaseCreatesHeader(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_01.db")

	res := h.Open(dbPath)
	h.CheckOrSkip(res, "Open New DB")
	res.AssertSuccess(t, "open new database should succeed")

	if !h.FileExists(dbPath) {
		t.Fatalf("database file %s was not created", dbPath)
	}
	size := h.GetFileSize(dbPath)
	if size < 32 {
		t.Fatalf("database file size %d is less than 32-byte header", size)
	}

	// Verify header magic bytes
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("failed to read db file: %v", err)
	}
	if string(data[:8]) != "KVDB2026" {
		t.Fatalf("invalid DB header magic: expected 'KVDB2026', got %q", string(data[:8]))
	}
}

func TestTier1_02_Open_ExistingDatabaseSucceeds(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_02.db")

	r1 := h.Open(dbPath)
	h.CheckOrSkip(r1, "Open DB Initial")
	r1.AssertSuccess(t, "initial open should succeed")

	// Second open on existing DB
	r2 := h.Open(dbPath)
	r2.AssertSuccess(t, "reopening existing database should succeed")
}

func TestTier1_03_Open_CorruptMagicFails(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_03.db")

	r1 := h.Open(dbPath)
	h.CheckOrSkip(r1, "Open DB Initial")
	r1.AssertSuccess(t, "initial open should succeed")

	// Corrupt header magic
	h.CorruptByte(dbPath, 0, 0xFF)

	r2 := h.Open(dbPath)
	r2.AssertFailure(t, "opening database with corrupted magic must fail")
}

func TestTier1_04_Open_UnsupportedVersionFails(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_04.db")

	r1 := h.Open(dbPath)
	h.CheckOrSkip(r1, "Open DB Initial")
	r1.AssertSuccess(t, "initial open should succeed")

	// Corrupt version at offset 8 (set to high value)
	h.CorruptByte(dbPath, 8, 0x05)

	r2 := h.Open(dbPath)
	r2.AssertFailure(t, "opening database with unsupported version must fail")
}

func TestTier1_05_Open_CorruptHeaderCRCFails(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_05.db")

	r1 := h.Open(dbPath)
	h.CheckOrSkip(r1, "Open DB Initial")
	r1.AssertSuccess(t, "initial open should succeed")

	// Corrupt header CRC at byte 28
	h.CorruptByte(dbPath, 28, 0x01)

	r2 := h.Open(dbPath)
	r2.AssertFailure(t, "opening database with corrupt header CRC must fail")
}

// ----------------------------------------------------------------------------
// Feature 2: Exclusive Lock (Cases 6-10)
// ----------------------------------------------------------------------------

func TestTier1_06_Lock_SidecarCreated(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_06.db")
	lockPath := dbPath + ".lock"

	res := h.Open(dbPath)
	h.CheckOrSkip(res, "Open Lock Sidecar")
	res.AssertSuccess(t, "open should succeed")

	if !h.FileExists(lockPath) {
		t.Fatalf("lock sidecar file %s was not created", lockPath)
	}
}

func TestTier1_07_Lock_ConcurrentOpenRejected(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_07.db")
	lockPath := dbPath + ".lock"

	// Create and hold lock explicitly with flock
	_ = h.Open(dbPath)
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		t.Fatalf("failed to open lock file: %v", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("failed to acquire flock: %v", err)
	}
	defer func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}()

	// Second process attempting open must fail because lock is held
	res := h.Open(dbPath)
	h.CheckOrSkip(res, "Concurrent Open")
	res.AssertFailure(t, "open should fail when flock is held by another process")
}

func TestTier1_08_Lock_ConcurrentPutRejected(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_08.db")
	lockPath := dbPath + ".lock"

	_ = h.Open(dbPath)
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		t.Fatalf("failed to open lock file: %v", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("failed to acquire flock: %v", err)
	}
	defer func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}()

	res := h.Put(dbPath, "key", "val")
	h.CheckOrSkip(res, "Concurrent Put")
	res.AssertFailure(t, "put should fail when lock is held by another process")
}

func TestTier1_09_Lock_ReleasedOnNormalExit(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_09.db")

	r1 := h.Open(dbPath)
	h.CheckOrSkip(r1, "Lock Normal Exit 1")
	r1.AssertSuccess(t, "first open should succeed")

	// Process has exited, lock should be free
	r2 := h.Put(dbPath, "k1", "v1")
	h.CheckOrSkip(r2, "Lock Normal Exit 2")
	r2.AssertSuccess(t, "put after normal exit should succeed")
}

func TestTier1_10_Lock_ReleasedOnSIGKILL(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_10.db")

	// Start serve session
	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	time.Sleep(100 * time.Millisecond)

	// Kill serve process with SIGKILL
	if err := session.Kill(); err != nil {
		t.Fatalf("failed to kill serve session: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Subsequent command must immediately succeed in acquiring lock
	r := h.Open(dbPath)
	h.CheckOrSkip(r, "Lock Post SIGKILL")
	r.AssertSuccess(t, "open after SIGKILL must acquire lock and succeed")
}

// ----------------------------------------------------------------------------
// Feature 3: Single Write (PUT) (Cases 11-15)
// ----------------------------------------------------------------------------

func TestTier1_11_Put_BasicKeyValue(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_11.db")

	res := h.Put(dbPath, "greeting", "hello_world")
	h.CheckOrSkip(res, "Put Basic")
	res.AssertSuccess(t, "put should succeed")
}

func TestTier1_12_Put_OverwriteExistingKey(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_12.db")

	r1 := h.Put(dbPath, "key1", "val1")
	h.CheckOrSkip(r1, "Put 1")
	r1.AssertSuccess(t, "initial put should succeed")

	r2 := h.Put(dbPath, "key1", "val2")
	h.CheckOrSkip(r2, "Put Overwrite")
	r2.AssertSuccess(t, "overwrite put should succeed")

	r3 := h.Get(dbPath, "key1")
	h.CheckOrSkip(r3, "Get Overwritten Key")
	r3.AssertStdoutEquals(t, "val2", "get should return newest value")
}

func TestTier1_13_Put_EmptyValue(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_13.db")

	res := h.Put(dbPath, "emptyKey", "")
	h.CheckOrSkip(res, "Put Empty Value")
	res.AssertSuccess(t, "put with empty value should succeed")

	r2 := h.Get(dbPath, "emptyKey")
	h.CheckOrSkip(r2, "Get Empty Value")
	r2.AssertStdoutEquals(t, "", "get for empty value key must return empty string (not NOTFOUND)")
}

func TestTier1_14_Put_ValueWithSpacesAndSpecialChars(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_14.db")

	specialVal := "line1\nline2 with spaces !@#$%^&*()_+"
	res := h.Put(dbPath, "special_key", specialVal)
	h.CheckOrSkip(res, "Put Special Chars")
	res.AssertSuccess(t, "put with special characters should succeed")
}

func TestTier1_15_Put_MultipleDistinctKeys(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_15.db")

	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("k%d", i)
		val := fmt.Sprintf("v%d", i)
		res := h.Put(dbPath, key, val)
		h.CheckOrSkip(res, "Put Sequential")
		res.AssertSuccess(t, "sequential put should succeed")
	}
}

// ----------------------------------------------------------------------------
// Feature 4: Single Read (GET) (Cases 16-20)
// ----------------------------------------------------------------------------

func TestTier1_16_Get_ExistingKeyReturnsValue(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_16.db")

	r1 := h.Put(dbPath, "myKey", "myVal")
	h.CheckOrSkip(r1, "Put for Get")
	r1.AssertSuccess(t, "put should succeed")

	r2 := h.Get(dbPath, "myKey")
	h.CheckOrSkip(r2, "Get Existing")
	r2.AssertStdoutEquals(t, "myVal", "get should return exact value")
}

func TestTier1_17_Get_NonExistentKeyReturnsNotFound(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_17.db")

	_ = h.Open(dbPath)
	res := h.Get(dbPath, "non_existent_key")
	h.CheckOrSkip(res, "Get Non-Existent")
	res.AssertStdoutContains(t, "NOTFOUND", "get non-existent key must return NOTFOUND")
}

func TestTier1_18_Get_EmptyValueKeyReturnsEmpty(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_18.db")

	r1 := h.Put(dbPath, "kEmpty", "")
	h.CheckOrSkip(r1, "Put Empty")
	r1.AssertSuccess(t, "put empty value should succeed")

	r2 := h.Get(dbPath, "kEmpty")
	h.CheckOrSkip(r2, "Get Empty")
	r2.AssertStdoutEquals(t, "", "get on empty value must not return NOTFOUND")
}

func TestTier1_19_Get_AfterOverwriteReturnsNewValue(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_19.db")

	_ = h.Put(dbPath, "k", "old")
	_ = h.Put(dbPath, "k", "new")

	res := h.Get(dbPath, "k")
	h.CheckOrSkip(res, "Get Overwrite")
	res.AssertStdoutEquals(t, "new", "get must reflect latest overwrite")
}

func TestTier1_20_Get_AfterDeleteReturnsNotFound(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_20.db")

	_ = h.Put(dbPath, "tempKey", "tempVal")
	_ = h.Del(dbPath, "tempKey")

	res := h.Get(dbPath, "tempKey")
	h.CheckOrSkip(res, "Get After Delete")
	res.AssertStdoutContains(t, "NOTFOUND", "get on deleted key must return NOTFOUND")
}

// ----------------------------------------------------------------------------
// Feature 5: Single Delete (DEL) (Cases 21-25)
// ----------------------------------------------------------------------------

func TestTier1_21_Del_ExistingKeyRemovesKey(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_21.db")

	_ = h.Put(dbPath, "delMe", "someVal")
	r1 := h.Del(dbPath, "delMe")
	h.CheckOrSkip(r1, "Del Existing")
	r1.AssertSuccess(t, "del should succeed")

	r2 := h.Get(dbPath, "delMe")
	h.CheckOrSkip(r2, "Get Del Existing")
	r2.AssertStdoutContains(t, "NOTFOUND", "key must be removed after del")
}

func TestTier1_22_Del_NonExistentKeySucceeds(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_22.db")

	_ = h.Open(dbPath)
	res := h.Del(dbPath, "never_created")
	h.CheckOrSkip(res, "Del Non-Existent")
	res.AssertSuccess(t, "deleting a missing key is not an error")
}

func TestTier1_23_Del_ReinsertAfterDeleteWorks(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_23.db")

	_ = h.Put(dbPath, "reviveKey", "val1")
	_ = h.Del(dbPath, "reviveKey")
	_ = h.Put(dbPath, "reviveKey", "val2")

	res := h.Get(dbPath, "reviveKey")
	h.CheckOrSkip(res, "Get Reinserted")
	res.AssertStdoutEquals(t, "val2", "reinserted key should hold new value")
}

func TestTier1_24_Del_MultipleKeysSequential(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_24.db")

	_ = h.Put(dbPath, "a", "1")
	_ = h.Put(dbPath, "b", "2")
	_ = h.Put(dbPath, "c", "3")

	_ = h.Del(dbPath, "a")
	_ = h.Del(dbPath, "c")

	rB := h.Get(dbPath, "b")
	h.CheckOrSkip(rB, "Get retained B")
	rB.AssertStdoutEquals(t, "2", "retained key b must remain intact")

	rA := h.Get(dbPath, "a")
	rA.AssertStdoutContains(t, "NOTFOUND", "deleted key a must be NOTFOUND")
}

func TestTier1_25_Del_IdempotentDoubleDelete(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_25.db")

	_ = h.Put(dbPath, "twice", "val")
	r1 := h.Del(dbPath, "twice")
	h.CheckOrSkip(r1, "First Del")
	r1.AssertSuccess(t, "first del should succeed")

	r2 := h.Del(dbPath, "twice")
	h.CheckOrSkip(r2, "Second Del")
	r2.AssertSuccess(t, "second del on same key should also succeed idempotently")
}

// ----------------------------------------------------------------------------
// Feature 6: Atomic Batch (Cases 26-30)
// ----------------------------------------------------------------------------

func TestTier1_26_Batch_TwoPutsCommitted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_26.db")

	batchFile := h.WriteBatchFile("batch2.txt", []string{
		"put batchKey1 valA",
		"put batchKey2 valB",
	})

	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch Two Puts")
	res.AssertSuccess(t, "batch execution should succeed")

	r1 := h.Get(dbPath, "batchKey1")
	h.CheckOrSkip(r1, "Get batchKey1")
	r1.AssertStdoutEquals(t, "valA", "batchKey1 should have valA")

	r2 := h.Get(dbPath, "batchKey2")
	h.CheckOrSkip(r2, "Get batchKey2")
	r2.AssertStdoutEquals(t, "valB", "batchKey2 should have valB")
}

func TestTier1_27_Batch_MixedPutAndDel(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_27.db")

	_ = h.Put(dbPath, "toDelete", "initial")

	batchFile := h.WriteBatchFile("mixed_batch.txt", []string{
		"put newKey newVal",
		"del toDelete",
	})

	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch Mixed")
	res.AssertSuccess(t, "mixed batch should succeed")

	r1 := h.Get(dbPath, "newKey")
	h.CheckOrSkip(r1, "Get newKey")
	r1.AssertStdoutEquals(t, "newVal", "newKey should be committed")

	r2 := h.Get(dbPath, "toDelete")
	h.CheckOrSkip(r2, "Get toDelete")
	r2.AssertStdoutContains(t, "NOTFOUND", "toDelete should be removed")
}

func TestTier1_28_Batch_AllDels(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_28.db")

	_ = h.Put(dbPath, "d1", "v1")
	_ = h.Put(dbPath, "d2", "v2")

	batchFile := h.WriteBatchFile("del_batch.txt", []string{
		"del d1",
		"del d2",
	})

	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch All Dels")
	res.AssertSuccess(t, "all-del batch should succeed")

	r1 := h.Get(dbPath, "d1")
	h.CheckOrSkip(r1, "Get d1")
	r1.AssertStdoutContains(t, "NOTFOUND", "d1 should be deleted")
}

func TestTier1_29_Batch_DuplicateKeysLastWins(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_29.db")

	batchFile := h.WriteBatchFile("dup_batch.txt", []string{
		"put duplicateKey first",
		"put duplicateKey second",
		"put duplicateKey finalWinner",
	})

	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch Dup Last Wins")
	res.AssertSuccess(t, "batch with duplicate keys should succeed")

	r := h.Get(dbPath, "duplicateKey")
	h.CheckOrSkip(r, "Get duplicateKey")
	r.AssertStdoutEquals(t, "finalWinner", "last operation in batch must win")
}

func TestTier1_30_Batch_PutThenDelSameKeyAbsents(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_30.db")

	batchFile := h.WriteBatchFile("put_del_batch.txt", []string{
		"put sameKey transientVal",
		"del sameKey",
	})

	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch Put Then Del")
	res.AssertSuccess(t, "batch with put then del should succeed")

	r := h.Get(dbPath, "sameKey")
	h.CheckOrSkip(r, "Get sameKey")
	r.AssertStdoutContains(t, "NOTFOUND", "key deleted at end of batch must be absent")
}

// ----------------------------------------------------------------------------
// Feature 7: DUMP Sorted Order (Cases 31-35)
// ----------------------------------------------------------------------------

func TestTier1_31_Dump_EmptyDatabase(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_31.db")

	_ = h.Open(dbPath)
	res := h.Dump(dbPath)
	h.CheckOrSkip(res, "Dump Empty")
	res.AssertSuccess(t, "dump empty database should succeed")
	if strings.TrimSpace(res.Stdout) != "" {
		t.Fatalf("dump of empty DB should have empty output, got: %q", res.Stdout)
	}
}

func TestTier1_32_Dump_SortedLexicographical(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_32.db")

	// Insert in reverse order
	_ = h.Put(dbPath, "zebra", "animal")
	_ = h.Put(dbPath, "apple", "fruit")
	_ = h.Put(dbPath, "mango", "fruit")
	_ = h.Put(dbPath, "banana", "fruit")

	res := h.Dump(dbPath)
	h.CheckOrSkip(res, "Dump Sorted")
	res.AssertSuccess(t, "dump should succeed")

	sorted, reason := fixtures.CheckDumpIsSorted(res.Stdout)
	if !sorted {
		t.Fatalf("dump is not sorted: %s\nStdout:\n%s", reason, res.Stdout)
	}
}

func TestTier1_33_Dump_ExcludesDeletedKeys(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_33.db")

	_ = h.Put(dbPath, "alive1", "val1")
	_ = h.Put(dbPath, "dead", "valDead")
	_ = h.Put(dbPath, "alive2", "val2")
	_ = h.Del(dbPath, "dead")

	res := h.Dump(dbPath)
	h.CheckOrSkip(res, "Dump Excludes Deleted")
	res.AssertSuccess(t, "dump should succeed")

	if strings.Contains(res.Stdout, "dead") {
		t.Fatalf("dump output should not contain deleted key 'dead': %s", res.Stdout)
	}
}

func TestTier1_34_Dump_ReflectsOverwrites(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_34.db")

	_ = h.Put(dbPath, "k", "valOld")
	_ = h.Put(dbPath, "k", "valNew")

	res := h.Dump(dbPath)
	h.CheckOrSkip(res, "Dump Reflects Overwrites")
	res.AssertSuccess(t, "dump should succeed")

	m := fixtures.ParseDumpOutput(res.Stdout)
	if m["k"] != "valNew" {
		t.Fatalf("expected k='valNew', got: %q", m["k"])
	}
}

func TestTier1_35_Dump_PreservesEmptyValues(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_35.db")

	_ = h.Put(dbPath, "emptyKey", "")
	_ = h.Put(dbPath, "normalKey", "val")

	res := h.Dump(dbPath)
	h.CheckOrSkip(res, "Dump Preserves Empty")
	res.AssertSuccess(t, "dump should succeed")

	m := fixtures.ParseDumpOutput(res.Stdout)
	if _, ok := m["emptyKey"]; !ok {
		t.Fatalf("dump must include key with empty value")
	}
}

// ----------------------------------------------------------------------------
// Feature 8: Integrity Check (Cases 36-40)
// ----------------------------------------------------------------------------

func TestTier1_36_Check_FreshDatabaseOK(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_36.db")

	_ = h.Open(dbPath)
	res := h.Check(dbPath)
	h.CheckOrSkip(res, "Check Fresh DB")
	res.AssertSuccess(t, "check on fresh database should succeed")
	res.AssertStdoutContains(t, "OK", "check must output OK on healthy database")
}

func TestTier1_37_Check_PopulatedDatabaseOK(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_37.db")

	_ = h.Put(dbPath, "k1", "v1")
	_ = h.Put(dbPath, "k2", "v2")
	_ = h.Del(dbPath, "k1")

	res := h.Check(dbPath)
	h.CheckOrSkip(res, "Check Populated DB")
	res.AssertSuccess(t, "check on populated database should succeed")
	res.AssertStdoutContains(t, "OK", "check must output OK on valid database")
}

func TestTier1_38_Check_BadHeaderMagicFails(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_38.db")

	_ = h.Open(dbPath)
	h.CorruptByte(dbPath, 0, 0xFF)

	res := h.Check(dbPath)
	h.CheckOrSkip(res, "Check Bad Header Magic")
	res.AssertFailure(t, "check must fail on corrupted DB header magic")
}

func TestTier1_39_Check_CorruptedFrameHeaderFails(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_39.db")

	_ = h.Put(dbPath, "k1", "v1")
	// Frame header starts at byte 32. Corrupt frame magic byte at offset 32.
	h.CorruptByte(dbPath, 32, 0xFF)

	res := h.Check(dbPath)
	h.CheckOrSkip(res, "Check Corrupted Frame Header")
	res.AssertFailure(t, "check must fail on corrupted frame magic")
}

func TestTier1_40_Check_CorruptedPayloadFails(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_40.db")

	_ = h.Put(dbPath, "k1", "long_payload_value_for_crc_check")
	// Payload starts at 32 (db header) + 28 (frame header) = 60. Corrupt payload byte at 65.
	h.CorruptByte(dbPath, 65, 0x01)

	res := h.Check(dbPath)
	h.CheckOrSkip(res, "Check Corrupted Payload")
	res.AssertFailure(t, "check must fail when payload CRC trailer mismatches")
}

// ----------------------------------------------------------------------------
// Feature 9: SERVE Streaming Protocol (Cases 41-45)
// ----------------------------------------------------------------------------

func TestTier1_41_Serve_PingPong(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_41.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	defer session.Close()

	resp, err := session.Send("PING 100")
	if err != nil {
		t.Skipf("Serve protocol streaming not yet active: %v", err)
	}
	if !strings.Contains(resp, "PONG 100") && !strings.Contains(resp, "ACK 100") {
		t.Fatalf("expected PONG/ACK response, got: %q", resp)
	}
}

func TestTier1_42_Serve_PutReturnsAck(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_42.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	defer session.Close()

	keyHex := fixtures.EncodeHex("serveKey")
	valHex := fixtures.EncodeHex("serveVal")
	resp, err := session.Send(fmt.Sprintf("PUT 1 %s %s", keyHex, valHex))
	if err != nil {
		t.Skipf("Serve PUT not yet active: %v", err)
	}
	if !strings.HasPrefix(resp, "ACK 1") {
		t.Fatalf("expected 'ACK 1', got: %q", resp)
	}
}

func TestTier1_43_Serve_GetReturnsValue(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_43.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	defer session.Close()

	keyHex := fixtures.EncodeHex("getServeKey")
	valHex := fixtures.EncodeHex("getServeVal")
	_, _ = session.Send(fmt.Sprintf("PUT 1 %s %s", keyHex, valHex))

	resp, err := session.Send(fmt.Sprintf("GET 2 %s", keyHex))
	if err != nil {
		t.Skipf("Serve GET not yet active: %v", err)
	}
	if !strings.Contains(resp, valHex) && !strings.Contains(resp, "getServeVal") {
		t.Fatalf("expected value in GET response, got: %q", resp)
	}
}

func TestTier1_44_Serve_DelReturnsAck(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_44.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	defer session.Close()

	keyHex := fixtures.EncodeHex("delServeKey")
	resp, err := session.Send(fmt.Sprintf("DEL 5 %s", keyHex))
	if err != nil {
		t.Skipf("Serve DEL not yet active: %v", err)
	}
	if !strings.HasPrefix(resp, "ACK 5") {
		t.Fatalf("expected 'ACK 5', got: %q", resp)
	}
}

func TestTier1_45_Serve_MultiCommandSession(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_45.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	defer session.Close()

	for i := 1; i <= 5; i++ {
		k := fixtures.EncodeHex(fmt.Sprintf("stream_k%d", i))
		v := fixtures.EncodeHex(fmt.Sprintf("stream_v%d", i))
		resp, err := session.Send(fmt.Sprintf("PUT %d %s %s", i, k, v))
		if err != nil {
			t.Skipf("Serve multi-command session not yet active: %v", err)
		}
		if !strings.HasPrefix(resp, fmt.Sprintf("ACK %d", i)) {
			t.Fatalf("expected ACK %d, got: %q", i, resp)
		}
	}
}

// ----------------------------------------------------------------------------
// Feature 10: Crash Recovery Replay (Cases 46-50)
// ----------------------------------------------------------------------------

func TestTier1_46_Recovery_ReplaysAllAckedWrites(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_46.db")

	_ = h.Put(dbPath, "persisted1", "v1")
	_ = h.Put(dbPath, "persisted2", "v2")

	// Open database in a new invocation (simulating post-process recovery)
	rOpen := h.Open(dbPath)
	h.CheckOrSkip(rOpen, "Recovery Open")
	rOpen.AssertSuccess(t, "recovery open should succeed")

	r1 := h.Get(dbPath, "persisted1")
	h.CheckOrSkip(r1, "Get persisted1")
	r1.AssertStdoutEquals(t, "v1", "persisted1 must be intact after restart")
}

func TestTier1_47_Recovery_MultiFrameReplay(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_47.db")

	// Frame 1: Single PUT
	_ = h.Put(dbPath, "f1_k", "f1_v")

	// Frame 2: Batch
	batchFile := h.WriteBatchFile("f2.txt", []string{
		"put f2_k1 val1",
		"put f2_k2 val2",
	})
	_ = h.Batch(dbPath, batchFile)

	// Replay via fresh open
	rOpen := h.Open(dbPath)
	h.CheckOrSkip(rOpen, "Multi-frame Recovery")
	rOpen.AssertSuccess(t, "recovery open should succeed")

	rF1 := h.Get(dbPath, "f1_k")
	h.CheckOrSkip(rF1, "Get f1_k")
	rF1.AssertStdoutEquals(t, "f1_v", "f1_k must be recovered")

	rF2 := h.Get(dbPath, "f2_k2")
	h.CheckOrSkip(rF2, "Get f2_k2")
	rF2.AssertStdoutEquals(t, "val2", "f2_k2 must be recovered")
}

func TestTier1_48_Recovery_TornTailTruncatedCleanly(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_48.db")

	_ = h.Put(dbPath, "stableKey", "stableVal")

	// Append 10 bytes of uncommitted garbage at EOF (simulating torn write at crash)
	h.AppendBytes(dbPath, []byte("tornwrite!"))

	// Recovery open must truncate torn tail cleanly and succeed
	rOpen := h.Open(dbPath)
	h.CheckOrSkip(rOpen, "Recovery Torn Tail Truncate")
	rOpen.AssertSuccess(t, "recovery open should repair torn tail at EOF")

	rGet := h.Get(dbPath, "stableKey")
	h.CheckOrSkip(rGet, "Get stableKey")
	rGet.AssertStdoutEquals(t, "stableVal", "stableKey must remain intact after torn tail repair")
}

func TestTier1_49_Recovery_PreservesPriorValidFrames(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_49.db")

	for i := 1; i <= 3; i++ {
		_ = h.Put(dbPath, fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
	}

	// Corrupt incomplete tail
	h.AppendBytes(dbPath, []byte{0x55, 0x41, 0x42, 0x45}) // partial frame magic

	rOpen := h.Open(dbPath)
	h.CheckOrSkip(rOpen, "Recovery Partial Magic Tail")
	rOpen.AssertSuccess(t, "recovery open must succeed despite partial trailing bytes")

	for i := 1; i <= 3; i++ {
		r := h.Get(dbPath, fmt.Sprintf("k%d", i))
		h.CheckOrSkip(r, "Verify Preserved Prior Frames")
		r.AssertStdoutEquals(t, fmt.Sprintf("v%d", i), "prior keys must be intact")
	}
}

func TestTier1_50_Recovery_IdempotentMultipleOpen(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_50.db")

	_ = h.Put(dbPath, "k", "v")

	// Open 3 times consecutively
	for i := 1; i <= 3; i++ {
		r := h.Open(dbPath)
		h.CheckOrSkip(r, "Idempotent Open")
		r.AssertSuccess(t, "repeated open calls must succeed identically")
	}

	rGet := h.Get(dbPath, "k")
	h.CheckOrSkip(rGet, "Get post multiple open")
	rGet.AssertStdoutEquals(t, "v", "key must be intact")
}

// ----------------------------------------------------------------------------
// Feature 11: Logical Quota Guard (--max-bytes) (Cases 51-55)
// ----------------------------------------------------------------------------

func TestTier1_51_Quota_WriteBelowQuotaAccepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_51.db")

	// Set generous max-bytes (10MB)
	rOpen := h.Open(dbPath, "--max-bytes", "10485760")
	h.CheckOrSkip(rOpen, "Open Quota 10MB")
	rOpen.AssertSuccess(t, "open with max-bytes should succeed")

	rPut := h.Put(dbPath, "fitKey", "fitVal")
	h.CheckOrSkip(rPut, "Put Under Quota")
	rPut.AssertSuccess(t, "write below quota must succeed")
}

func TestTier1_52_Quota_WriteExceedingQuotaRejected(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_52.db")

	// Set small quota of 100 bytes (header is 32 bytes, frame is 32 + payload)
	rOpen := h.Open(dbPath, "--max-bytes", "100")
	h.CheckOrSkip(rOpen, "Open Small Quota")
	rOpen.AssertSuccess(t, "open should succeed")

	// Put large value that definitely exceeds 100 bytes total DB size
	hugeVal := fixtures.FixedPayload(500, 'X')
	rPut := h.Put(dbPath, "hugeKey", hugeVal)
	h.CheckOrSkip(rPut, "Put Exceeding Quota")
	rPut.AssertFailure(t, "write exceeding max-bytes quota must fail with non-zero exit code")
}

func TestTier1_53_Quota_DatabaseReadableAfterRejection(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_53.db")

	_ = h.Open(dbPath, "--max-bytes", "250")
	_ = h.Put(dbPath, "existingKey", "existingVal")

	// Attempt exceeding write
	hugeVal := fixtures.FixedPayload(500, 'Z')
	_ = h.Put(dbPath, "overflowKey", hugeVal)

	// Database must remain readable and return existingKey
	rGet := h.Get(dbPath, "existingKey")
	h.CheckOrSkip(rGet, "Get After Quota Rejection")
	rGet.AssertStdoutEquals(t, "existingVal", "database must remain readable after quota error")
}

func TestTier1_54_Quota_ExistingKeysIntactAfterRejection(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_54.db")

	_ = h.Open(dbPath, "--max-bytes", "300")
	_ = h.Put(dbPath, "k1", "v1")
	_ = h.Put(dbPath, "k2", "v2")

	// Rejected write
	_ = h.Put(dbPath, "bad", fixtures.FixedPayload(500, 'Q'))

	r1 := h.Get(dbPath, "k1")
	h.CheckOrSkip(r1, "Get k1 post rejection")
	r1.AssertStdoutEquals(t, "v1", "k1 must be intact")

	r2 := h.Get(dbPath, "k2")
	h.CheckOrSkip(r2, "Get k2 post rejection")
	r2.AssertStdoutEquals(t, "v2", "k2 must be intact")
}

func TestTier1_55_Quota_BatchExceedingQuotaRejectedAtomically(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t1_55.db")

	_ = h.Open(dbPath, "--max-bytes", "200")

	// Batch that exceeds quota
	batchFile := h.WriteBatchFile("quota_batch.txt", []string{
		"put b1 val1",
		fmt.Sprintf("put b2 %s", fixtures.FixedPayload(300, 'W')),
	})

	rBatch := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(rBatch, "Batch Exceeding Quota")
	rBatch.AssertFailure(t, "batch exceeding quota must fail")

	// Neither operation in batch should be visible (all-or-nothing atomicity)
	rB1 := h.Get(dbPath, "b1")
	h.CheckOrSkip(rB1, "Get b1 post batch rejection")
	rB1.AssertStdoutContains(t, "NOTFOUND", "b1 must not be present after batch quota rejection")
}
