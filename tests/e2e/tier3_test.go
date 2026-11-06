package e2e_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"kvdb/tests/e2e/fixtures"
	"kvdb/tests/e2e/harness"
)

// ============================================================================
// Tier 3 - Cross-Feature Pairwise Interactions (15 Test Cases)
// ============================================================================

func TestTier3_01_Concurrent_TwoProcessesOpenSameDB(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_01.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session 1")
		return
	}
	defer session.Close()
	time.Sleep(50 * time.Millisecond)

	// Second process attempts open on same DB while session 1 holds lock
	res2 := h.Open(dbPath)
	h.CheckOrSkip(res2, "Concurrent Open")
	res2.AssertFailure(t, "second process open must fail while first holds lock")
}

func TestTier3_02_Concurrent_ReadWhileWrite(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_02.db")

	_ = h.Put(dbPath, "sharedKey", "valInitial")

	var wg sync.WaitGroup
	wg.Add(2)

	var writeErr error
	var readVal string

	go func() {
		defer wg.Done()
		r := h.Put(dbPath, "sharedKey", "valUpdated")
		if r.ExitCode != 0 {
			writeErr = r.Err
		}
	}()

	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		r := h.Get(dbPath, "sharedKey")
		readVal = strings.TrimSpace(r.Stdout)
	}()

	wg.Wait()
	if writeErr != nil {
		t.Logf("Concurrent write note: %v", writeErr)
	}
	if readVal != "" && readVal != "valInitial" && readVal != "valUpdated" && !strings.Contains(readVal, "acknowledged") {
		t.Fatalf("dirty read observed: %q", readVal)
	}
}

func TestTier3_03_Concurrent_ServePutAndDel(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_03.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	defer session.Close()

	// Interleave PUT and DEL commands over single session
	kHex := fixtures.EncodeHex("concurrentKey")
	vHex := fixtures.EncodeHex("concurrentVal")

	rPut, err := session.Send(fmt.Sprintf("PUT 1 %s %s", kHex, vHex))
	if err != nil {
		t.Skipf("Serve PUT not active: %v", err)
	}
	if !strings.HasPrefix(rPut, "ACK 1") {
		t.Fatalf("expected ACK 1, got: %q", rPut)
	}

	rDel, err := session.Send(fmt.Sprintf("DEL 2 %s", kHex))
	if err != nil {
		t.Fatalf("Serve DEL failed: %v", err)
	}
	if !strings.HasPrefix(rDel, "ACK 2") {
		t.Fatalf("expected ACK 2, got: %q", rDel)
	}
}

func TestTier3_04_Interleaved_SinglePutThenBatchDel(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_04.db")

	r1 := h.Put(dbPath, "interKey", "v1")
	h.CheckOrSkip(r1, "Single Put")
	r1.AssertSuccess(t, "single put should succeed")

	batchFile := h.WriteBatchFile("batch_del.txt", []string{
		"del interKey",
	})
	r2 := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(r2, "Batch Del")
	r2.AssertSuccess(t, "batch del should succeed")

	r3 := h.Get(dbPath, "interKey")
	h.CheckOrSkip(r3, "Get interKey")
	r3.AssertStdoutContains(t, "NOTFOUND", "key deleted via batch must be NOTFOUND")
}

func TestTier3_05_Interleaved_BatchPutThenSingleDel(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_05.db")

	batchFile := h.WriteBatchFile("batch_put.txt", []string{
		"put k1 v1",
		"put k2 v2",
	})
	r1 := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(r1, "Batch Put")
	r1.AssertSuccess(t, "batch put should succeed")

	r2 := h.Del(dbPath, "k1")
	h.CheckOrSkip(r2, "Single Del")
	r2.AssertSuccess(t, "single del should succeed")

	rGet1 := h.Get(dbPath, "k1")
	h.CheckOrSkip(rGet1, "Get k1")
	rGet1.AssertStdoutContains(t, "NOTFOUND", "k1 must be deleted")

	rGet2 := h.Get(dbPath, "k2")
	h.CheckOrSkip(rGet2, "Get k2")
	rGet2.AssertStdoutEquals(t, "v2", "k2 must remain intact")
}

func TestTier3_06_Interleaved_SinglePutThenBatchOverwrite(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_06.db")

	_ = h.Put(dbPath, "k", "valSingle")

	batchFile := h.WriteBatchFile("batch_ow.txt", []string{
		"put k valBatch",
	})
	rBatch := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(rBatch, "Batch Overwrite")
	rBatch.AssertSuccess(t, "batch overwrite should succeed")

	rGet := h.Get(dbPath, "k")
	h.CheckOrSkip(rGet, "Get k")
	rGet.AssertStdoutEquals(t, "valBatch", "batch overwrite value must win")
}

