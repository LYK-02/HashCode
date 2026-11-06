package format_test

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"kvdb/internal/format"
	"kvdb/internal/testutil"
)

// ============================================================================
// Independent Reference Oracle Implementation (RFC 3720 Castagnoli CRC32C)
// Completely independent of hash/crc32 and internal/format
// ============================================================================

const oracleCastagnoliPoly = uint32(0x82F63B78)

var oracleTable [256]uint32

func init() {
	for i := 0; i < 256; i++ {
		crc := uint32(i)
		for j := 0; j < 8; j++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ oracleCastagnoliPoly
			} else {
				crc >>= 1
			}
		}
		oracleTable[i] = crc
	}
}

// oracleCRC32C computes the 32-bit Castagnoli CRC using bitwise arithmetic.
func oracleCRC32C(data []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, b := range data {
		crc = oracleTable[byte(crc)^b] ^ (crc >> 8)
	}
	return crc ^ 0xFFFFFFFF
}

// oracleBuildDBHeader constructs a 32-byte DB header independently.
func oracleBuildDBHeader(gen uint64, version uint32, reserved uint64) []byte {
	buf := make([]byte, 32)
	copy(buf[0:8], "KVDB2026")
	binary.LittleEndian.PutUint32(buf[8:12], version)
	binary.LittleEndian.PutUint64(buf[12:20], gen)
	binary.LittleEndian.PutUint64(buf[20:28], reserved)
	crc := oracleCRC32C(buf[0:28])
	binary.LittleEndian.PutUint32(buf[28:32], crc)
	return buf
}

type oracleOp struct {
	opType byte
	key    []byte
	val    []byte
}

// oracleBuildFrame constructs a full 28B header + payload + 4B trailer frame independently.
func oracleBuildFrame(seqNo uint64, ops []oracleOp) ([]byte, error) {
	if len(ops) < 1 {
		return nil, errors.New("empty ops")
	}
	var payload []byte
	for _, op := range ops {
		rec := make([]byte, 7+len(op.key)+len(op.val))
		rec[0] = op.opType
		binary.LittleEndian.PutUint16(rec[1:3], uint16(len(op.key)))
		binary.LittleEndian.PutUint32(rec[3:7], uint32(len(op.val)))
		copy(rec[7:7+len(op.key)], op.key)
		if len(op.val) > 0 {
			copy(rec[7+len(op.key):], op.val)
		}
		payload = append(payload, rec...)
	}

	payloadLen := uint64(len(payload))
	hdr := make([]byte, 28)
	binary.LittleEndian.PutUint32(hdr[0:4], 0x55414245)
	binary.LittleEndian.PutUint64(hdr[8:16], payloadLen)
	binary.LittleEndian.PutUint64(hdr[16:24], seqNo)
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(len(ops)))

	hdrCRC := oracleCRC32C(hdr[8:28])
	binary.LittleEndian.PutUint32(hdr[4:8], hdrCRC)

	trailerCRC := oracleCRC32C(payload)
	trailer := make([]byte, 4)
	binary.LittleEndian.PutUint32(trailer, trailerCRC)

	frame := make([]byte, 0, 28+len(payload)+4)
	frame = append(frame, hdr...)
	frame = append(frame, payload...)
	frame = append(frame, trailer...)
	return frame, nil
}

// ============================================================================
// Test 1: Oracle CRC32C RFC 3720 Test Vectors & Cross-Check with format.ChecksumCastagnoli
// ============================================================================

