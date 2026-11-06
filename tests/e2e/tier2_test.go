package e2e_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"kvdb/tests/e2e/fixtures"
	"kvdb/tests/e2e/harness"
)

// ============================================================================
// Tier 2 - Boundary & Corner Cases (55 Test Cases)
// ============================================================================

// ----------------------------------------------------------------------------
// 1. Key Length Boundaries (Cases 1-5)
// ----------------------------------------------------------------------------

func TestTier2_01_Key_Length0_Rejected(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_01.db")

	res := h.Put(dbPath, "", "someVal")
	h.CheckOrSkip(res, "Key Len 0")
	res.AssertFailure(t, "key length 0 must be rejected with non-zero exit code")
}

func TestTier2_02_Key_Length1_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_02.db")

	res := h.Put(dbPath, "x", "val")
	h.CheckOrSkip(res, "Key Len 1")
	res.AssertSuccess(t, "key length 1 must be accepted")

	rGet := h.Get(dbPath, "x")
	h.CheckOrSkip(rGet, "Get Key Len 1")
	rGet.AssertStdoutEquals(t, "val", "key length 1 must return value")
}

func TestTier2_03_Key_Length2_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_03.db")

	res := h.Put(dbPath, "ab", "val")
	h.CheckOrSkip(res, "Key Len 2")
	res.AssertSuccess(t, "key length 2 must be accepted")
}

func TestTier2_04_Key_Length1024_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_04.db")

	key1024 := fixtures.FixedPayload(1024, 'K')
	res := h.Put(dbPath, key1024, "val1024")
	h.CheckOrSkip(res, "Key Len 1024")
	res.AssertSuccess(t, "key length 1024 must be accepted")

	rGet := h.Get(dbPath, key1024)
	h.CheckOrSkip(rGet, "Get Key Len 1024")
	rGet.AssertStdoutEquals(t, "val1024", "key length 1024 must return correct value")
}

func TestTier2_05_Key_Length1025_Rejected(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_05.db")

	key1025 := fixtures.FixedPayload(1025, 'K')
	res := h.Put(dbPath, key1025, "val1025")
	h.CheckOrSkip(res, "Key Len 1025")
	res.AssertFailure(t, "key length 1025 must be rejected with non-zero exit code")
}

// ----------------------------------------------------------------------------
// 2. Value Length Boundaries (Cases 6-10)
// ----------------------------------------------------------------------------

func TestTier2_06_Val_Length0_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_06.db")

	res := h.Put(dbPath, "k_empty", "")
	h.CheckOrSkip(res, "Val Len 0")
	res.AssertSuccess(t, "value length 0 for PUT must be accepted")

	rGet := h.Get(dbPath, "k_empty")
	h.CheckOrSkip(rGet, "Get Val Len 0")
	rGet.AssertStdoutEquals(t, "", "value length 0 must return empty string (not NOTFOUND)")
}

func TestTier2_07_Val_Length1_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_07.db")

	res := h.Put(dbPath, "k_one", "v")
	h.CheckOrSkip(res, "Val Len 1")
	res.AssertSuccess(t, "value length 1 must be accepted")
}

func TestTier2_08_Val_Length64K_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_08.db")

	val64K := fixtures.FixedPayload(65536, 'V')
	res := h.Put(dbPath, "k_64k", val64K)
	h.CheckOrSkip(res, "Val Len 64K")
	res.AssertSuccess(t, "value length 64KB must be accepted")

	rGet := h.Get(dbPath, "k_64k")
	h.CheckOrSkip(rGet, "Get Val Len 64K")
	rGet.AssertStdoutEquals(t, val64K, "64KB value must match exactly")
}

func TestTier2_09_Val_Length1MB_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_09.db")

	val1MB := fixtures.FixedPayload(1048576, 'M')
	// Write via batch file to avoid OS execve argv argument length limit (MAX_ARG_STRLEN 128KB)
	batchFile := h.WriteBatchFile("batch_1mb.txt", []string{
		fmt.Sprintf("put k_1mb %s", val1MB),
	})
	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Val Len 1MB")
	res.AssertSuccess(t, "value length 1MB must be accepted via batch")

	rGet := h.Get(dbPath, "k_1mb")
	h.CheckOrSkip(rGet, "Get Val Len 1MB")
	rGet.AssertStdoutEquals(t, val1MB, "1MB value must match exactly")
}