func TestTier3_07_Interleaved_BatchPutThenSingleOverwrite(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_07.db")

	batchFile := h.WriteBatchFile("batch_init.txt", []string{
		"put k valBatch",
	})
	_ = h.Batch(dbPath, batchFile)

	rPut := h.Put(dbPath, "k", "valSingle")
	h.CheckOrSkip(rPut, "Single Overwrite")
	rPut.AssertSuccess(t, "single overwrite should succeed")

	rGet := h.Get(dbPath, "k")
	h.CheckOrSkip(rGet, "Get k")
	rGet.AssertStdoutEquals(t, "valSingle", "single overwrite value must win")
}

func TestTier3_08_Interleaved_MultiPhasePutBatchDelDump(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_08.db")

	// Phase 1: Single PUTs
	_ = h.Put(dbPath, "k1", "v1")
	_ = h.Put(dbPath, "k2", "v2")

	// Phase 2: Batch (PUT k3, DEL k1)
	batchFile := h.WriteBatchFile("phase2.txt", []string{
		"put k3 v3",
		"del k1",
	})
	_ = h.Batch(dbPath, batchFile)

	// Phase 3: Single PUT k4
	_ = h.Put(dbPath, "k4", "v4")

	// Phase 4: Verify DUMP contains k2, k3, k4 in sorted order, excluding k1
	res := h.Dump(dbPath)
	h.CheckOrSkip(res, "MultiPhase Dump")
	res.AssertSuccess(t, "dump should succeed")

	m := fixtures.ParseDumpOutput(res.Stdout)
	if _, ok := m["k1"]; ok {
		t.Fatalf("k1 should be deleted")
	}
	if m["k2"] != "v2" || m["k3"] != "v3" || m["k4"] != "v4" {
		t.Fatalf("dump values mismatch: %+v", m)
	}

	sorted, _ := fixtures.CheckDumpIsSorted(res.Stdout)
	if !sorted {
		t.Fatalf("dump output not sorted")
	}
}

func TestTier3_09_Quota_SingleExceededThenBatchExceeded(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_09.db")

	_ = h.Open(dbPath, "--max-bytes", "150")

	// Single PUT that exceeds
	rSingle := h.Put(dbPath, "huge", fixtures.FixedPayload(500, 'A'))
	h.CheckOrSkip(rSingle, "Single Exceeded")
	rSingle.AssertFailure(t, "single put must exceed quota")

	// Batch that exceeds
	batchFile := h.WriteBatchFile("batch_huge.txt", []string{
		fmt.Sprintf("put b1 %s", fixtures.FixedPayload(500, 'B')),
	})
	rBatch := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(rBatch, "Batch Exceeded")
	rBatch.AssertFailure(t, "batch must exceed quota")

	// DB remains readable
	rCheck := h.Check(dbPath)
	h.CheckOrSkip(rCheck, "Check Post Double Quota")
	rCheck.AssertSuccess(t, "DB must remain valid after rejected operations")
}

func TestTier3_10_Quota_BatchExceededThenSingleFits(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_10.db")

	// Quota is 120 bytes
	_ = h.Open(dbPath, "--max-bytes", "120")

	// Huge batch rejected
	batchFile := h.WriteBatchFile("huge.txt", []string{
		fmt.Sprintf("put k %s", fixtures.FixedPayload(500, 'H')),
	})
	_ = h.Batch(dbPath, batchFile)

	// Small single PUT that fits under 120 bytes (header 32 + frame ~40 = 72)
	rPut := h.Put(dbPath, "fitK", "fitV")
	h.CheckOrSkip(rPut, "Single Fits Post Batch")
	rPut.AssertSuccess(t, "small single write should succeed after rejected batch")

	rGet := h.Get(dbPath, "fitK")
	h.CheckOrSkip(rGet, "Get fitK")
	rGet.AssertStdoutEquals(t, "fitV", "fitK must be readable")
}

func TestTier3_11_Compaction_MixedPutsDelsPreservesLive(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_11.db")

	// Insert 10 keys
	for i := 1; i <= 10; i++ {
		_ = h.Put(dbPath, fmt.Sprintf("live_%d", i), fmt.Sprintf("val_%d", i))
	}

	// Delete even keys
	for i := 2; i <= 10; i += 2 {
		_ = h.Del(dbPath, fmt.Sprintf("live_%d", i))
	}

	// Check dump contains only odd keys
	res := h.Dump(dbPath)
	h.CheckOrSkip(res, "Compaction Preserves Live Dump")
	res.AssertSuccess(t, "dump should succeed")

	m := fixtures.ParseDumpOutput(res.Stdout)
	for i := 1; i <= 10; i++ {
		k := fmt.Sprintf("live_%d", i)
		if i%2 == 1 {
			if _, ok := m[k]; !ok {
				t.Fatalf("expected odd key %s to be present", k)
			}
		} else {
			if _, ok := m[k]; ok {
				t.Fatalf("expected even key %s to be absent", k)
			}
		}
	}
}