func TestOracleCRC32C_RFC3720(t *testing.T) {
	// RFC 3720 vector: "123456789" -> 0xE3069283
	input := []byte("123456789")
	expected := uint32(0xe3069283)
	if got := oracleCRC32C(input); got != expected {
		t.Fatalf("Oracle CRC32C mismatch for RFC vector: got 0x%08x, want 0x%08x", got, expected)
	}

	// 32 zero bytes -> 0x8A9136AA
	zeros := make([]byte, 32)
	expectedZeros := uint32(0x8a9136aa)
	if got := oracleCRC32C(zeros); got != expectedZeros {
		t.Fatalf("Oracle CRC32C mismatch for 32 zeros: got 0x%08x, want 0x%08x", got, expectedZeros)
	}

	// Cross-verify oracle against format.ChecksumCastagnoli on 1,000 random buffers
	for i := 0; i < 1000; i++ {
		length := i % 512
		buf := make([]byte, length)
		_, _ = rand.Read(buf)
		oracleRes := oracleCRC32C(buf)
		targetRes := format.ChecksumCastagnoli(buf)
		if oracleRes != targetRes {
			t.Fatalf("CRC32C mismatch between oracle (0x%08x) and target (0x%08x) for len %d", oracleRes, targetRes, length)
		}
	}
}

// ============================================================================
// Test 2: Differential Verification of DB Header (Gen 0, Gen 5, & 1000 Random Gens)
// ============================================================================

func TestOracleDifferential_DBHeader(t *testing.T) {
	// 1. Verify Gen 0 matches golden fixture and format.EncodeHeader
	gen0Oracle := oracleBuildDBHeader(0, 1, 0)
	golden0 := testutil.MustDecodeHex(testutil.GoldenHeaderHex)
	if !bytes.Equal(gen0Oracle, golden0) {
		t.Fatalf("Gen 0 oracle does not match golden fixture")
	}

	h0 := format.NewHeader(0)
	gen0Target, err := format.EncodeHeader(&h0)
	if err != nil {
		t.Fatalf("EncodeHeader failed: %v", err)
	}
	if !bytes.Equal(gen0Oracle, gen0Target) {
		t.Fatalf("Gen 0 mismatch: oracle %x vs target %x", gen0Oracle, gen0Target)
	}

	// 2. Verify Gen 5 matches golden fixture and format.EncodeHeader
	gen5Oracle := oracleBuildDBHeader(5, 1, 0)
	golden5 := testutil.MustDecodeHex(testutil.GoldenCompactedHeaderHex)
	if !bytes.Equal(gen5Oracle, golden5) {
		t.Fatalf("Gen 5 oracle does not match golden fixture")
	}

	h5 := format.NewHeader(5)
	gen5Target, err := format.EncodeHeader(&h5)
	if err != nil {
		t.Fatalf("EncodeHeader failed: %v", err)
	}
	if !bytes.Equal(gen5Oracle, gen5Target) {
		t.Fatalf("Gen 5 mismatch: oracle %x vs target %x", gen5Oracle, gen5Target)
	}

	// 3. Verify DecodeHeader accepts oracle outputs
	dec0, err := format.DecodeHeader(gen0Oracle)
	if err != nil {
		t.Fatalf("DecodeHeader rejected oracle gen 0: %v", err)
	}
	if dec0.CompactionGen != 0 || dec0.Version != 1 || dec0.CRC32C != 0x6708c580 {
		t.Fatalf("DecodeHeader parsed incorrect fields for gen 0")
	}

	dec5, err := format.DecodeHeader(gen5Oracle)
	if err != nil {
		t.Fatalf("DecodeHeader rejected oracle gen 5: %v", err)
	}
	if dec5.CompactionGen != 5 || dec5.Version != 1 || dec5.CRC32C != 0x53006495 {
		t.Fatalf("DecodeHeader parsed incorrect fields for gen 5")
	}

	// 4. Test 1,000 random generation numbers
	for i := 0; i < 1000; i++ {
		gen := uint64(i) * 12345678901
		oracleBytes := oracleBuildDBHeader(gen, 1, 0)
		targetHeader := format.NewHeader(gen)
		targetBytes, err := format.EncodeHeader(&targetHeader)
		if err != nil {
			t.Fatalf("EncodeHeader failed on gen %d: %v", gen, err)
		}
		if !bytes.Equal(oracleBytes, targetBytes) {
			t.Fatalf("Byte mismatch on gen %d:\noracle: %x\ntarget: %x", gen, oracleBytes, targetBytes)
		}

		decoded, err := format.DecodeHeader(oracleBytes)
		if err != nil {
			t.Fatalf("DecodeHeader rejected oracle on gen %d: %v", gen, err)
		}
		if decoded.CompactionGen != gen || decoded.Version != 1 || decoded.Reserved != 0 {
			t.Fatalf("Decoded header values mismatch on gen %d", gen)
		}
	}
}

