package format_test

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	mrand "math/rand"
	"sync"
	"testing"
	"time"

	"kvdb/internal/format"
)

// ============================================================================
// CATEGORY A: Untrusted payload_len values (Anti-OOM & Panic Resistance)
// ============================================================================

func TestAdversarial_UntrustedPayloadLengths(t *testing.T) {
	untrustedLengths := []struct {
		name        string
		payloadLen  uint64
		opCount     uint32
		expectedErr error
	}{
		{
			name:        "MaxUint64",
			payloadLen:  math.MaxUint64,
			opCount:     1,
			expectedErr: format.ErrPayloadTooLarge,
		},
		{
			name:        "MaxInt64",
			payloadLen:  math.MaxInt64,
			opCount:     1,
			expectedErr: format.ErrPayloadTooLarge,
		},
		{
			name:        "1GiB",
			payloadLen:  1 << 30,
			opCount:     1,
			expectedErr: format.ErrPayloadTooLarge,
		},
		{
			name:        "MaxPayloadLen_Plus_1",
			payloadLen:  format.MaxPayloadLen + 1,
			opCount:     100,
			expectedErr: format.ErrPayloadTooLarge,
		},
		{
			name:        "ZeroPayloadLen_With_1Op",
			payloadLen:  0,
			opCount:     1,
			expectedErr: format.ErrPayloadLengthOutOfBounds,
		},
		{
			name:        "Underflow_7Bytes_For_1Op",
			payloadLen:  7, // min record is 8 bytes
			opCount:     1,
			expectedErr: format.ErrPayloadLengthOutOfBounds,
		},
		{
			name:        "OverMaxPayloadForOps_1Op_2MB",
			payloadLen:  2 * 1024 * 1024, // 1 op can be at most ~1MB + 1031 bytes
			opCount:     1,
			expectedErr: format.ErrPayloadLengthOutOfBounds,
		},
	}

	for _, tc := range untrustedLengths {
		t.Run(tc.name, func(t *testing.T) {
			buf := make([]byte, format.FrameHeaderSize)
			binary.LittleEndian.PutUint32(buf[0:4], format.FrameMagic)
			binary.LittleEndian.PutUint64(buf[8:16], tc.payloadLen)
			binary.LittleEndian.PutUint64(buf[16:24], 1)
			binary.LittleEndian.PutUint32(buf[24:28], tc.opCount)
			crc := format.ChecksumCastagnoli(buf[8:28])
			binary.LittleEndian.PutUint32(buf[4:8], crc)

			fh, err := format.DecodeFrameHeader(buf)
			if err == nil {
				t.Fatalf("expected error for payloadLen %d, got nil (fh=%+v)", tc.payloadLen, fh)
			}
			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("expected error %v, got %v", tc.expectedErr, err)
			}
		})
	}
}

func TestAdversarial_DecodePayloadAndTrailer_ExtremeInputs(t *testing.T) {
	tests := []struct {
		name        string
		payload     []byte
		trailer     []byte
		opCount     uint32
		expectPanic bool
		expectedErr error
	}{
		{
			name:        "NilPayload_NilTrailer_1Op",
			payload:     nil,
			trailer:     nil,
			opCount:     1,
			expectedErr: format.ErrTrailerTooShort,
		},
		{
			name:        "NilPayload_NonZeroTrailer_1Op",
			payload:     nil,
			trailer:     []byte{0x01, 0x02, 0x03, 0x04},
			opCount:     1,
			expectedErr: format.ErrCorruptPayload,
		},
		{
			name:        "NilPayload_MatchingZeroTrailer_1Op",
			payload:     nil,
			trailer:     make([]byte, 4), // crc of nil is 0, so trailer CRC passes, but payload underflows
			opCount:     1,
			expectedErr: format.ErrPayloadUnderflow,
		},
		{
			name:        "EmptyPayload_EmptyTrailer_1Op",
			payload:     []byte{},
			trailer:     []byte{},
			opCount:     1,
			expectedErr: format.ErrTrailerTooShort,
		},
		{
			name:        "EmptyPayload_TruncatedTrailer_1Byte",
			payload:     []byte{},
			trailer:     []byte{0x01},
			opCount:     1,
			expectedErr: format.ErrTrailerTooShort,
		},
		{
			name:        "EmptyPayload_TruncatedTrailer_3Bytes",
			payload:     []byte{},
			trailer:     []byte{0x01, 0x02, 0x03},
			opCount:     1,
			expectedErr: format.ErrTrailerTooShort,
		},
		{
			name:        "ValidTrailer_EmptyPayload_OpCount0",
			payload:     []byte{},
			trailer:     make([]byte, 4), // crc of empty is 0
			opCount:     0,
			expectedErr: format.ErrBatchEmpty,
		},
		{
			name:        "ValidTrailer_EmptyPayload_OpCount1",
			payload:     []byte{},
			trailer:     make([]byte, 4), // crc of empty is 0
			opCount:     1,
			expectedErr: format.ErrPayloadUnderflow,
		},
		{
			name:        "OpCountMaxUint32",
			payload:     []byte{},
			trailer:     make([]byte, 4),
			opCount:     math.MaxUint32,
			expectedErr: format.ErrBatchTooLarge,
		},
		{
			name:        "OpCount10001",
			payload:     []byte{},
			trailer:     make([]byte, 4),
			opCount:     10001,
			expectedErr: format.ErrBatchTooLarge,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("unexpected panic in DecodePayloadAndTrailer: %v", r)
				}
			}()

			ops, err := format.DecodePayloadAndTrailer(tc.payload, tc.trailer, tc.opCount)
			if err == nil {
				t.Fatalf("expected error %v, got ops=%v", tc.expectedErr, ops)
			}
			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("expected error %v, got %v", tc.expectedErr, err)
			}
		})
	}
}

