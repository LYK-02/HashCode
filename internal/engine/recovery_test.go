package engine

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kvdb/internal/format"
)

// Helper to write a valid 32-byte header to a file
func writeValidHeader(t *testing.T, f *os.File) {
	t.Helper()
	hdr := format.NewHeader(0)
	hdrBytes, err := format.EncodeHeader(&hdr)
	if err != nil {
		t.Fatalf("EncodeHeader failed: %v", err)
	}
	if _, err := f.WriteAt(hdrBytes, 0); err != nil {
		t.Fatalf("WriteAt failed: %v", err)
	}
}

func TestRecovery_CleanReplay(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "clean.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)

	// Append 3 frames
	ops1 := []format.Operation{{Type: format.OpPut, Key: []byte("k1"), Value: []byte("v1")}}
	ops2 := []format.Operation{{Type: format.OpPut, Key: []byte("k2"), Value: []byte("v2")}}
	ops3 := []format.Operation{{Type: format.OpDel, Key: []byte("k1")}}

	f1, _ := format.EncodeFrame(1, ops1)
	f2, _ := format.EncodeFrame(2, ops2)
	f3, _ := format.EncodeFrame(3, ops3)

	_, _ = f.WriteAt(f1, 32)
	_, _ = f.WriteAt(f2, int64(32+len(f1)))
	_, _ = f.WriteAt(f3, int64(32+len(f1)+len(f2)))
	totalSize := uint64(32 + len(f1) + len(f2) + len(f3))
	_ = f.Sync()

	res, err := RecoverLog(f, totalSize)
	if err != nil {
		t.Fatalf("RecoverLog failed: %v", err)
	}

	if res.ValidEOF != totalSize {
		t.Errorf("ValidEOF mismatch: expected %d, got %d", totalSize, res.ValidEOF)
	}
	if res.NextSeqNo != 4 {
		t.Errorf("NextSeqNo mismatch: expected 4, got %d", res.NextSeqNo)
	}
	if _, ok := res.KeyDir["k1"]; ok {
		t.Errorf("k1 should have been deleted by frame 3")
	}
	if e, ok := res.KeyDir["k2"]; !ok || e.ValueLen != 2 {
		t.Errorf("k2 missing or invalid: %+v", e)
	}
}

func TestRecovery_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)

	res, err := RecoverLog(f, 32)
	if err != nil {
		t.Fatalf("RecoverLog failed: %v", err)
	}
	if res.ValidEOF != 32 {
		t.Errorf("ValidEOF expected 32, got %d", res.ValidEOF)
	}
	if res.NextSeqNo != 1 {
		t.Errorf("NextSeqNo expected 1, got %d", res.NextSeqNo)
	}
	if len(res.KeyDir) != 0 {
		t.Errorf("KeyDir expected empty, got %d entries", len(res.KeyDir))
	}
}

func TestRecovery_HeaderTooShort(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "short.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	_, _ = f.WriteAt([]byte("KVDB2026"), 0)

	_, err = RecoverLog(f, 8)
	if err == nil {
		t.Fatalf("RecoverLog should fail on short file")
	}
	if !errors.Is(err, format.ErrHeaderTooShort) {
		t.Errorf("Expected ErrHeaderTooShort, got %v", err)
	}
}

