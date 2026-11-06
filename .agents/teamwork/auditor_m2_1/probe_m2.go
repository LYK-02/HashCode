package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"kvdb/internal/engine"
	"kvdb/internal/format"
)

var castagnoliTable = crc32.MakeTable(crc32.Castagnoli)

func check(err error, msg string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %s: %v\n", msg, err)
		os.Exit(1)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "hold_lock" {
		dbPath := os.Args[2]
		eng, err := engine.Open(dbPath, engine.Options{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "CHILD: failed to open: %v\n", err)
			os.Exit(2)
		}
		fmt.Println("CHILD_READY")
		time.Sleep(30 * time.Second)
		_ = eng.Close()
		os.Exit(0)
	}

	fmt.Println("=== Starting Milestone M2 Independent Forensic Integrity Probe ===")

	tmpDir, err := os.MkdirTemp("", "auditor_m2_probe_*")
	check(err, "create temp dir")
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	lockPath := dbPath + ".lock"

	// ------------------------------------------------------------------------
	// Check 1: Multi-Process Flock & SIGKILL Recovery Verification
	// ------------------------------------------------------------------------
	fmt.Print("[PROBE 1/6] Multi-process flock mutual exclusion & SIGKILL recovery... ")

	child := exec.Command(os.Args[0], "hold_lock", dbPath)
	childOut := &bytes.Buffer{}
	child.Stdout = childOut
	check(child.Start(), "start child process")

	// Wait for child to acquire lock
	for i := 0; i < 50; i++ {
		if bytes.Contains(childOut.Bytes(), []byte("CHILD_READY")) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Parent attempts to open while child holds lock: MUST fail with ErrLocked
	_, err = engine.Open(dbPath, engine.Options{})
	if err == nil {
		fmt.Println("FAIL: Open succeeded while child holds exclusive lock!")
		os.Exit(1)
	}
	if err != engine.ErrLocked {
		fmt.Printf("FAIL: Expected ErrLocked, got: %v\n", err)
		os.Exit(1)
	}

	// Verify lock sidecar exists and was not deleted
	if _, err := os.Stat(lockPath); err != nil {
		fmt.Printf("FAIL: Lock sidecar missing: %v\n", err)
		os.Exit(1)
	}

	// Kill child with SIGKILL
	check(child.Process.Signal(syscall.SIGKILL), "kill child with SIGKILL")
	_ = child.Wait()

	// Immediately attempt to acquire lock in parent: MUST succeed
	eng1, err := engine.Open(dbPath, engine.Options{})
	check(err, "parent Open after child SIGKILL")
	check(eng1.Close(), "close eng1")

	// Sidecar must STILL be intact on filesystem
	if _, err := os.Stat(lockPath); err != nil {
		fmt.Printf("FAIL: Lock sidecar deleted after SIGKILL/close: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("PASSED")

	// ------------------------------------------------------------------------
	// Check 2: Physical Disk Layout & UABE Frame Structure Verification
	// ------------------------------------------------------------------------
	fmt.Print("[PROBE 2/6] Binary frame encoding & on-disk Castagnoli checksums... ")

	eng2, err := engine.Open(dbPath, engine.Options{})
	check(err, "open eng2")

	key1 := []byte("forensic_key_alpha")
	val1 := []byte("forensic_val_beta_1234567890")
	check(eng2.Put(key1, val1), "eng2.Put")
	check(eng2.Close(), "close eng2")

	// Inspect raw data.db on disk
	rawBytes, err := os.ReadFile(dbPath)
	check(err, "read raw data.db")

	if len(rawBytes) < 32+28+len(key1)+len(val1)+7+4 {
		fmt.Printf("FAIL: rawBytes too short: %d\n", len(rawBytes))
		os.Exit(1)
	}

	// Verify 32-byte DB Header
	magic := string(rawBytes[0:8])
	if magic != "KVDB2026" {
		fmt.Printf("FAIL: invalid header magic: %q\n", magic)
		os.Exit(1)
	}
	ver := binary.LittleEndian.Uint16(rawBytes[8:10])
	if ver != 1 {
		fmt.Printf("FAIL: invalid header version: %d\n", ver)
		os.Exit(1)
	}
	hdrCRCExpected := binary.LittleEndian.Uint32(rawBytes[28:32])
	hdrCRCActual := crc32.Checksum(rawBytes[0:28], castagnoliTable)
	if hdrCRCExpected != hdrCRCActual {
		fmt.Printf("FAIL: header CRC mismatch: exp %x, got %x\n", hdrCRCExpected, hdrCRCActual)
		os.Exit(1)
	}

	// Verify UABE Frame Header at offset 32
	frameOffset := 32
	frameMagic := binary.LittleEndian.Uint32(rawBytes[frameOffset : frameOffset+4])
	if frameMagic != 0x55414245 {
		fmt.Printf("FAIL: invalid frame magic: %x\n", frameMagic)
		os.Exit(1)
	}
	frameHdrCRCExpected := binary.LittleEndian.Uint32(rawBytes[frameOffset+4 : frameOffset+8])
	frameHdrCRCActual := crc32.Checksum(rawBytes[frameOffset+8:frameOffset+28], castagnoliTable)
	if frameHdrCRCExpected != frameHdrCRCActual {
		fmt.Printf("FAIL: frame header CRC mismatch: exp %x, got %x\n", frameHdrCRCExpected, frameHdrCRCActual)
		os.Exit(1)
	}
	payloadLen := binary.LittleEndian.Uint64(rawBytes[frameOffset+8 : frameOffset+16])
	seqNo := binary.LittleEndian.Uint64(rawBytes[frameOffset+16 : frameOffset+24])
	opCount := binary.LittleEndian.Uint32(rawBytes[frameOffset+24 : frameOffset+28])

	if seqNo != 1 || opCount != 1 {
		fmt.Printf("FAIL: unexpected seqNo %d or opCount %d\n", seqNo, opCount)
		os.Exit(1)
	}

	// Verify Payload and Trailer CRC
	payloadOffset := frameOffset + 28
	payloadBytes := rawBytes[payloadOffset : payloadOffset+int(payloadLen)]
	trailerOffset := payloadOffset + int(payloadLen)
	trailerCRCExpected := binary.LittleEndian.Uint32(rawBytes[trailerOffset : trailerOffset+4])
	trailerCRCActual := crc32.Checksum(payloadBytes, castagnoliTable)
	if trailerCRCExpected != trailerCRCActual {
		fmt.Printf("FAIL: trailer CRC mismatch: exp %x, got %x\n", trailerCRCExpected, trailerCRCActual)
		os.Exit(1)
	}
	fmt.Println("PASSED")

	// ------------------------------------------------------------------------
	// Check 3: Genuine ReadAt Verification via Disk Mutation (Anti-Facade Check)
	// ------------------------------------------------------------------------
	fmt.Print("[PROBE 3/6] Anti-facade check: ReadAt direct physical disk read... ")

	eng3, err := engine.Open(dbPath, engine.Options{})
	check(err, "open eng3")

	// Verify Get returns original value
	valGot, found, err := eng3.Get(key1)
	check(err, "eng3.Get")
	if !found || !bytes.Equal(valGot, val1) {
		fmt.Printf("FAIL: initial Get failed: found=%v, got=%q\n", found, string(valGot))
		os.Exit(1)
	}

	// Now tamper raw disk file directly at the exact value location while engine is open
	// In Bitcask, value offset = frameOffset (32) + 28 + 7 (subrecord header) + len(key1)
	targetValOffset := int64(frameOffset + 28 + 7 + len(key1))
	tamperedBytes := bytes.Repeat([]byte("T"), len(val1))

	fRaw, err := os.OpenFile(dbPath, os.O_RDWR, 0666)
	check(err, "open raw file for direct tamper")
	_, err = fRaw.WriteAt(tamperedBytes, targetValOffset)
	check(err, "write tamper bytes directly to disk")
	check(fRaw.Sync(), "sync raw file")
	_ = fRaw.Close()

	// Call eng3.Get(): It MUST read the tampered bytes directly from disk!
	// If it returned a cached value from a map or dummy return, it would return original val1!
	valTampered, found, err := eng3.Get(key1)
	check(err, "eng3.Get after disk tamper")
	if !found {
		fmt.Println("FAIL: key lost after disk tamper")
		os.Exit(1)
	}
	if !bytes.Equal(valTampered, tamperedBytes) {
		fmt.Printf("FAIL: Get did not read from disk! Got %q, expected tampered %q\n", string(valTampered), string(tamperedBytes))
		os.Exit(1)
	}
	check(eng3.Close(), "close eng3")
	fmt.Println("PASSED (Direct ReadAt Verified)")

	// ------------------------------------------------------------------------
	// Check 4: Append-Only Tombstone Delete Verification
	// ------------------------------------------------------------------------
	fmt.Print("[PROBE 4/6] Append-only tombstone DELETE & non-destructive growth... ")

	eng4, err := engine.Open(dbPath, engine.Options{})
	check(err, "open eng4")

	sizeBefore := eng4.ValidEOF()
	check(eng4.Delete(key1), "eng4.Delete")
	sizeAfter := eng4.ValidEOF()

	if sizeAfter <= sizeBefore {
		fmt.Printf("FAIL: file did not grow on DELETE! sizeBefore=%d, sizeAfter=%d\n", sizeBefore, sizeAfter)
		os.Exit(1)
	}

	valAfterDel, found, err := eng4.Get(key1)
	check(err, "eng4.Get after del")
	if found || valAfterDel != nil {
		fmt.Printf("FAIL: Get found deleted key: %v\n", string(valAfterDel))
		os.Exit(1)
	}
	check(eng4.Close(), "close eng4")

	// Read last frame on disk, verify OpDel
	rawBytes4, err := os.ReadFile(dbPath)
	check(err, "read rawBytes4")
	lastFrameBytes := rawBytes4[sizeBefore:sizeAfter]
	fh4, err := format.DecodeFrameHeader(lastFrameBytes[:format.FrameHeaderSize])
	check(err, "decode delete frame header")
	delPayload := lastFrameBytes[format.FrameHeaderSize : format.FrameHeaderSize+fh4.PayloadLen]
	delTrailer := lastFrameBytes[format.FrameHeaderSize+fh4.PayloadLen:]
	records, err := format.DecodePayloadRecords(delPayload, delTrailer, fh4.OpCount)
	check(err, "decode delete payload")
	if len(records) != 1 || records[0].Type != format.OpDel || !bytes.Equal(records[0].Key, key1) {
		fmt.Printf("FAIL: tombstone record incorrect: %+v\n", records)
		os.Exit(1)
	}
	fmt.Println("PASSED")

	// ------------------------------------------------------------------------
	// Check 5: Dynamic Binary & Random Key/Value Cryptographic Fuzzing
	// ------------------------------------------------------------------------
	fmt.Print("[PROBE 5/6] Dynamic random binary keys & values with special bytes... ")

	dbPathFuzz := filepath.Join(tmpDir, "fuzz.db")
	eng5, err := engine.Open(dbPathFuzz, engine.Options{})
	check(err, "open fuzz db")

	testPairs := make(map[string][]byte)
	for i := 0; i < 50; i++ {
		// Key with null bytes, newlines, binary sequences
		k := make([]byte, 16)
		_, _ = rand.Read(k)
		k[0] = 0x00
		k[len(k)-1] = '\n'

		v := make([]byte, 256)
		_, _ = rand.Read(v)

		check(eng5.Put(k, v), fmt.Sprintf("put random pair %d", i))
		testPairs[string(k)] = v
	}

	// Verify all 50 pairs via Get
	for kStr, expectedV := range testPairs {
		gotV, found, err := eng5.Get([]byte(kStr))
		check(err, "get fuzz pair")
		if !found || !bytes.Equal(gotV, expectedV) {
			fmt.Printf("FAIL: mismatch on binary key: found=%v, lenGot=%d, lenExp=%d\n", found, len(gotV), len(expectedV))
			os.Exit(1)
		}
	}
	check(eng5.Close(), "close eng5")
	fmt.Println("PASSED")

	// ------------------------------------------------------------------------
	// Check 6: Startup Log Replay & Recovery Precision
	// ------------------------------------------------------------------------
	fmt.Print("[PROBE 6/6] Recovery & log replay across process restart... ")

	eng6, err := engine.Open(dbPathFuzz, engine.Options{})
	check(err, "reopen fuzz db")

	if eng6.KeyCount() != len(testPairs) {
		fmt.Printf("FAIL: keyCount mismatch on recovery: exp %d, got %d\n", len(testPairs), eng6.KeyCount())
		os.Exit(1)
	}

	for kStr, expectedV := range testPairs {
		gotV, found, err := eng6.Get([]byte(kStr))
		check(err, "get after recovery")
		if !found || !bytes.Equal(gotV, expectedV) {
			fmt.Println("FAIL: key mismatch after recovery replay")
			os.Exit(1)
		}
	}
	check(eng6.Close(), "close eng6")
	fmt.Println("PASSED")

	fmt.Println("\n=== ALL FORENSIC PROBES PASSED WITH 100% INTEGRITY ===")
}