// ============================================================================
// Test 3: Differential Verification of UABE Frames (Golden Vectors & Random Workloads)
// ============================================================================

func TestOracleDifferential_UABEFrames(t *testing.T) {
	testCases := []struct {
		name      string
		seqNo     uint64
		ops       []oracleOp
		targetOps []format.Operation
		goldenHex string
	}{
		{
			name:  "Golden_PUT_HelloWorld",
			seqNo: 1,
			ops:   []oracleOp{{opType: 1, key: []byte("hello"), val: []byte("world")}},
			targetOps: []format.Operation{
				{Type: format.OpPut, Key: []byte("hello"), Value: []byte("world")},
			},
			goldenHex: testutil.GoldenPutFrameHex,
		},
		{
			name:  "Golden_PUT_FooBar",
			seqNo: 1,
			ops:   []oracleOp{{opType: 1, key: []byte("foo"), val: []byte("bar")}},
			targetOps: []format.Operation{
				{Type: format.OpPut, Key: []byte("foo"), Value: []byte("bar")},
			},
			goldenHex: testutil.GoldenPutFooBarFrameHex,
		},
		{
			name:  "Golden_PUT_EmptyValue",
			seqNo: 2,
			ops:   []oracleOp{{opType: 1, key: []byte("foo"), val: []byte{}}},
			targetOps: []format.Operation{
				{Type: format.OpPut, Key: []byte("foo"), Value: []byte{}},
			},
			goldenHex: testutil.GoldenEmptyValPutFrameHex,
		},
		{
			name:  "Golden_DEL_Foo",
			seqNo: 3,
			ops:   []oracleOp{{opType: 2, key: []byte("foo"), val: []byte{}}},
			targetOps: []format.Operation{
				{Type: format.OpDel, Key: []byte("foo"), Value: nil},
			},
			goldenHex: testutil.GoldenDelFrameHex,
		},
		{
			name:  "Golden_Batch_3Ops",
			seqNo: 42,
			ops: []oracleOp{
				{opType: 1, key: []byte("k1"), val: []byte("v1")},
				{opType: 2, key: []byte("k2"), val: []byte{}},
				{opType: 1, key: []byte("k3"), val: []byte{}},
			},
			targetOps: []format.Operation{
				{Type: format.OpPut, Key: []byte("k1"), Value: []byte("v1")},
				{Type: format.OpDel, Key: []byte("k2"), Value: nil},
				{Type: format.OpPut, Key: []byte("k3"), Value: []byte{}},
			},
			goldenHex: testutil.GoldenBatchFrameHex,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oracleFrame, err := oracleBuildFrame(tc.seqNo, tc.ops)
			if err != nil {
				t.Fatalf("oracleBuildFrame failed: %v", err)
			}
			goldenBytes := testutil.MustDecodeHex(tc.goldenHex)
			if !bytes.Equal(oracleFrame, goldenBytes) {
				t.Fatalf("Oracle frame does not match golden fixture:\noracle: %x\ngolden: %x", oracleFrame, goldenBytes)
			}

			targetFrame, err := format.EncodeFrame(tc.seqNo, tc.targetOps)
			if err != nil {
				t.Fatalf("format.EncodeFrame failed: %v", err)
			}
			if !bytes.Equal(oracleFrame, targetFrame) {
				t.Fatalf("Byte-for-byte mismatch between oracle and target:\noracle: %x\ntarget: %x", oracleFrame, targetFrame)
			}

			// Verify target decoder accepts oracle frame
			fh, err := format.DecodeFrameHeader(oracleFrame[:format.FrameHeaderSize])
			if err != nil {
				t.Fatalf("DecodeFrameHeader rejected oracle frame: %v", err)
			}
			if fh.SeqNo != tc.seqNo || fh.OpCount != uint32(len(tc.ops)) {
				t.Fatalf("DecodeFrameHeader header metadata mismatch")
			}

			payload := oracleFrame[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
			trailer := oracleFrame[format.FrameHeaderSize+fh.PayloadLen:]
			decodedOps, err := format.DecodePayloadAndTrailer(payload, trailer, fh.OpCount)
			if err != nil {
				t.Fatalf("DecodePayloadAndTrailer rejected oracle payload/trailer: %v", err)
			}
			if len(decodedOps) != len(tc.targetOps) {
				t.Fatalf("Decoded ops count mismatch: got %d, want %d", len(decodedOps), len(tc.targetOps))
			}
			for i := range decodedOps {
				if byte(decodedOps[i].Type) != tc.ops[i].opType {
					t.Fatalf("op[%d] type mismatch: got %d, want %d", i, decodedOps[i].Type, tc.ops[i].opType)
				}
				if !bytes.Equal(decodedOps[i].Key, tc.ops[i].key) {
					t.Fatalf("op[%d] key mismatch: got %q, want %q", i, decodedOps[i].Key, tc.ops[i].key)
				}
				expectedVal := tc.ops[i].val
				if expectedVal == nil {
					expectedVal = []byte{}
				}
				if !bytes.Equal(decodedOps[i].Value, expectedVal) {
					t.Fatalf("op[%d] val mismatch: got %q, want %q", i, decodedOps[i].Value, expectedVal)
				}
			}
		})
	}
}

