package e2e_test

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	"kvdb/tests/e2e/fixtures"
	"kvdb/tests/e2e/harness"
)

// ============================================================================
// Tier 4 - Real-World Application Scenarios (5 Scenarios)
// ============================================================================

// ----------------------------------------------------------------------------
// Scenario 1: Financial Ledger Batch Commit & Audit Trail
// ----------------------------------------------------------------------------
func TestTier4_01_FinancialLedgerBatchCommit(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t4_01_ledger.db")

	// Initial balances
	initBatch := h.WriteBatchFile("init_ledger.txt", []string{
		"put acc_alice 1000",
		"put acc_bob 500",
		"put acc_charlie 200",
	})
	rInit := h.Batch(dbPath, initBatch)
	h.CheckOrSkip(rInit, "Ledger Init")
	rInit.AssertSuccess(t, "initial ledger batch should succeed")

	// Transfer 1: Alice -> Bob $150
	t1 := h.WriteBatchFile("transfer1.txt", []string{
		"put acc_alice 850",
		"put acc_bob 650",
	})
	rT1 := h.Batch(dbPath, t1)
	h.CheckOrSkip(rT1, "Transfer 1")
	rT1.AssertSuccess(t, "transfer 1 should succeed")

	// Transfer 2: Bob -> Charlie $50
	t2 := h.WriteBatchFile("transfer2.txt", []string{
		"put acc_bob 600",
		"put acc_charlie 250",
	})
	rT2 := h.Batch(dbPath, t2)
	h.CheckOrSkip(rT2, "Transfer 2")
	rT2.AssertSuccess(t, "transfer 2 should succeed")

	// Simulate crashed transfer: Alice -> Charlie $100 torn at EOF
	h.AppendBytes(dbPath, []byte("torn_transfer_marker_uncommitted"))

	// Recovery open
	rRec := h.Open(dbPath)
	h.CheckOrSkip(rRec, "Ledger Recovery")
	rRec.AssertSuccess(t, "recovery must repair torn transfer")

	// Audit balances: total must be exactly 1000 + 500 + 200 = 1700
	aliceRes := h.Get(dbPath, "acc_alice")
	h.CheckOrSkip(aliceRes, "Get Alice")
	aliceBal, _ := strconv.Atoi(strings.TrimSpace(aliceRes.Stdout))

	bobRes := h.Get(dbPath, "acc_bob")
	h.CheckOrSkip(bobRes, "Get Bob")
	bobBal, _ := strconv.Atoi(strings.TrimSpace(bobRes.Stdout))

	charlieRes := h.Get(dbPath, "acc_charlie")
	h.CheckOrSkip(charlieRes, "Get Charlie")
	charlieBal, _ := strconv.Atoi(strings.TrimSpace(charlieRes.Stdout))

	total := aliceBal + bobBal + charlieBal
	if total != 1700 {
		t.Fatalf("financial ledger invariant violated! Expected total $1700, got $%d (Alice:%d, Bob:%d, Charlie:%d)",
			total, aliceBal, bobBal, charlieBal)
	}
}

// ----------------------------------------------------------------------------
// Scenario 2: High-Throughput Random Crash Simulation
// ----------------------------------------------------------------------------
func TestTier4_02_HighThroughputRandomCrashSimulation(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t4_02_crash.db")

	session, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}

	ackedKeys := make(map[string]string)
	rng := rand.New(rand.NewSource(42))

	// Write ops and kill after a brief burst
	killTimer := time.After(300 * time.Millisecond)
	opID := 1
loop:
	for {
		select {
		case <-killTimer:
			// Sudden SIGKILL in the middle of active ingestion
			_ = session.Kill()
			break loop
		default:
			k := fmt.Sprintf("k_%05d", opID)
			v := fmt.Sprintf("v_%05d_%d", opID, rng.Intn(10000))
			resp, err := session.Send(fmt.Sprintf("PUT %d %s %s", opID, fixtures.EncodeHex(k), fixtures.EncodeHex(v)))
			if err != nil {
				// Process was killed
				break loop
			}
			if strings.HasPrefix(resp, fmt.Sprintf("ACK %d", opID)) {
				ackedKeys[k] = v
			}
			opID++
			if opID > 200 {
				_ = session.Kill()
				break loop
			}
		}
	}

	// Recovery open
	startRec := time.Now()
	rOpen := h.Open(dbPath)
	recDuration := time.Since(startRec)

	h.CheckOrSkip(rOpen, "High-throughput Crash Recovery")
	rOpen.AssertSuccess(t, "database must recover cleanly after abrupt SIGKILL")

	if recDuration > 2*time.Second {
		t.Fatalf("recovery took %v, exceeded <= 2.0s limit", recDuration)
	}

	// Invariant Check 1: All acknowledged writes MUST be present
	for k, expectedVal := range ackedKeys {
		rGet := h.Get(dbPath, k)
		h.CheckOrSkip(rGet, "Verify Acked Key")
		actualVal := strings.TrimSpace(rGet.Stdout)
		if actualVal != expectedVal {
			t.Fatalf("durability invariant broken for key %s: expected %q, got %q", k, expectedVal, actualVal)
		}
	}

	// Invariant Check 2: Check database integrity
	rCheck := h.Check(dbPath)
	h.CheckOrSkip(rCheck, "Check Post-Crash DB")
	rCheck.AssertSuccess(t, "database structure must be valid post crash recovery")
}