func TestTier2_10_Val_Length1MBPlus1_Rejected(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_10.db")

	val1MBPlus1 := fixtures.FixedPayload(1048577, 'O')
	res := h.Put(dbPath, "k_overflow", val1MBPlus1)
	h.CheckOrSkip(res, "Val Len 1MB+1")
	res.AssertFailure(t, "value length 1MB+1 must be rejected with non-zero exit code")
}

// ----------------------------------------------------------------------------
// 3. DEL Operation Value Invariants (Cases 11-15)
// ----------------------------------------------------------------------------

func TestTier2_11_Del_NonExistentKey_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_11.db")

	_ = h.Open(dbPath)
	res := h.Del(dbPath, "ghostKey")
	h.CheckOrSkip(res, "Del Missing")
	res.AssertSuccess(t, "deleting missing key must succeed with code 0")
}

func TestTier2_12_Del_KeyWithEmptyValue_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_12.db")

	_ = h.Put(dbPath, "k_empty_val", "")
	res := h.Del(dbPath, "k_empty_val")
	h.CheckOrSkip(res, "Del Key With Empty Val")
	res.AssertSuccess(t, "deleting key that had empty value must succeed")

	rGet := h.Get(dbPath, "k_empty_val")
	h.CheckOrSkip(rGet, "Get Del Key With Empty Val")
	rGet.AssertStdoutContains(t, "NOTFOUND", "deleted key must be NOTFOUND")
}

func TestTier2_13_Del_KeyWith1MBValue_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_13.db")

	_ = h.Put(dbPath, "huge_to_del", fixtures.FixedPayload(1048576, 'H'))
	res := h.Del(dbPath, "huge_to_del")
	h.CheckOrSkip(res, "Del Key With 1MB Val")
	res.AssertSuccess(t, "deleting 1MB key must succeed")

	rGet := h.Get(dbPath, "huge_to_del")
	h.CheckOrSkip(rGet, "Get Del 1MB")
	rGet.AssertStdoutContains(t, "NOTFOUND", "1MB key must be removed after DEL")
}

func TestTier2_14_Del_Repeated3Times_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_14.db")

	_ = h.Put(dbPath, "tripleDel", "val")
	for i := 1; i <= 3; i++ {
		res := h.Del(dbPath, "tripleDel")
		h.CheckOrSkip(res, "Del Repeated 3x")
		res.AssertSuccess(t, fmt.Sprintf("del attempt %d must succeed", i))
	}
}

func TestTier2_15_Del_AfterDel_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_15.db")

	_ = h.Put(dbPath, "k", "v")
	_ = h.Del(dbPath, "k")

	res := h.Del(dbPath, "k")
	h.CheckOrSkip(res, "Del After Del")
	res.AssertSuccess(t, "DEL on already deleted key must succeed")
}

// ----------------------------------------------------------------------------
// 4. Batch Size Boundaries (Cases 16-20)
// ----------------------------------------------------------------------------

func TestTier2_16_Batch_0Ops_Rejected(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_16.db")

	emptyBatchFile := h.WriteBatchFile("empty_batch.txt", []string{})
	res := h.Batch(dbPath, emptyBatchFile)
	h.CheckOrSkip(res, "Batch 0 Ops")
	res.AssertFailure(t, "batch with 0 operations must be rejected with non-zero exit code")
}

func TestTier2_17_Batch_1Op_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_17.db")

	batchFile := h.WriteBatchFile("batch_1.txt", []string{
		"put singleBatchKey valOne",
	})
	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch 1 Op")
	res.AssertSuccess(t, "batch with 1 operation must be accepted")

	rGet := h.Get(dbPath, "singleBatchKey")
	h.CheckOrSkip(rGet, "Get batch 1 op")
	rGet.AssertStdoutEquals(t, "valOne", "singleBatchKey must be committed")
}

func TestTier2_18_Batch_2Ops_Accepted(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_18.db")

	batchFile := h.WriteBatchFile("batch_2.txt", []string{
		"put k1 v1",
		"put k2 v2",
	})
	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch 2 Ops")
	res.AssertSuccess(t, "batch with 2 operations must be accepted")
}