func TestRecovery_TornTail_ShortHeader_Repaired(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "torn_short_hdr.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)
	ops := []format.Operation{{Type: format.OpPut, Key: []byte("k1"), Value: []byte("v1")}}
	frameBytes, _ := format.EncodeFrame(1, ops)
	_, _ = f.WriteAt(frameBytes, 32)
	validSize := uint64(32 + len(frameBytes))

	// Append 10 partial bytes of next frame header (< 28B)
	tornBytes := []byte{0x55, 0x41, 0x42, 0x45, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	_, _ = f.WriteAt(tornBytes, int64(validSize))
	totalSize := validSize + 10
	_ = f.Sync()

	res, err := RecoverLog(f, totalSize)
	if err != nil {
		t.Fatalf("RecoverLog failed: %v", err)
	}
	if !res.TornTailFixed {
		t.Errorf("TornTailFixed should be true")
	}
	if res.ValidEOF != validSize {
		t.Errorf("ValidEOF not truncated back: expected %d, got %d", validSize, res.ValidEOF)
	}
	fi, _ := f.Stat()
	if uint64(fi.Size()) != validSize {
		t.Errorf("Physical file size not truncated: expected %d, got %d", validSize, fi.Size())
	}
}

func TestRecovery_TornTail_PartialPayload_Repaired(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "torn_payload.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)
	validOps := []format.Operation{{Type: format.OpPut, Key: []byte("k1"), Value: []byte("v1")}}
	f1, _ := format.EncodeFrame(1, validOps)
	_, _ = f.WriteAt(f1, 32)
	validSize := uint64(32 + len(f1))

	// Construct torn frame declaring 200B payload, write header + only 30B
	tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("torn"), Value: bytes.Repeat([]byte("X"), 180)}}
	fullTornFrame, _ := format.EncodeFrame(2, tornOps)
	partialTorn := fullTornFrame[:format.FrameHeaderSize+30]

	_, _ = f.WriteAt(partialTorn, int64(validSize))
	totalSize := validSize + uint64(len(partialTorn))
	_ = f.Sync()

	res, err := RecoverLog(f, totalSize)
	if err != nil {
		t.Fatalf("RecoverLog failed: %v", err)
	}
	if res.ValidEOF != validSize {
		t.Errorf("ValidEOF mismatch: expected %d, got %d", validSize, res.ValidEOF)
	}
	if _, found := res.KeyDir["torn"]; found {
		t.Errorf("Torn key must not be present in KeyDir")
	}
}

func TestRecovery_TornTail_CorruptTrailerCRC_Repaired(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "corrupt_trailer_tail.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)
	f1, _ := format.EncodeFrame(1, []format.Operation{{Type: format.OpPut, Key: []byte("k1"), Value: []byte("v1")}})
	_, _ = f.WriteAt(f1, 32)
	validSize := uint64(32 + len(f1))

	f2, _ := format.EncodeFrame(2, []format.Operation{{Type: format.OpPut, Key: []byte("k2"), Value: []byte("v2")}})
	// Flip bit in trailer CRC of f2
	f2[len(f2)-1] ^= 0x01
	_, _ = f.WriteAt(f2, int64(validSize))
	totalSize := validSize + uint64(len(f2))
	_ = f.Sync()

	res, err := RecoverLog(f, totalSize)
	if err != nil {
		t.Fatalf("RecoverLog failed: %v", err)
	}
	if res.ValidEOF != validSize {
		t.Errorf("ValidEOF mismatch: expected %d, got %d", validSize, res.ValidEOF)
	}
	if _, found := res.KeyDir["k2"]; found {
		t.Errorf("Corrupt tail k2 must not be present in KeyDir")
	}
}

func TestRecovery_FailClosed_MiddleCorruption_Magic(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "middle_magic.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)
	f1, _ := format.EncodeFrame(1, []format.Operation{{Type: format.OpPut, Key: []byte("k1"), Value: []byte("v1")}})
	f2, _ := format.EncodeFrame(2, []format.Operation{{Type: format.OpPut, Key: []byte("k2"), Value: []byte("v2")}})

	// Corrupt magic byte of f1 at offset 32
	f1[0] = 0xFF
	_, _ = f.WriteAt(f1, 32)
	_, _ = f.WriteAt(f2, int64(32+len(f1)))
	totalSize := uint64(32 + len(f1) + len(f2))
	_ = f.Sync()

	_, err = RecoverLog(f, totalSize)
	if err == nil {
		t.Fatalf("RecoverLog should fail closed on middle corruption")
	}
	if !errors.Is(err, format.ErrInvalidFrameMagic) && !errors.Is(err, format.ErrCorruptFrameHeader) {
		t.Errorf("Unexpected error: %v", err)
	}

	// File MUST NOT be truncated!
	fi, _ := f.Stat()
	if uint64(fi.Size()) != totalSize {
		t.Errorf("File inappropriately truncated: expected %d, got %d", totalSize, fi.Size())
	}
}