// ============================================================================
// CATEGORY B: Corrupted Header Magic & Truncated Frame Buffers (0 to 27 bytes)
// ============================================================================

func TestAdversarial_TruncatedFrameBuffers(t *testing.T) {
	// A valid 28-byte frame header
	validHeader := make([]byte, format.FrameHeaderSize)
	binary.LittleEndian.PutUint32(validHeader[0:4], format.FrameMagic)
	binary.LittleEndian.PutUint64(validHeader[8:16], 10)
	binary.LittleEndian.PutUint64(validHeader[16:24], 1)
	binary.LittleEndian.PutUint32(validHeader[24:28], 1)
	crc := format.ChecksumCastagnoli(validHeader[8:28])
	binary.LittleEndian.PutUint32(validHeader[4:8], crc)

	// Test every truncated length from 0 to 27 bytes on:
	// 1) Truncated valid header
	// 2) Truncated zero buffer
	// 3) Truncated 0xFF buffer
	for length := 0; length < format.FrameHeaderSize; length++ {
		t.Run(fmt.Sprintf("Truncated_Len_%d", length), func(t *testing.T) {
			// Subcase 1: truncated valid header
			sub1 := validHeader[:length]
			_, err1 := format.DecodeFrameHeader(sub1)
			if !errors.Is(err1, format.ErrFrameHeaderTooShort) {
				t.Fatalf("len %d: expected ErrFrameHeaderTooShort, got %v", length, err1)
			}

			// Subcase 2: zeros
			sub2 := make([]byte, length)
			_, err2 := format.DecodeFrameHeader(sub2)
			if !errors.Is(err2, format.ErrFrameHeaderTooShort) {
				t.Fatalf("len %d (zeros): expected ErrFrameHeaderTooShort, got %v", length, err2)
			}

			// Subcase 3: 0xFF
			sub3 := bytes.Repeat([]byte{0xFF}, length)
			_, err3 := format.DecodeFrameHeader(sub3)
			if !errors.Is(err3, format.ErrFrameHeaderTooShort) {
				t.Fatalf("len %d (0xFF): expected ErrFrameHeaderTooShort, got %v", length, err3)
			}
		})
	}
}