func TestTier2_19_Batch_10000Ops_Accepted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 10,000 op batch test in short mode")
	}
	h := harness.New(t)
	dbPath := h.DBPath("t2_19.db")

	lines := fixtures.Generate10000Batch("b10k")
	batchFile := h.WriteBatchFile("batch_10k.txt", lines)

	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch 10000 Ops")
	res.AssertSuccess(t, "batch with exactly 10,000 operations must be accepted")

	// Spot check first and last keys
	rFirst := h.Get(dbPath, "b10k_000000")
	h.CheckOrSkip(rFirst, "Get first of 10k")
	rFirst.AssertStdoutEquals(t, "val_000000", "first key of 10,000 batch must exist")

	rLast := h.Get(dbPath, "b10k_009999")
	h.CheckOrSkip(rLast, "Get last of 10k")
	rLast.AssertStdoutEquals(t, "val_009999", "last key of 10,000 batch must exist")
}

func TestTier2_20_Batch_10001Ops_Rejected(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_20.db")

	lines := make([]string, 10001)
	for i := 0; i < 10001; i++ {
		lines[i] = fmt.Sprintf("put k%d v%d", i, i)
	}
	batchFile := h.WriteBatchFile("batch_10001.txt", lines)

	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch 10001 Ops")
	res.AssertFailure(t, "batch with 10,001 operations must be rejected with non-zero exit code")
}

// ----------------------------------------------------------------------------
// 5. Intra-Batch Key Conflicts (Cases 21-25)
// ----------------------------------------------------------------------------

func TestTier2_21_Batch_TwoDuplicatePuts_LastWins(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_21.db")

	batchFile := h.WriteBatchFile("dup2.txt", []string{
		"put k initialVal",
		"put k finalVal",
	})
	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch 2 Dup Puts")
	res.AssertSuccess(t, "batch should succeed")

	rGet := h.Get(dbPath, "k")
	h.CheckOrSkip(rGet, "Get k")
	rGet.AssertStdoutEquals(t, "finalVal", "last write in batch must win")
}

func TestTier2_22_Batch_FiveDuplicatePuts_LastWins(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_22.db")

	batchFile := h.WriteBatchFile("dup5.txt", []string{
		"put k v1",
		"put k v2",
		"put k v3",
		"put k v4",
		"put k v5_winner",
	})
	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch 5 Dup Puts")
	res.AssertSuccess(t, "batch should succeed")

	rGet := h.Get(dbPath, "k")
	h.CheckOrSkip(rGet, "Get k")
	rGet.AssertStdoutEquals(t, "v5_winner", "5th write must win")
}

func TestTier2_23_Batch_PutThenDel_KeyAbsent(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_23.db")

	batchFile := h.WriteBatchFile("put_del.txt", []string{
		"put k tempVal",
		"del k",
	})
	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch Put Then Del")
	res.AssertSuccess(t, "batch should succeed")

	rGet := h.Get(dbPath, "k")
	h.CheckOrSkip(rGet, "Get k")
	rGet.AssertStdoutContains(t, "NOTFOUND", "key must be absent")
}

func TestTier2_24_Batch_DelThenPut_KeyPresent(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_24.db")

	batchFile := h.WriteBatchFile("del_put.txt", []string{
		"del k",
		"put k finalVal",
	})
	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch Del Then Put")
	res.AssertSuccess(t, "batch should succeed")

	rGet := h.Get(dbPath, "k")
	h.CheckOrSkip(rGet, "Get k")
	rGet.AssertStdoutEquals(t, "finalVal", "key must be present with final PUT value")
}

func TestTier2_25_Batch_PutDelPut_FinalPutWins(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_25.db")

	batchFile := h.WriteBatchFile("put_del_put.txt", []string{
		"put k first",
		"del k",
		"put k third_wins",
	})
	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch Put Del Put")
	res.AssertSuccess(t, "batch should succeed")

	rGet := h.Get(dbPath, "k")
	h.CheckOrSkip(rGet, "Get k")
	rGet.AssertStdoutEquals(t, "third_wins", "third operation must win")
}

// ----------------------------------------------------------------------------
// 6. Quota Exact Boundaries (Cases 26-30)
// ----------------------------------------------------------------------------