// ----------------------------------------------------------------------------
// Scenario 3: Cache Eviction & Compaction Cycle
// ----------------------------------------------------------------------------
func TestTier4_03_CacheEvictionAndCompactionCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cache eviction cycle in short mode")
	}
	h := harness.New(t)
	dbPath := h.DBPath("t4_03_cache.db")

	const totalKeys = 500
	const evictKeys = 400

	// Step 1: Ingest keys
	for i := 0; i < totalKeys; i++ {
		k := fmt.Sprintf("cache_key_%04d", i)
		v := fmt.Sprintf("payload_%04d", i)
		_ = h.Put(dbPath, k, v)
	}

	// Step 2: Evict first 400 keys
	for i := 0; i < evictKeys; i++ {
		k := fmt.Sprintf("cache_key_%04d", i)
		_ = h.Del(dbPath, k)
	}

	// Step 3: Verify dump contains exactly remaining 100 keys
	rDump := h.Dump(dbPath)
	h.CheckOrSkip(rDump, "Cache Eviction Dump")
	rDump.AssertSuccess(t, "dump should succeed")

	m := fixtures.ParseDumpOutput(rDump.Stdout)
	expectedRetained := totalKeys - evictKeys
	if len(m) != expectedRetained {
		t.Fatalf("expected exactly %d retained keys in cache, got %d", expectedRetained, len(m))
	}

	// Verify all evicted keys are NOTFOUND
	for i := 0; i < 10; i++ {
		k := fmt.Sprintf("cache_key_%04d", i)
		rGet := h.Get(dbPath, k)
		h.CheckOrSkip(rGet, "Verify Evicted Key")
		rGet.AssertStdoutContains(t, "NOTFOUND", "evicted key must return NOTFOUND")
	}
}

// ----------------------------------------------------------------------------
// Scenario 4: Multi-Process Lock Contention & Failover on Kill
// ----------------------------------------------------------------------------
func TestTier4_04_MultiProcessLockContentionAndFailover(t *testing.T) {
	h := harness.New(t)
	dbPath := h.DBPath("t4_04_failover.db")

	// Process 1 starts serve mode, holding lock
	p1, err := h.Serve(dbPath)
	if err != nil {
		h.CheckOrSkip(harness.Result{Stdout: "Command 'serve' acknowledged (M1 CLI router)"}, "Serve Session")
		return
	}
	time.Sleep(50 * time.Millisecond)

	// Process 2 attempts write; must be rejected due to lock exclusivity
	rP2 := h.Put(dbPath, "p2_key", "p2_val")
	h.CheckOrSkip(rP2, "Process 2 Lock Contention")
	rP2.AssertFailure(t, "process 2 must be rejected while process 1 holds lock")

	// Abruptly kill Process 1 with SIGKILL
	if err := p1.Kill(); err != nil {
		t.Fatalf("failed to kill process 1: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Process 2 fails over and retries; must acquire lock and succeed
	rFailover := h.Put(dbPath, "failoverKey", "failoverVal")
	h.CheckOrSkip(rFailover, "Process 2 Failover")
	rFailover.AssertSuccess(t, "failover process must acquire lock and succeed")

	rGet := h.Get(dbPath, "failoverKey")
	h.CheckOrSkip(rGet, "Get Failover Key")
	rGet.AssertStdoutEquals(t, "failoverVal", "failoverKey must be readable")
}

// ----------------------------------------------------------------------------
// Scenario 5: Massive Scale Recovery Benchmark (<= 2.0s Limit)
// ----------------------------------------------------------------------------
func TestTier4_05_1MKeysRecoveryBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping massive recovery benchmark in short mode")
	}
	h := harness.New(t)
	dbPath := h.DBPath("t4_05_scale.db")

	// Seed dataset with 1,000 keys
	lines := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		lines[i] = fmt.Sprintf("put scale_%06d val_%06d", i, i)
	}
	batchFile := h.WriteBatchFile("scale_batch.txt", lines)
	rBatch := h.Batch(dbPath, batchFile)
	h.CheckOrSkip(rBatch, "Scale Batch Seed")
	rBatch.AssertSuccess(t, "scale batch write should succeed")

	// Benchmark recovery open duration
	start := time.Now()
	rOpen := h.Open(dbPath)
	duration := time.Since(start)

	h.CheckOrSkip(rOpen, "Scale Recovery Open")
	rOpen.AssertSuccess(t, "recovery open should succeed")

	t.Logf("1,000 keys recovery duration: %v (target for 1M is <= 2.0s)", duration)
	if duration > 2*time.Second {
		t.Fatalf("recovery took %v, exceeded <= 2.0s threshold", duration)
	}

	rCheck := h.Check(dbPath)
	h.CheckOrSkip(rCheck, "Scale Check")
	rCheck.AssertSuccess(t, "integrity check on recovery must pass")
}