func TestRecovery_FailClosed_MiddleCorruption_CRC(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "middle_crc.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)
	f1, _ := format.EncodeFrame(1, []format.Operation{{Type: format.OpPut, Key: []byte("k1"), Value: []byte("v1")}})
	f2, _ := format.EncodeFrame(2, []format.Operation{{Type: format.OpPut, Key: []byte("k2"), Value: []byte("v2")}})

	// Corrupt header CRC byte of f1 at offset 36 (byte 4 of frame)
	f1[4] ^= 0xFF
	_, _ = f.WriteAt(f1, 32)
	_, _ = f.WriteAt(f2, int64(32+len(f1)))
	totalSize := uint64(32 + len(f1) + len(f2))
	_ = f.Sync()

	_, err = RecoverLog(f, totalSize)
	if err == nil {
		t.Fatalf("RecoverLog should fail closed on middle header CRC corruption")
	}
	if !errors.Is(err, format.ErrCorruptFrameHeader) {
		t.Errorf("Unexpected error: %v", err)
	}

	// File MUST NOT be truncated!
	fi, _ := f.Stat()
	if uint64(fi.Size()) != totalSize {
		t.Errorf("File inappropriately truncated: expected %d, got %d", totalSize, fi.Size())
	}
}

func TestRecovery_FailClosed_MiddleCorruption_Payload(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "middle_payload.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)
	f1, _ := format.EncodeFrame(1, []format.Operation{{Type: format.OpPut, Key: []byte("k1"), Value: []byte("long_value_here")}})
	f2, _ := format.EncodeFrame(2, []format.Operation{{Type: format.OpPut, Key: []byte("k2"), Value: []byte("v2")}})

	// Corrupt payload byte in f1
	f1[format.FrameHeaderSize+2] ^= 0x01
	_, _ = f.WriteAt(f1, 32)
	_, _ = f.WriteAt(f2, int64(32+len(f1)))
	totalSize := uint64(32 + len(f1) + len(f2))
	_ = f.Sync()

	_, err = RecoverLog(f, totalSize)
	if err == nil {
		t.Fatalf("RecoverLog should fail closed on middle payload corruption")
	}
	if !errors.Is(err, format.ErrCorruptPayload) {
		t.Errorf("Expected ErrCorruptPayload, got: %v", err)
	}

	fi, _ := f.Stat()
	if uint64(fi.Size()) != totalSize {
		t.Errorf("File inappropriately truncated: expected %d, got %d", totalSize, fi.Size())
	}
}

func TestRecovery_BatchOrdering_PutThenDel(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "order.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)
	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("x"), Value: []byte("val1")},
		{Type: format.OpDel, Key: []byte("x")},
	}
	frameBytes, _ := format.EncodeFrame(1, ops)
	_, _ = f.WriteAt(frameBytes, 32)
	totalSize := uint64(32 + len(frameBytes))
	_ = f.Sync()

	res, err := RecoverLog(f, totalSize)
	if err != nil {
		t.Fatalf("RecoverLog failed: %v", err)
	}
	if _, ok := res.KeyDir["x"]; ok {
		t.Errorf("Key 'x' should be absent after PUT then DEL in same batch")
	}
}