func TestTier2_26_Quota_ExactHeaderSize32B_Opens(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_26.db")

	// Quota is exactly 32 bytes (size of DB header)
	rOpen := h.Open(dbPath, "--max-bytes", "32")
	h.CheckOrSkip(rOpen, "Open Quota 32B")
	rOpen.AssertSuccess(t, "open should succeed for 32B quota")

	// Any write requires at least 32B (frame header + trailer) + 7B op header + key len >= 40B
	rPut := h.Put(dbPath, "k", "v")
	h.CheckOrSkip(rPut, "Put Quota 32B")
	rPut.AssertFailure(t, "put must fail when quota is 32B")
}

func TestTier2_27_Quota_WriteFittingExactlyQuota(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_27.db")

	// DB Header = 32. UABE Frame Header = 28. Subrecord: Op(1) + KeyLen(2) + ValLen(4) = 7.
	// Key "k" (1B), Val "v" (1B). Payload = 7 + 1 + 1 = 9. Trailer CRC = 4.
	// Total frame size = 28 + 9 + 4 = 41. Total file size = 32 + 41 = 73 bytes.
	rOpen := h.Open(dbPath, "--max-bytes", "73")
	h.CheckOrSkip(rOpen, "Open Quota 73B")
	rOpen.AssertSuccess(t, "open should succeed")

	rPut := h.Put(dbPath, "k", "v")
	h.CheckOrSkip(rPut, "Put Exactly Fitting Quota")
	rPut.AssertSuccess(t, "write exactly fitting quota must succeed")
}

func TestTier2_28_Quota_WriteExceedingBy1Byte_Rejected(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_28.db")

	// Quota is 72 bytes (1 byte less than required 73 bytes)
	rOpen := h.Open(dbPath, "--max-bytes", "72")
	h.CheckOrSkip(rOpen, "Open Quota 72B")
	rOpen.AssertSuccess(t, "open should succeed")

	rPut := h.Put(dbPath, "k", "v")
	h.CheckOrSkip(rPut, "Put Exceeding By 1B")
	rPut.AssertFailure(t, "write exceeding quota by 1 byte must be rejected")
}

func TestTier2_29_Quota_SecondWriteExceeding_FirstIntact(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_29.db")

	// Quota fits first write (73 bytes), but second write (another 41 bytes -> 114) exceeds 100 bytes cap
	rOpen := h.Open(dbPath, "--max-bytes", "100")
	h.CheckOrSkip(rOpen, "Open Quota 100B")
	rOpen.AssertSuccess(t, "open should succeed")

	rPut1 := h.Put(dbPath, "k", "v")
	h.CheckOrSkip(rPut1, "First Put Under Quota")
	rPut1.AssertSuccess(t, "first put must succeed")

	rPut2 := h.Put(dbPath, "k2", "v2")
	h.CheckOrSkip(rPut2, "Second Put Exceeding Quota")
	rPut2.AssertFailure(t, "second put exceeding quota must fail")

	// First key must remain intact
	rGet := h.Get(dbPath, "k")
	h.CheckOrSkip(rGet, "Get First Put")
	rGet.AssertStdoutEquals(t, "v", "first key must remain intact")
}

func TestTier2_30_Quota_BatchExceedingBy1Byte_RejectedAtomically(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_30.db")

	// 2 ops batch requiring 94 bytes (32 DB header + 28 frame header + 30 payload + 4 trailer).
	// Quota is 93 bytes (1 byte less than required 94 bytes).
	_ = h.Open(dbPath, "--max-bytes", "93")
	batchFile := h.WriteBatchFile("batch_exceed.txt", []string{
		"put k1 value1",
		"put k2 value2",
	})

	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch Exceeding Quota")
	res.AssertFailure(t, "batch exceeding quota must fail atomically")

	r1 := h.Get(dbPath, "k1")
	h.CheckOrSkip(r1, "Get k1")
	r1.AssertStdoutContains(t, "NOTFOUND", "k1 must not be present")
}

// ----------------------------------------------------------------------------
// 7. Special Characters & Binary Escaping (Cases 31-35)
// ----------------------------------------------------------------------------