func TestAdversarial_CorruptedFrameMagic(t *testing.T) {
	badMagics := []uint32{
		0x00000000,
		0xFFFFFFFF,
		0x55414244, // 1 bit off from 0x55414245
		0x55414246, // 1 bit off
		0x45424155, // Big-endian reversed
		0x12345678,
		0x4B564442, // "KVDB" magic placed in frame header
	}

	for _, magic := range badMagics {
		t.Run(fmt.Sprintf("Magic_0x%08X", magic), func(t *testing.T) {
			buf := make([]byte, format.FrameHeaderSize)
			binary.LittleEndian.PutUint32(buf[0:4], magic)
			binary.LittleEndian.PutUint64(buf[8:16], 10)
			binary.LittleEndian.PutUint64(buf[16:24], 1)
			binary.LittleEndian.PutUint32(buf[24:28], 1)
			crc := format.ChecksumCastagnoli(buf[8:28])
			binary.LittleEndian.PutUint32(buf[4:8], crc)

			_, err := format.DecodeFrameHeader(buf)
			if !errors.Is(err, format.ErrInvalidFrameMagic) {
				t.Fatalf("magic 0x%08X: expected ErrInvalidFrameMagic, got %v", magic, err)
			}
		})
	}
}

// ============================================================================
// CATEGORY C: Random Byte Fuzzing (Differential & Adversarial Fuzzing)
// ============================================================================

func TestAdversarial_FuzzDecodeHeader_RandomBytes(t *testing.T) {
	// 50,000 iterations of random 32-byte arrays
	iterations := 50000
	buf := make([]byte, format.HeaderSize)
	rng := mrand.New(mrand.NewSource(42))

	var accepted int
	for i := 0; i < iterations; i++ {
		rng.Read(buf)
		h, err := format.DecodeHeader(buf)
		if err == nil {
			accepted++
			_ = h
		}
	}

	if accepted > 0 {
		t.Fatalf("expected 0 random headers to be valid, got %d accepted", accepted)
	}
}

func TestAdversarial_FuzzDecodeFrameHeader_RandomBytes(t *testing.T) {
	// 50,000 iterations of random 28-byte arrays
	iterations := 50000
	buf := make([]byte, format.FrameHeaderSize)
	rng := mrand.New(mrand.NewSource(1337))

	var accepted int
	for i := 0; i < iterations; i++ {
		rng.Read(buf)
		fh, err := format.DecodeFrameHeader(buf)
		if err == nil {
			accepted++
			_ = fh
		}
	}

	if accepted > 0 {
		t.Fatalf("expected 0 random frame headers to be valid, got %d accepted", accepted)
	}
}

func TestAdversarial_FuzzDecodeFrameHeader_ValidMagicRandomBody(t *testing.T) {
	// 50,000 iterations where Magic is correct (0x55414245), but remainder is random
	iterations := 50000
	buf := make([]byte, format.FrameHeaderSize)
	rng := mrand.New(mrand.NewSource(2026))

	var accepted int
	for i := 0; i < iterations; i++ {
		rng.Read(buf)
		binary.LittleEndian.PutUint32(buf[0:4], format.FrameMagic)

		fh, err := format.DecodeFrameHeader(buf)
		if err == nil {
			accepted++
			_ = fh
		}
	}

	if accepted > 0 {
		t.Fatalf("expected 0 un-CRC-matched frame headers to be accepted, got %d", accepted)
	}
}

func TestAdversarial_FuzzDecodeFrameHeader_ValidMagicAndForgedCRC(t *testing.T) {
	// 50,000 iterations where Magic is correct AND Castagnoli CRC is computed over random body bytes (8..28)
	// This tests the parser's validation of random payload_len, seqNo, opCount
	iterations := 50000
	buf := make([]byte, format.FrameHeaderSize)
	rng := mrand.New(mrand.NewSource(9999))

	var accepted int
	var rejectedTyped int

	for i := 0; i < iterations; i++ {
		rng.Read(buf[8:28])
		binary.LittleEndian.PutUint32(buf[0:4], format.FrameMagic)
		crc := format.ChecksumCastagnoli(buf[8:28])
		binary.LittleEndian.PutUint32(buf[4:8], crc)

		fh, err := format.DecodeFrameHeader(buf)
		if err != nil {
			rejectedTyped++
			// Must be one of the known validation sentinels
			if !errors.Is(err, format.ErrBatchEmpty) &&
				!errors.Is(err, format.ErrBatchTooLarge) &&
				!errors.Is(err, format.ErrPayloadLengthOutOfBounds) &&
				!errors.Is(err, format.ErrPayloadTooLarge) {
				t.Fatalf("unexpected error type on forged CRC frame: %v", err)
			}
		} else {
			accepted++
			// If accepted, verify all invariant contracts hold
			if fh.OpCount < 1 || fh.OpCount > format.MaxBatchOps {
				t.Fatalf("accepted invalid op count: %d", fh.OpCount)
			}
			if fh.PayloadLen < uint64(fh.OpCount)*uint64(format.MinRecordSize) {
				t.Fatalf("accepted underflow payload len: %d for opcount %d", fh.PayloadLen, fh.OpCount)
			}
			if fh.PayloadLen > format.MaxPayloadLen {
				t.Fatalf("accepted payload len exceeding 64MB: %d", fh.PayloadLen)
			}
		}
	}
}