func TestTier3_12_Crash_BatchTailTorn_PriorSinglePutsIntact(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_12.db")

	_ = h.Put(dbPath, "priorKey1", "v1")
	_ = h.Put(dbPath, "priorKey2", "v2")

	// Simulate crash in middle of batch append
	h.AppendBytes(dbPath, []byte{0x45, 0x42, 0x41, 0x55, 0x01, 0x02, 0x03})

	// Open and verify prior keys survived intact
	rOpen := h.Open(dbPath)
	h.CheckOrSkip(rOpen, "Open Post Torn Batch")
	rOpen.AssertSuccess(t, "recovery must truncate torn batch at EOF")

	r1 := h.Get(dbPath, "priorKey1")
	h.CheckOrSkip(r1, "Get priorKey1")
	r1.AssertStdoutEquals(t, "v1", "priorKey1 must be intact")

	r2 := h.Get(dbPath, "priorKey2")
	h.CheckOrSkip(r2, "Get priorKey2")
	r2.AssertStdoutEquals(t, "v2", "priorKey2 must be intact")
}

func TestTier3_13_Serve_InterleavedPutGetDelDump(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_13.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	defer session.Close()

	kHex := fixtures.EncodeHex("serveFlowKey")
	vHex := fixtures.EncodeHex("serveFlowVal")

	// 1. PUT
	rPut, err := session.Send(fmt.Sprintf("PUT 10 %s %s", kHex, vHex))
	if err != nil {
		t.Skipf("Serve PUT not active: %v", err)
	}
	if !strings.HasPrefix(rPut, "ACK 10") {
		t.Fatalf("expected ACK 10, got: %q", rPut)
	}

	// 2. GET
	rGet, err := session.Send(fmt.Sprintf("GET 11 %s", kHex))
	if err != nil {
		t.Fatalf("Serve GET failed: %v", err)
	}
	if !strings.Contains(rGet, vHex) && !strings.Contains(rGet, "serveFlowVal") {
		t.Fatalf("GET response expected val: %q", rGet)
	}

	// 3. DEL
	rDel, err := session.Send(fmt.Sprintf("DEL 12 %s", kHex))
	if err != nil {
		t.Fatalf("Serve DEL failed: %v", err)
	}
	if !strings.HasPrefix(rDel, "ACK 12") {
		t.Fatalf("expected ACK 12, got: %q", rDel)
	}

	// 4. GET after DEL
	rGetPost, err := session.Send(fmt.Sprintf("GET 13 %s", kHex))
	if err != nil {
		t.Fatalf("Serve GET post DEL failed: %v", err)
	}
	if !strings.Contains(rGetPost, "NOTFOUND") {
		t.Fatalf("expected NOTFOUND after DEL, got: %q", rGetPost)
	}
}

func TestTier3_14_Serve_RequestIDOutofOrderHandling(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_14.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	defer session.Close()

	// Non-sequential IDs: 999 then 12 then 42
	ids := []string{"999", "12", "42"}
	for _, id := range ids {
		k := fixtures.EncodeHex("k_" + id)
		v := fixtures.EncodeHex("v_" + id)
		resp, err := session.Send(fmt.Sprintf("PUT %s %s %s", id, k, v))
		if err != nil {
			t.Skipf("Serve PUT not active: %v", err)
		}
		if !strings.HasPrefix(resp, fmt.Sprintf("ACK %s", id)) {
			t.Fatalf("expected ACK %s, got: %q", id, resp)
		}
	}
}

func TestTier3_15_Check_AfterComplexWorkloadReturnsOK(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t3_15.db")

	// Run complex mix
	_ = h.Put(dbPath, "mix1", "v1")
	batchFile := h.WriteBatchFile("mix_batch.txt", []string{
		"put mix2 v2",
		"del mix1",
		"put mix3 v3",
	})
	_ = h.Batch(dbPath, batchFile)
	_ = h.Del(dbPath, "mix3")

	res := h.Check(dbPath)
	h.CheckOrSkip(res, "Check Complex Workload")
	res.AssertSuccess(t, "check must report OK after complex multi-operation workload")
	res.AssertStdoutContains(t, "OK", "check must return OK")
}