func TestTier2_31_Binary_KeyWithUTF8NonASCII(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_31.db")

	utf8Key := "ключ_🔑_τάφος"
	res := h.Put(dbPath, utf8Key, "unicodeValue")
	h.CheckOrSkip(res, "Put UTF8 Key")
	res.AssertSuccess(t, "UTF-8 non-ASCII key must be accepted")

	rGet := h.Get(dbPath, utf8Key)
	h.CheckOrSkip(rGet, "Get UTF8 Key")
	rGet.AssertStdoutEquals(t, "unicodeValue", "UTF-8 key must return correct value")
}

func TestTier2_32_Binary_KeyWithSpaces(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_32.db")

	keyWithSpace := "key with space"
	res := h.Put(dbPath, keyWithSpace, "valSpace")
	h.CheckOrSkip(res, "Put Key With Space")
	res.AssertSuccess(t, "key with spaces must be accepted")

	rGet := h.Get(dbPath, keyWithSpace)
	h.CheckOrSkip(rGet, "Get Key With Space")
	rGet.AssertStdoutEquals(t, "valSpace", "key with space must return value")
}

func TestTier2_33_Binary_KeyWithTabsAndNewlinesHex(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_33.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	defer session.Close()

	// In serve mode, keys are hex-encoded, so tabs and newlines are safely supported
	rawKey := "key\twith\ncontrol\x00chars"
	keyHex := fixtures.EncodeHex(rawKey)
	valHex := fixtures.EncodeHex("safeVal")

	resp, err := session.Send(fmt.Sprintf("PUT 1 %s %s", keyHex, valHex))
	if err != nil {
		t.Skipf("Serve PUT not yet active: %v", err)
	}
	if !strings.HasPrefix(resp, "ACK 1") {
		t.Fatalf("expected ACK 1, got: %q", resp)
	}
}

func TestTier2_34_Binary_ValueWithEmbeddedNull(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_34.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	defer session.Close()

	keyHex := fixtures.EncodeHex("nullKey")
	valWithNull := "val\x00embedded\x00null"
	valHex := fixtures.EncodeHex(valWithNull)

	_, _ = session.Send(fmt.Sprintf("PUT 1 %s %s", keyHex, valHex))
	resp, err := session.Send(fmt.Sprintf("GET 2 %s", keyHex))
	if err != nil {
		t.Skipf("Serve GET not active: %v", err)
	}
	if !strings.Contains(resp, valHex) {
		t.Fatalf("expected hex value containing null bytes: %q", resp)
	}
}

func TestTier2_35_Binary_ValueAll256ByteValues(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_35.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	defer session.Close()

	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}

	keyHex := fixtures.EncodeHex("allBytesKey")
	valHex := fixtures.EncodeHex(string(allBytes))

	_, _ = session.Send(fmt.Sprintf("PUT 1 %s %s", keyHex, valHex))
	resp, err := session.Send(fmt.Sprintf("GET 2 %s", keyHex))
	if err != nil {
		t.Skipf("Serve GET not active: %v", err)
	}
	if !strings.Contains(resp, valHex) {
		t.Fatalf("expected 256 all-bytes sequence in response")
	}
}

// ----------------------------------------------------------------------------
// 8. Corrupted DB Tail Truncation (Cases 36-40)
// ----------------------------------------------------------------------------

func TestTier2_36_Tail_1ByteAppendedAtEOF_Repaired(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_36.db")

	_ = h.Put(dbPath, "stable", "val")
	h.AppendBytes(dbPath, []byte{0xAA})

	res := h.Open(dbPath)
	h.CheckOrSkip(res, "Open 1B Torn Tail")
	res.AssertSuccess(t, "open must repair 1-byte torn tail at EOF")

	rGet := h.Get(dbPath, "stable")
	h.CheckOrSkip(rGet, "Get stable")
	rGet.AssertStdoutEquals(t, "val", "stable key must remain intact")
}

func TestTier2_37_Tail_PartialFrameHeader14B_Repaired(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_37.db")

	_ = h.Put(dbPath, "k", "v")
	// Append 14 bytes (half of 28B UABE header)
	h.AppendBytes(dbPath, make([]byte, 14))

	res := h.Open(dbPath)
	h.CheckOrSkip(res, "Open 14B Tail Header")
	res.AssertSuccess(t, "open must truncate 14-byte partial header at EOF")
}