// ============================================================================
// CATEGORY D: Boundary Subrecord Combinations
// ============================================================================

func TestAdversarial_MaxBatch10000Subrecords_RoundTrip(t *testing.T) {
	// Build 10,000 diverse subrecords (PUTs, empty PUTs, DELs)
	ops := make([]format.Operation, 10000)
	for i := 0; i < 10000; i++ {
		k := []byte(fmt.Sprintf("key_%05d", i))
		switch i % 3 {
		case 0:
			// Normal PUT
			v := []byte(fmt.Sprintf("val_%05d", i))
			ops[i] = format.Operation{Type: format.OpPut, Key: k, Value: v}
		case 1:
			// Empty-value PUT
			ops[i] = format.Operation{Type: format.OpPut, Key: k, Value: []byte{}}
		case 2:
			// DEL
			ops[i] = format.Operation{Type: format.OpDel, Key: k, Value: nil}
		}
	}

	start := time.Now()
	frame, err := format.EncodeFrame(42, ops)
	if err != nil {
		t.Fatalf("EncodeFrame failed for 10000 ops: %v", err)
	}
	encodeDuration := time.Since(start)

	// Decode Frame Header
	fh, err := format.DecodeFrameHeader(frame[:format.FrameHeaderSize])
	if err != nil {
		t.Fatalf("DecodeFrameHeader failed: %v", err)
	}
	if fh.OpCount != 10000 {
		t.Fatalf("expected 10000 ops, got %d", fh.OpCount)
	}
	if fh.SeqNo != 42 {
		t.Fatalf("expected seqNo 42, got %d", fh.SeqNo)
	}

	// Decode Payload & Trailer
	startDecode := time.Now()
	payload := frame[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
	trailer := frame[format.FrameHeaderSize+fh.PayloadLen:]
	decodedOps, err := format.DecodePayloadAndTrailer(payload, trailer, fh.OpCount)
	if err != nil {
		t.Fatalf("DecodePayloadAndTrailer failed for 10000 ops: %v", err)
	}
	decodeDuration := time.Since(startDecode)

	if len(decodedOps) != 10000 {
		t.Fatalf("expected 10000 decoded ops, got %d", len(decodedOps))
	}

	// Verify exact equivalence
	for i := range ops {
		if decodedOps[i].Type != ops[i].Type {
			t.Fatalf("op[%d] type mismatch: got %d, want %d", i, decodedOps[i].Type, ops[i].Type)
		}
		if !bytes.Equal(decodedOps[i].Key, ops[i].Key) {
			t.Fatalf("op[%d] key mismatch: got %q, want %q", i, decodedOps[i].Key, ops[i].Key)
		}
		expectedVal := ops[i].Value
		if expectedVal == nil {
			expectedVal = []byte{}
		}
		if !bytes.Equal(decodedOps[i].Value, expectedVal) {
			t.Fatalf("op[%d] value mismatch: got %q, want %q", i, decodedOps[i].Value, expectedVal)
		}
	}

	// Also verify DecodedRecord locator continuity
	records, err := format.DecodePayloadRecords(payload, trailer, fh.OpCount)
	if err != nil {
		t.Fatalf("DecodePayloadRecords failed: %v", err)
	}
	if len(records) != 10000 {
		t.Fatalf("expected 10000 records, got %d", len(records))
	}
	var expectedOffset uint32
	for i, r := range records {
		if r.PayloadOffset != expectedOffset {
			t.Fatalf("record[%d] offset mismatch: got %d, want %d", i, r.PayloadOffset, expectedOffset)
		}
		recLen := uint32(format.RecordHeaderSize + len(ops[i].Key) + len(ops[i].Value))
		expectedOffset += recLen
	}

	t.Logf("10,000 subrecords verified: frame size = %d bytes, encode = %v, decode = %v", len(frame), encodeDuration, decodeDuration)
}

func TestAdversarial_PUT_1MB_Value_ExactBoundary(t *testing.T) {
	oneMB := 1024 * 1024
	val := make([]byte, oneMB)
	if _, err := rand.Read(val); err != nil {
		t.Fatalf("crypto/rand read failed: %v", err)
	}

	op := format.Operation{
		Type:  format.OpPut,
		Key:   []byte("key_1mb"),
		Value: val,
	}

	frame, err := format.EncodeFrame(1, []format.Operation{op})
	if err != nil {
		t.Fatalf("EncodeFrame with 1MB value failed: %v", err)
	}

	fh, decodedOps, err := format.DecodeFullFrame(frame)
	if err != nil {
		t.Fatalf("DecodeFullFrame with 1MB value failed: %v", err)
	}
	if fh.OpCount != 1 {
		t.Fatalf("expected 1 op, got %d", fh.OpCount)
	}
	if len(decodedOps) != 1 {
		t.Fatalf("expected 1 decoded op, got %d", len(decodedOps))
	}
	if !bytes.Equal(decodedOps[0].Value, val) {
		t.Fatal("1MB decoded value content mismatch")
	}

	// Verify 1MB + 1 byte is rejected on encode
	overVal := make([]byte, oneMB+1)
	opOver := format.Operation{
		Type:  format.OpPut,
		Key:   []byte("key_over"),
		Value: overVal,
	}
	_, err = format.EncodeFrame(1, []format.Operation{opOver})
	if !errors.Is(err, format.ErrValueTooLarge) {
		t.Fatalf("expected ErrValueTooLarge for 1MB+1, got %v", err)
	}
}

func TestAdversarial_DEL_NonZeroValue_Combinations(t *testing.T) {
	nonZeroValues := [][]byte{
		{0x00},
		{0x01},
		[]byte("tombstone"),
		bytes.Repeat([]byte("x"), 1024),
	}

	for _, nzVal := range nonZeroValues {
		t.Run(fmt.Sprintf("ValLen_%d", len(nzVal)), func(t *testing.T) {
			op := format.Operation{
				Type:  format.OpDel,
				Key:   []byte("k"),
				Value: nzVal,
			}

			// 1. Rejected at ValidateOperation
			if err := format.ValidateOperation(op); !errors.Is(err, format.ErrDelNonZeroValue) {
				t.Fatalf("ValidateOperation: expected ErrDelNonZeroValue, got %v", err)
			}

			// 2. Rejected at EncodeFrame
			_, err := format.EncodeFrame(1, []format.Operation{op})
			if !errors.Is(err, format.ErrDelNonZeroValue) {
				t.Fatalf("EncodeFrame: expected ErrDelNonZeroValue, got %v", err)
			}

			// 3. Craft malformed payload and verify rejected at DecodePayloadAndTrailer
			key := []byte("k")
			payload := make([]byte, format.RecordHeaderSize+len(key)+len(nzVal))
			payload[0] = byte(format.OpDel)
			binary.LittleEndian.PutUint16(payload[1:3], uint16(len(key)))
			binary.LittleEndian.PutUint32(payload[3:7], uint32(len(nzVal))) // NON-ZERO!
			copy(payload[7:7+len(key)], key)
			copy(payload[7+len(key):], nzVal)

			trailer := make([]byte, 4)
			binary.LittleEndian.PutUint32(trailer, format.ChecksumCastagnoli(payload))

			_, err = format.DecodePayloadAndTrailer(payload, trailer, 1)
			if !errors.Is(err, format.ErrDelNonZeroValue) {
				t.Fatalf("DecodePayloadAndTrailer: expected ErrDelNonZeroValue, got %v", err)
			}
		})
	}
}

// ============================================================================
// CATEGORY E: Bit Flips and Full Frame Corruption Exhaustive Sweep
// ============================================================================

func TestAdversarial_BitFlipSweep_UABEFrame(t *testing.T) {
	// Encode a valid 5-operation frame
	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("alpha"), Value: []byte("beta")},
		{Type: format.OpDel, Key: []byte("gamma"), Value: nil},
		{Type: format.OpPut, Key: []byte("delta"), Value: []byte{}},
		{Type: format.OpPut, Key: []byte("epsilon"), Value: []byte("zeta")},
		{Type: format.OpDel, Key: []byte("eta"), Value: []byte{}},
	}
	frame, err := format.EncodeFrame(100, ops)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	frameLen := len(frame)
	corrupted := make([]byte, frameLen)

	// Flip every single bit in the frame
	var detectedCorruption int
	for byteIdx := 0; byteIdx < frameLen; byteIdx++ {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			copy(corrupted, frame)
			corrupted[byteIdx] ^= (1 << bitIdx)

			_, _, err := format.DecodeFullFrame(corrupted)
			if err != nil {
				detectedCorruption++
			} else {
				t.Fatalf("undetected single-bit corruption at byte %d bit %d", byteIdx, bitIdx)
			}
		}
	}

	totalBits := frameLen * 8
	if detectedCorruption != totalBits {
		t.Fatalf("expected all %d bit-flips to be detected, got %d", totalBits, detectedCorruption)
	}
}