func TestRecovery_Idempotence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "idempotent.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)
	ops := []format.Operation{{Type: format.OpPut, Key: []byte("k"), Value: []byte("v")}}
	frameBytes, _ := format.EncodeFrame(1, ops)
	_, _ = f.WriteAt(frameBytes, 32)
	validSize := uint64(32 + len(frameBytes))

	// Append 5 garbage bytes at EOF
	_, _ = f.WriteAt([]byte("EXTRA"), int64(validSize))
	totalSize := validSize + 5
	_ = f.Sync()

	// First recovery
	res1, err := RecoverLog(f, totalSize)
	if err != nil {
		t.Fatalf("First RecoverLog failed: %v", err)
	}
	if res1.ValidEOF != validSize {
		t.Errorf("First recovery ValidEOF mismatch: expected %d, got %d", validSize, res1.ValidEOF)
	}

	// Second recovery on the repaired file
	fi, _ := f.Stat()
	res2, err := RecoverLog(f, uint64(fi.Size()))
	if err != nil {
		t.Fatalf("Second RecoverLog failed: %v", err)
	}
	if res2.ValidEOF != validSize {
		t.Errorf("Second recovery ValidEOF mismatch: expected %d, got %d", validSize, res2.ValidEOF)
	}
	if len(res2.KeyDir) != len(res1.KeyDir) {
		t.Errorf("KeyDir count mismatch across idempotent runs")
	}
}

func TestRecovery_ForwardScanner_LargeChunk(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "large_scan.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)

	// Write 70 KiB of garbage after header (offset 32..32+70*1024)
	garbageSize := 70 * 1024
	garbage := bytes.Repeat([]byte{0xAA}, garbageSize)
	_, _ = f.WriteAt(garbage, 32)

	// Write a valid frame after garbage
	validOps := []format.Operation{{Type: format.OpPut, Key: []byte("k_far"), Value: []byte("v_far")}}
	fValid, _ := format.EncodeFrame(1, validOps)
	validOffset := int64(32 + garbageSize)
	_, _ = f.WriteAt(fValid, validOffset)
	totalSize := uint64(validOffset + int64(len(fValid)))
	_ = f.Sync()

	// hasValidFrameAhead from offset 32 must detect the frame at 32 + 70 KiB!
	hasAhead, err := hasValidFrameAhead(f, 32, totalSize)
	if err != nil {
		t.Fatalf("hasValidFrameAhead error: %v", err)
	}
	if !hasAhead {
		t.Fatalf("hasValidFrameAhead failed to detect valid frame across 64 KiB chunk boundary")
	}

	// But from after the frame, it should return false
	hasAheadAfter, err := hasValidFrameAhead(f, totalSize, totalSize)
	if err != nil {
		t.Fatalf("hasValidFrameAhead error: %v", err)
	}
	if hasAheadAfter {
		t.Fatalf("hasValidFrameAhead returned true when at EOF")
	}
}

func TestRecovery_FailClosed_MiddleCorruption_Payload_TrailingGarbage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "middle_payload_garbage.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)
	f1, _ := format.EncodeFrame(1, []format.Operation{{Type: format.OpPut, Key: []byte("k1"), Value: []byte("payload_val")}})

	// Corrupt payload byte in f1
	f1[format.FrameHeaderSize+2] ^= 0x01
	_, _ = f.WriteAt(f1, 32)

	// Append 10 trailing non-frame garbage bytes
	garbage := bytes.Repeat([]byte{0x77}, 10)
	_, _ = f.WriteAt(garbage, int64(32+len(f1)))
	totalSize := uint64(32 + len(f1) + len(garbage))
	_ = f.Sync()

	_, err = RecoverLog(f, totalSize)
	if err == nil {
		t.Fatalf("RecoverLog should fail closed on middle payload corruption with trailing garbage")
	}
	if !errors.Is(err, format.ErrCorruptPayload) {
		t.Errorf("Expected ErrCorruptPayload, got: %v", err)
	}

	fi, _ := f.Stat()
	if uint64(fi.Size()) != totalSize {
		t.Errorf("File inappropriately truncated: expected %d, got %d", totalSize, fi.Size())
	}
}