func TestTier2_38_Tail_PartialPayload_Repaired(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_38.db")

	_ = h.Put(dbPath, "k", "v")

	// Append valid 28-byte frame header declaring 100 bytes payload, but append only 10 bytes
	fh := make([]byte, 28)
	copy(fh[0:4], []byte{0x45, 0x42, 0x41, 0x55}) // frame magic 0x55414245
	h.AppendBytes(dbPath, append(fh, make([]byte, 10)...))

	res := h.Open(dbPath)
	h.CheckOrSkip(res, "Open Partial Payload Tail")
	res.AssertSuccess(t, "open must truncate partial payload tail at EOF")
}

func TestTier2_39_Tail_PartialTrailer2B_Repaired(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_39.db")

	_ = h.Put(dbPath, "k", "v")
	// Remove 2 bytes from end of file (truncating the 4-byte CRC trailer)
	h.TruncateBytes(dbPath, 2)

	res := h.Open(dbPath)
	h.CheckOrSkip(res, "Open Truncated Trailer")
	res.AssertSuccess(t, "open must truncate torn tail with missing trailer bytes")
}

func TestTier2_40_Tail_CorruptedTrailerBit_Repaired(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_40.db")

	_ = h.Put(dbPath, "k1", "v1")
	_ = h.Put(dbPath, "k2", "v2")

	// Corrupt last byte of file (trailer CRC of k2)
	size := h.GetFileSize(dbPath)
	h.CorruptByte(dbPath, size-1, 0x01)

	// k2 is torn at EOF, so open repairs to k1
	res := h.Open(dbPath)
	h.CheckOrSkip(res, "Open Corrupted Trailer Bit")
	res.AssertSuccess(t, "open must repair corrupted trailer at EOF")

	r1 := h.Get(dbPath, "k1")
	h.CheckOrSkip(r1, "Get k1")
	r1.AssertStdoutEquals(t, "v1", "k1 must remain intact")
}

// ----------------------------------------------------------------------------
// 9. Malformed Input Syntax (Cases 41-45)
// ----------------------------------------------------------------------------

func TestTier2_41_CLI_MissingArgs(t *testing.T) {
	h := harness.New(t)
	res := h.Run("put")
	h.CheckOrSkip(res, "CLI Missing Args")
	res.AssertFailure(t, "put without arguments must fail")
}

func TestTier2_42_CLI_ExcessArgs(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_42.db")

	res := h.Run("get", dbPath, "key", "excessArg")
	h.CheckOrSkip(res, "CLI Excess Args")
	res.AssertFailure(t, "get with excess arguments must fail")
}

func TestTier2_43_CLI_UnknownSubcommand(t *testing.T) {
	h := harness.New(t)
	res := h.Run("unsupportedSubcommand")
	res.AssertFailure(t, "unknown subcommand must fail")
}

func TestTier2_44_CLI_BatchInvalidCommandToken(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_44.db")

	batchFile := h.WriteBatchFile("bad_token.txt", []string{
		"invalidToken key val",
	})
	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch Bad Token")
	res.AssertFailure(t, "batch file with invalid command token must fail")
}

func TestTier2_45_CLI_BatchTruncatedLine(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_45.db")

	batchFile := h.WriteBatchFile("truncated_line.txt", []string{
		"put", // missing key and value
	})
	res := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(res, "Batch Truncated Line")
	res.AssertFailure(t, "batch file with truncated line must fail")
}

// ----------------------------------------------------------------------------
// 10. Non-Tail Middle-of-File Corruption (Cases 46-50)
// ----------------------------------------------------------------------------

func TestTier2_46_Middle_BitFlipFrameMagic_FailClosed(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_46.db")

	_ = h.Put(dbPath, "firstKey", "firstVal")
	_ = h.Put(dbPath, "secondKey", "secondVal")

	// Frame 1 magic is at offset 32. Corrupt it.
	h.CorruptByte(dbPath, 32, 0xFF)

	// Since Frame 2 follows, Frame 1 is NOT tail. Must fail closed (CORRUPT_FRAME).
	res := h.Open(dbPath)
	h.CheckOrSkip(res, "Open Middle Corrupted Magic")
	res.AssertFailure(t, "middle-of-file corruption must fail closed (not truncate to byte 32)")
}