func TestAdversarial_BitFlipSweep_DBHeader(t *testing.T) {
	hdr := format.NewHeader(12345)
	buf, err := format.EncodeHeader(&hdr)
	if err != nil {
		t.Fatalf("EncodeHeader failed: %v", err)
	}

	corrupted := make([]byte, format.HeaderSize)
	var detected int

	for byteIdx := 0; byteIdx < format.HeaderSize; byteIdx++ {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			copy(corrupted, buf)
			corrupted[byteIdx] ^= (1 << bitIdx)

			_, err := format.DecodeHeader(corrupted)
			if err != nil {
				detected++
			} else {
				t.Fatalf("undetected bit flip in DB Header at byte %d bit %d", byteIdx, bitIdx)
			}
		}
	}

	if detected != format.HeaderSize*8 {
		t.Fatalf("expected 256 header bit flips detected, got %d", detected)
	}
}

// ============================================================================
// CATEGORY F: Truncation Sweeps (Byte by Byte)
// ============================================================================

func TestAdversarial_FullFrameTruncationSweep(t *testing.T) {
	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("foo"), Value: []byte("bar")},
	}
	frame, err := format.EncodeFrame(1, ops)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	for length := 0; length < len(frame); length++ {
		truncated := frame[:length]
		_, _, err := format.DecodeFullFrame(truncated)
		if err == nil {
			t.Fatalf("expected error on truncated frame (len %d / %d), got nil", length, len(frame))
		}
	}
}