func TestRecovery_FailClosed_MiddleCorruption_TrailerCRC_TrailingGarbage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "middle_trailer_garbage.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)
	f1, _ := format.EncodeFrame(1, []format.Operation{{Type: format.OpPut, Key: []byte("k1"), Value: []byte("trailer_val")}})

	// Corrupt trailer CRC in f1
	f1[len(f1)-1] ^= 0x01
	_, _ = f.WriteAt(f1, 32)

	// Append 10 trailing non-frame garbage bytes
	garbage := bytes.Repeat([]byte{0x88}, 10)
	_, _ = f.WriteAt(garbage, int64(32+len(f1)))
	totalSize := uint64(32 + len(f1) + len(garbage))
	_ = f.Sync()

	_, err = RecoverLog(f, totalSize)
	if err == nil {
		t.Fatalf("RecoverLog should fail closed on middle trailer CRC corruption with trailing garbage")
	}
	if !errors.Is(err, format.ErrCorruptPayload) {
		t.Errorf("Expected ErrCorruptPayload, got: %v", err)
	}

	fi, _ := f.Stat()
	if uint64(fi.Size()) != totalSize {
		t.Errorf("File inappropriately truncated: expected %d, got %d", totalSize, fi.Size())
	}
}

func TestRecovery_TornTail_NestedMagicInPartialPayload_RepairedCleanly(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "torn_nested_magic.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)
	f1, _ := format.EncodeFrame(1, []format.Operation{{Type: format.OpPut, Key: []byte("base_k"), Value: []byte("base_v")}})
	_, _ = f.WriteAt(f1, 32)
	validOffset := uint64(32 + len(f1))

	// Frame declaring 100-byte payload (op with 10-byte key and 83-byte value = 100 byte payload)
	tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("0123456789"), Value: make([]byte, 83)}}
	fullTornFrame, err := format.EncodeFrame(2, tornOps)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}
	fh, _ := format.DecodeFrameHeader(fullTornFrame[:format.FrameHeaderSize])
	if fh.PayloadLen != 100 {
		t.Fatalf("Expected 100-byte payload, got %d", fh.PayloadLen)
	}

	// Write only 30 bytes of the payload to disk (total partial frame = 28 + 30 = 58 bytes)
	partialTorn := make([]byte, format.FrameHeaderSize+30)
	copy(partialTorn, fullTornFrame[:format.FrameHeaderSize+30])

	// Inject nested FrameMagic (0x55414245) inside the partial payload
	// Embedding a valid candidate frame header to verify hasValidFrameAhead is not mistakenly triggered
	candidateFrame, _ := format.EncodeFrame(99, []format.Operation{{Type: format.OpPut, Key: []byte("k"), Value: []byte("v")}})
	copy(partialTorn[format.FrameHeaderSize:format.FrameHeaderSize+28], candidateFrame[:28])

	_, _ = f.WriteAt(partialTorn, int64(validOffset))
	totalSize := validOffset + uint64(len(partialTorn))
	_ = f.Sync()

	res, err := RecoverLog(f, totalSize)
	if err != nil {
		t.Fatalf("RecoverLog should not fail on torn tail with nested magic: %v", err)
	}
	if !res.TornTailFixed {
		t.Errorf("TornTailFixed should be true")
	}
	if res.ValidEOF != validOffset {
		t.Errorf("ValidEOF mismatch: expected %d, got %d", validOffset, res.ValidEOF)
	}

	fi, _ := f.Stat()
	if uint64(fi.Size()) != validOffset {
		t.Errorf("Physical file size not truncated cleanly: expected %d, got %d", validOffset, fi.Size())
	}
	if _, ok := res.KeyDir["base_k"]; !ok {
		t.Errorf("base_k missing from KeyDir")
	}
	if _, ok := res.KeyDir["0123456789"]; ok {
		t.Errorf("torn key should not be present in KeyDir")
	}
}