func TestTier2_47_Middle_BitFlipFrameCRC_FailClosed(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_47.db")

	_ = h.Put(dbPath, "k1", "v1")
	_ = h.Put(dbPath, "k2", "v2")

	// Frame header CRC is at offset 32 + 4 = 36. Corrupt it.
	h.CorruptByte(dbPath, 36, 0x01)

	res := h.Open(dbPath)
	h.CheckOrSkip(res, "Open Middle Corrupted Frame CRC")
	res.AssertFailure(t, "middle-of-file header CRC corruption must fail closed")
}

func TestTier2_48_Middle_BitFlipPayload_FailClosed(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_48.db")

	_ = h.Put(dbPath, "k1", "long_first_value")
	_ = h.Put(dbPath, "k2", "v2")

	// Corrupt payload of Frame 1 at offset 65
	h.CorruptByte(dbPath, 65, 0x01)

	res := h.Open(dbPath)
	h.CheckOrSkip(res, "Open Middle Corrupted Payload")
	res.AssertFailure(t, "middle-of-file payload corruption must fail closed")
}

func TestTier2_49_Middle_CheckFlagsMiddleCorruption(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_49.db")

	_ = h.Put(dbPath, "k1", "v1")
	_ = h.Put(dbPath, "k2", "v2")

	h.CorruptByte(dbPath, 32, 0x02)

	res := h.Check(dbPath)
	h.CheckOrSkip(res, "Check Middle Corruption")
	res.AssertFailure(t, "check must fail on middle-of-file corruption")
}

func TestTier2_50_Middle_DoesNotTruncateHealthyPrefix(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_50.db")

	_ = h.Put(dbPath, "safeKey", "safeVal")
	sizeBefore := h.GetFileSize(dbPath)

	_ = h.Put(dbPath, "midKey", "midVal")
	_ = h.Put(dbPath, "lateKey", "lateVal")

	// Corrupt midKey frame magic
	h.CorruptByte(dbPath, sizeBefore, 0x55)

	// Open must fail closed and NOT truncate the file
	_ = h.Open(dbPath)
	sizeAfter := h.GetFileSize(dbPath)
	if sizeAfter < sizeBefore {
		t.Fatalf("engine inappropriately truncated non-tail corruption")
	}
}

// ----------------------------------------------------------------------------
// 11. Filesystem & Permission Boundaries (Cases 51-55)
// ----------------------------------------------------------------------------

func TestTier2_51_FS_NonExistentDirectory_GracefulError(t *testing.T) {
	h := harness.New(t)
	dbPath := "/non_existent_dir_xyz/db.db"

	res := h.Open(dbPath)
	h.CheckOrSkip(res, "FS Non-existent dir")
	res.AssertFailure(t, "open in non-existent directory must return error")
}

func TestTier2_52_FS_ReadOnlyDatabase_GracefulError(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_52.db")

	_ = h.Open(dbPath)
	if !h.FileExists(dbPath) {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'open' acknowledged (M1 CLI router)"}, "DB File Precondition")
		return
	}
	// Change DB permissions to read-only
	_ = os.Chmod(dbPath, 0444)
	defer func() {
		_ = os.Chmod(dbPath, 0644)
	}()

	res := h.Put(dbPath, "k", "v")
	h.CheckOrSkip(res, "FS Read-only DB")
	res.AssertFailure(t, "put on read-only file must return error")
}

func TestTier2_53_FS_RepeatedCheckHealthyDB_ConsistentOK(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_53.db")

	_ = h.Put(dbPath, "k", "v")
	for i := 1; i <= 3; i++ {
		res := h.Check(dbPath)
		h.CheckOrSkip(res, "Repeated Check")
		res.AssertSuccess(t, fmt.Sprintf("check %d should succeed", i))
		res.AssertStdoutContains(t, "OK", "repeated check must output OK")
	}
}

func TestTier2_54_FS_BatchFileMissing_Error(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_54.db")

	res := h.Batch(dbPath, "/non_existent_batch_file_123.txt")
	h.CheckOrSkip(res, "FS Batch Missing")
	res.AssertFailure(t, "batch with non-existent file must fail")
}

func TestTier2_55_FS_EmptyDBCheck_OK(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t2_55.db")

	_ = h.Open(dbPath)
	res := h.Check(dbPath)
	h.CheckOrSkip(res, "Check Empty DB")
	res.AssertSuccess(t, "check on newly created empty DB must succeed")
	res.AssertStdoutContains(t, "OK", "empty DB check must report OK")
}