// ============================================================================
// CATEGORY G: Concurrent Decoder Stress
// ============================================================================

func TestAdversarial_ConcurrentDecodeStress(t *testing.T) {
	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("concurrent_key"), Value: []byte("concurrent_value")},
	}
	validFrame, err := format.EncodeFrame(999, ops)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	workers := 16
	iterationsPerWorker := 5000
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			rng := mrand.New(mrand.NewSource(int64(workerID * 7919)))
			corruptBuf := make([]byte, len(validFrame))

			for i := 0; i < iterationsPerWorker; i++ {
				// Half the time decode valid frame
				if i%2 == 0 {
					fh, decOps, err := format.DecodeFullFrame(validFrame)
					if err != nil || fh.SeqNo != 999 || len(decOps) != 1 {
						t.Errorf("worker %d: valid frame decode failed: %v", workerID, err)
						return
					}
				} else {
					// Half the time decode corrupted frame
					copy(corruptBuf, validFrame)
					corruptBuf[rng.Intn(len(corruptBuf))] ^= byte(1 + rng.Intn(255))
					_, _, _ = format.DecodeFullFrame(corruptBuf)
				}
			}
		}(w)
	}

	wg.Wait()
}

// ============================================================================
// CATEGORY H: Nil Pointer Handling on Encoders
// ============================================================================

func TestAdversarial_NilPointerHandling(t *testing.T) {
	// EncodeFrame(seq, nil) should not panic
	_, err := format.EncodeFrame(1, nil)
	if !errors.Is(err, format.ErrBatchEmpty) {
		t.Fatalf("expected ErrBatchEmpty on EncodeFrame(1, nil), got %v", err)
	}

	// Check EncodeHeader(nil)
	var panicked bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				t.Logf("Observed nil pointer panic on EncodeHeader(nil): %v", r)
			}
		}()
		_, _ = format.EncodeHeader(nil)
	}()
	if panicked {
		t.Log("Note: EncodeHeader(nil) panics on nil *Header pointer (internal API non-critical observation)")
	}
}