// ============================================================================
// Test 4: Randomized Stress Fuzzing & Locator Consistency
// ============================================================================

func TestOracleDifferential_FuzzAndLocators(t *testing.T) {
	// Generate 100 random batches with varying sizes (1..100 ops), random keys (including null bytes), and random values
	for b := 0; b < 100; b++ {
		opCount := (b % 50) + 1
		seqNo := uint64(1000 + b)

		oracleOps := make([]oracleOp, opCount)
		targetOps := make([]format.Operation, opCount)

		for i := 0; i < opCount; i++ {
			isDel := (i%3 == 0)
			keyLen := (i % 64) + 1
			key := make([]byte, keyLen)
			_, _ = rand.Read(key)

			if isDel {
				oracleOps[i] = oracleOp{opType: 2, key: key, val: []byte{}}
				targetOps[i] = format.Operation{Type: format.OpDel, Key: key, Value: nil}
			} else {
				valLen := (i % 256)
				val := make([]byte, valLen)
				_, _ = rand.Read(val)
				oracleOps[i] = oracleOp{opType: 1, key: key, val: val}
				targetOps[i] = format.Operation{Type: format.OpPut, Key: key, Value: val}
			}
		}

		oracleFrame, err := oracleBuildFrame(seqNo, oracleOps)
		if err != nil {
			t.Fatalf("Batch %d: oracleBuildFrame error: %v", b, err)
		}

		targetFrame, err := format.EncodeFrame(seqNo, targetOps)
		if err != nil {
			t.Fatalf("Batch %d: format.EncodeFrame error: %v", b, err)
		}

		// Cross-verify byte-for-byte identity
		if !bytes.Equal(oracleFrame, targetFrame) {
			t.Fatalf("Batch %d: byte-for-byte mismatch between oracle and target (frame len %d)", b, len(oracleFrame))
		}

		// Decode and verify locators
		fh, err := format.DecodeFrameHeader(oracleFrame[:format.FrameHeaderSize])
		if err != nil {
			t.Fatalf("Batch %d: DecodeFrameHeader failed: %v", b, err)
		}

		payload := oracleFrame[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
		trailer := oracleFrame[format.FrameHeaderSize+fh.PayloadLen:]

		records, err := format.DecodePayloadRecords(payload, trailer, fh.OpCount)
		if err != nil {
			t.Fatalf("Batch %d: DecodePayloadRecords failed: %v", b, err)
		}

		// Check locators
		frameStart := uint64(32 + b*100000)
		for i, rec := range records {
			if rec.Type == format.OpPut && rec.ValueLen > 0 {
				valOffset := format.ValueFileOffset(frameStart, rec.PayloadOffset, rec.KeyLen)
				// Compute expected file offset independently:
				expectedOffset := frameStart + 28 + uint64(rec.PayloadOffset) + 7 + uint64(rec.KeyLen)
				if valOffset != expectedOffset {
					t.Fatalf("Batch %d op %d: ValueFileOffset mismatch: got %d, want %d", b, i, valOffset, expectedOffset)
				}

				// Check that bytes at this locator inside the frame match the original value
				frameRelOffset := int(valOffset - frameStart)
				extractedVal := oracleFrame[frameRelOffset : frameRelOffset+int(rec.ValueLen)]
				if !bytes.Equal(extractedVal, oracleOps[i].val) {
					t.Fatalf("Batch %d op %d: extracted locator value mismatch", b, i)
				}
			}
		}
	}
}

// ============================================================================
// Test 5: Exhaustive Single-Bit Corruption Matrix (Adversarial Stress)
// ============================================================================

func TestExhaustiveBitCorruptionMatrix(t *testing.T) {
	// 1. Every single bit in a 32-byte DB Header
	gen0 := oracleBuildDBHeader(0, 1, 0)
	for byteIdx := 0; byteIdx < 32; byteIdx++ {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			corrupt := bytes.Clone(gen0)
			corrupt[byteIdx] ^= (1 << bitIdx)
			_, err := format.DecodeHeader(corrupt)
			if err == nil {
				t.Fatalf("DecodeHeader unexpectedly accepted corrupt header at byte %d, bit %d", byteIdx, bitIdx)
			}
		}
	}

	// 2. Every single bit in a 28-byte UABE Frame Header
	sampleFrame, err := oracleBuildFrame(1, []oracleOp{{opType: 1, key: []byte("k"), val: []byte("v")}})
	if err != nil {
		t.Fatalf("oracleBuildFrame failed: %v", err)
	}

	for byteIdx := 0; byteIdx < 28; byteIdx++ {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			corruptHdr := bytes.Clone(sampleFrame[:28])
			corruptHdr[byteIdx] ^= (1 << bitIdx)
			_, err := format.DecodeFrameHeader(corruptHdr)
			if err == nil {
				t.Fatalf("DecodeFrameHeader unexpectedly accepted corrupt frame header at byte %d, bit %d", byteIdx, bitIdx)
			}
		}
	}

	// 3. Every single bit in the 4-byte Trailer CRC
	payload := sampleFrame[28 : len(sampleFrame)-4]
	trailer := sampleFrame[len(sampleFrame)-4:]
	for byteIdx := 0; byteIdx < 4; byteIdx++ {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			corruptTrailer := bytes.Clone(trailer)
			corruptTrailer[byteIdx] ^= (1 << bitIdx)
			_, err := format.DecodePayloadAndTrailer(payload, corruptTrailer, 1)
			if err == nil {
				t.Fatalf("DecodePayloadAndTrailer unexpectedly accepted corrupt trailer at byte %d, bit %d", byteIdx, bitIdx)
			}
		}
	}

	// 4. Every single bit in the payload
	for byteIdx := 0; byteIdx < len(payload); byteIdx++ {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			corruptPayload := bytes.Clone(payload)
			corruptPayload[byteIdx] ^= (1 << bitIdx)
			_, err := format.DecodePayloadAndTrailer(corruptPayload, trailer, 1)
			if err == nil {
				t.Fatalf("DecodePayloadAndTrailer unexpectedly accepted corrupt payload at byte %d, bit %d", byteIdx, bitIdx)
			}
		}
	}
}

// ============================================================================
// Test 6: Adversarial Boundary & Exploit Probing
// ============================================================================

func TestAdversarialBoundaries(t *testing.T) {
	// Case A: Key length boundary 1..1024
	t.Run("KeyBoundaries", func(t *testing.T) {
		// Key length 0 rejected
		_, err := format.EncodeFrame(1, []format.Operation{{Type: format.OpPut, Key: nil, Value: []byte("v")}})
		if !errors.Is(err, format.ErrKeyEmpty) {
			t.Fatalf("expected ErrKeyEmpty, got %v", err)
		}

		// Key length 1024 accepted
		k1024 := bytes.Repeat([]byte("a"), 1024)
		frame, err := format.EncodeFrame(1, []format.Operation{{Type: format.OpPut, Key: k1024, Value: []byte("v")}})
		if err != nil {
			t.Fatalf("key 1024 should be accepted: %v", err)
		}
		if _, _, err := format.DecodeFullFrame(frame); err != nil {
			t.Fatalf("DecodeFullFrame failed on key 1024: %v", err)
		}

		// Key length 1025 rejected
		k1025 := bytes.Repeat([]byte("a"), 1025)
		_, err = format.EncodeFrame(1, []format.Operation{{Type: format.OpPut, Key: k1025, Value: []byte("v")}})
		if !errors.Is(err, format.ErrKeyTooLarge) {
			t.Fatalf("expected ErrKeyTooLarge, got %v", err)
		}
	})

	// Case B: Value length boundary 0..1MB
	t.Run("ValueBoundaries", func(t *testing.T) {
		// Value length 1MB accepted
		v1mb := make([]byte, 1024*1024)
		frame, err := format.EncodeFrame(1, []format.Operation{{Type: format.OpPut, Key: []byte("k"), Value: v1mb}})
		if err != nil {
			t.Fatalf("value 1MB should be accepted: %v", err)
		}
		if _, _, err := format.DecodeFullFrame(frame); err != nil {
			t.Fatalf("DecodeFullFrame failed on value 1MB: %v", err)
		}

		// Value length 1MB + 1 rejected
		v1mbPlus1 := make([]byte, 1024*1024+1)
		_, err = format.EncodeFrame(1, []format.Operation{{Type: format.OpPut, Key: []byte("k"), Value: v1mbPlus1}})
		if !errors.Is(err, format.ErrValueTooLarge) {
			t.Fatalf("expected ErrValueTooLarge, got %v", err)
		}
	})

	// Case C: DEL cannot have non-zero value
	t.Run("DelWithNonZeroValue", func(t *testing.T) {
		_, err := format.EncodeFrame(1, []format.Operation{{Type: format.OpDel, Key: []byte("k"), Value: []byte("x")}})
		if !errors.Is(err, format.ErrDelNonZeroValue) {
			t.Fatalf("expected ErrDelNonZeroValue, got %v", err)
		}
	})

	// Case D: Batch operations boundary 1..10000
	t.Run("BatchCountBoundaries", func(t *testing.T) {
		// 0 ops rejected
		_, err := format.EncodeFrame(1, []format.Operation{})
		if !errors.Is(err, format.ErrBatchEmpty) {
			t.Fatalf("expected ErrBatchEmpty, got %v", err)
		}

		// 10001 ops rejected
		ops10001 := make([]format.Operation, 10001)
		for i := range ops10001 {
			ops10001[i] = format.Operation{Type: format.OpPut, Key: []byte("k"), Value: []byte{}}
		}
		_, err = format.EncodeFrame(1, ops10001)
		if !errors.Is(err, format.ErrBatchTooLarge) {
			t.Fatalf("expected ErrBatchTooLarge, got %v", err)
		}
	})

	// Case E: Anti-OOM payload bound checks before allocation
	t.Run("AntiOOMChecks", func(t *testing.T) {
		// Craft a valid header CRC with math.MaxUint64 payload length
		hdr := make([]byte, 28)
		binary.LittleEndian.PutUint32(hdr[0:4], 0x55414245)
		binary.LittleEndian.PutUint64(hdr[8:16], math.MaxUint64)
		binary.LittleEndian.PutUint64(hdr[16:24], 1)
		binary.LittleEndian.PutUint32(hdr[24:28], 1)
		binary.LittleEndian.PutUint32(hdr[4:8], oracleCRC32C(hdr[8:28]))

		_, err := format.DecodeFrameHeader(hdr)
		if !errors.Is(err, format.ErrPayloadTooLarge) {
			t.Fatalf("expected ErrPayloadTooLarge on MaxUint64, got %v", err)
		}
	})
}
