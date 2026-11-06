package format_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"testing"

	"kvdb/internal/format"
	"kvdb/internal/testutil"
)

// 1. Castagnoli CRC32C RFC 3720 & Reference Vectors
func TestCastagnoliCRC32C_RFC3720(t *testing.T) {
	// RFC 3720 Section B.4 test vector: "123456789" -> 0xE3069283
	input := []byte("123456789")
	expected := uint32(0xe3069283)
	actual := format.ChecksumCastagnoli(input)
	if actual != expected {
		t.Fatalf("RFC 3720 Castagnoli CRC32C mismatch: got 0x%08x, want 0x%08x", actual, expected)
	}

	// 32 zero bytes -> 0x8A9136AA
	zeros := make([]byte, 32)
	expectedZeros := uint32(0x8a9136aa)
	if actualZeros := format.ChecksumCastagnoli(zeros); actualZeros != expectedZeros {
		t.Fatalf("Castagnoli 32 zeros mismatch: got 0x%08x, want 0x%08x", actualZeros, expectedZeros)
	}
}

// 2. DB Header Golden Fixtures and Round-Trip Tests
func TestEncodeDecodeHeader_Golden(t *testing.T) {
	tests := []struct {
		name        string
		header      format.Header
		goldenHex   string
		expectedCRC uint32
	}{
		{
			name:        "FreshHeader_Gen0",
			header:      format.NewHeader(0),
			goldenHex:   testutil.GoldenHeaderHex,
			expectedCRC: 0x6708c580,
		},
		{
			name:        "CompactedHeader_Gen5",
			header:      format.NewHeader(5),
			goldenHex:   testutil.GoldenCompactedHeaderHex,
			expectedCRC: 0x53006495,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := format.EncodeHeader(&tc.header)
			if err != nil {
				t.Fatalf("EncodeHeader failed: %v", err)
			}
			if len(encoded) != format.HeaderSize {
				t.Fatalf("expected header length %d, got %d", format.HeaderSize, len(encoded))
			}
			if actualHex := hex.EncodeToString(encoded); actualHex != tc.goldenHex {
				t.Fatalf("golden header hex mismatch:\ngot:  %s\nwant: %s", actualHex, tc.goldenHex)
			}

			// Round-trip Decode
			decoded, err := format.DecodeHeader(encoded)
			if err != nil {
				t.Fatalf("DecodeHeader failed: %v", err)
			}
			if decoded.Magic != tc.header.Magic {
				t.Fatalf("decoded magic mismatch: got %v, want %v", decoded.Magic, tc.header.Magic)
			}
			if decoded.Version != tc.header.Version {
				t.Fatalf("decoded version mismatch: got %d, want %d", decoded.Version, tc.header.Version)
			}
			if decoded.CompactionGen != tc.header.CompactionGen {
				t.Fatalf("decoded compaction gen mismatch: got %d, want %d", decoded.CompactionGen, tc.header.CompactionGen)
			}
			if decoded.Reserved != 0 {
				t.Fatalf("decoded reserved must be 0, got %d", decoded.Reserved)
			}
			if decoded.CRC32C != tc.expectedCRC {
				t.Fatalf("decoded CRC32C mismatch: got 0x%08x, want 0x%08x", decoded.CRC32C, tc.expectedCRC)
			}
		})
	}
}

// 3. DB Header Corruption and Validation Tests
func TestDecodeHeader_Corruption(t *testing.T) {
	t.Run("ShortHeader", func(t *testing.T) {
		short := make([]byte, 31)
		_, err := format.DecodeHeader(short)
		if !errors.Is(err, format.ErrHeaderTooShort) {
			t.Fatalf("expected ErrHeaderTooShort, got %v", err)
		}
	})

	t.Run("InvalidMagic", func(t *testing.T) {
		raw := testutil.MustDecodeHex(testutil.GoldenHeaderHex)
		raw[0] = 'J' // "JVDB2026"
		_, err := format.DecodeHeader(raw)
		if !errors.Is(err, format.ErrInvalidHeaderMagic) {
			t.Fatalf("expected ErrInvalidHeaderMagic, got %v", err)
		}
	})

	t.Run("UnsupportedVersion", func(t *testing.T) {
		raw := testutil.MustDecodeHex(testutil.GoldenHeaderHex)
		binary.LittleEndian.PutUint32(raw[8:12], 2)
		_, err := format.DecodeHeader(raw)
		if !errors.Is(err, format.ErrUnsupportedVersion) {
			t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
		}
	})

	t.Run("ReservedNonZero", func(t *testing.T) {
		raw := testutil.MustDecodeHex(testutil.GoldenHeaderHex)
		binary.LittleEndian.PutUint64(raw[20:28], 1)
		_, err := format.DecodeHeader(raw)
		if !errors.Is(err, format.ErrReservedNonZero) {
			t.Fatalf("expected ErrReservedNonZero, got %v", err)
		}
	})

	t.Run("CorruptCRC_BitFlipInCRC", func(t *testing.T) {
		raw := testutil.MustDecodeHex(testutil.GoldenHeaderHex)
		raw[28] ^= 0x01
		_, err := format.DecodeHeader(raw)
		if !errors.Is(err, format.ErrCorruptHeader) {
			t.Fatalf("expected ErrCorruptHeader, got %v", err)
		}
	})

	t.Run("CorruptCRC_BitFlipInBody", func(t *testing.T) {
		raw := testutil.MustDecodeHex(testutil.GoldenHeaderHex)
		raw[12] ^= 0x01 // Bit flip in compaction gen
		_, err := format.DecodeHeader(raw)
		if !errors.Is(err, format.ErrCorruptHeader) {
			t.Fatalf("expected ErrCorruptHeader, got %v", err)
		}
	})
}

// 4. UABE Frame Golden Fixtures & Round-Trip Tests
func TestEncodeDecodeFrame_Golden(t *testing.T) {
	tests := []struct {
		name        string
		seqNo       uint64
		ops         []format.Operation
		goldenHex   string
		expectedLen int
	}{
		{
			name:  "SinglePUT_HelloWorld",
			seqNo: 1,
			ops: []format.Operation{
				{Type: format.OpPut, Key: []byte("hello"), Value: []byte("world")},
			},
			goldenHex:   testutil.GoldenPutFrameHex,
			expectedLen: 49,
		},
		{
			name:  "SinglePUT_FooBar",
			seqNo: 1,
			ops: []format.Operation{
				{Type: format.OpPut, Key: []byte("foo"), Value: []byte("bar")},
			},
			goldenHex:   testutil.GoldenPutFooBarFrameHex,
			expectedLen: 45,
		},
		{
			name:  "SinglePUT_EmptyValue",
			seqNo: 2,
			ops: []format.Operation{
				{Type: format.OpPut, Key: []byte("foo"), Value: []byte{}},
			},
			goldenHex:   testutil.GoldenEmptyValPutFrameHex,
			expectedLen: 42,
		},
		{
			name:  "SingleDEL_Foo",
			seqNo: 3,
			ops: []format.Operation{
				{Type: format.OpDel, Key: []byte("foo"), Value: nil},
			},
			goldenHex:   testutil.GoldenDelFrameHex,
			expectedLen: 42,
		},
		{
			name:  "MultiOpBatch_3Ops",
			seqNo: 42,
			ops: []format.Operation{
				{Type: format.OpPut, Key: []byte("k1"), Value: []byte("v1")},
				{Type: format.OpDel, Key: []byte("k2"), Value: nil},
				{Type: format.OpPut, Key: []byte("k3"), Value: []byte{}},
			},
			goldenHex:   testutil.GoldenBatchFrameHex,
			expectedLen: 61,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := format.EncodeFrame(tc.seqNo, tc.ops)
			if err != nil {
				t.Fatalf("EncodeFrame failed: %v", err)
			}
			if len(encoded) != tc.expectedLen {
				t.Fatalf("expected encoded length %d, got %d", tc.expectedLen, len(encoded))
			}
			if actualHex := hex.EncodeToString(encoded); actualHex != tc.goldenHex {
				t.Fatalf("golden frame hex mismatch:\ngot:  %s\nwant: %s", actualHex, tc.goldenHex)
			}

			// Decode frame header
			fh, err := format.DecodeFrameHeader(encoded[:format.FrameHeaderSize])
			if err != nil {
				t.Fatalf("DecodeFrameHeader failed: %v", err)
			}
			if fh.Magic != format.FrameMagic {
				t.Fatalf("magic mismatch: got 0x%08x, want 0x%08x", fh.Magic, format.FrameMagic)
			}
			if fh.SeqNo != tc.seqNo {
				t.Fatalf("sequence number mismatch: got %d, want %d", fh.SeqNo, tc.seqNo)
			}
			if fh.OpCount != uint32(len(tc.ops)) {
				t.Fatalf("op count mismatch: got %d, want %d", fh.OpCount, len(tc.ops))
			}

			// Decode payload & trailer
			payload := encoded[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
			trailer := encoded[format.FrameHeaderSize+fh.PayloadLen:]
			ops, err := format.DecodePayloadAndTrailer(payload, trailer, fh.OpCount)
			if err != nil {
				t.Fatalf("DecodePayloadAndTrailer failed: %v", err)
			}
			if len(ops) != len(tc.ops) {
				t.Fatalf("decoded ops count mismatch: got %d, want %d", len(ops), len(tc.ops))
			}
			for i := range ops {
				if ops[i].Type != tc.ops[i].Type {
					t.Fatalf("op[%d] type mismatch: got %d, want %d", i, ops[i].Type, tc.ops[i].Type)
				}
				if !bytes.Equal(ops[i].Key, tc.ops[i].Key) {
					t.Fatalf("op[%d] key mismatch: got %q, want %q", i, ops[i].Key, tc.ops[i].Key)
				}
				expectedVal := tc.ops[i].Value
				if expectedVal == nil {
					expectedVal = []byte{}
				}
				if !bytes.Equal(ops[i].Value, expectedVal) {
					t.Fatalf("op[%d] value mismatch: got %q, want %q", i, ops[i].Value, expectedVal)
				}
			}

			// Test DecodeFullFrame helper
			fh2, ops2, err := format.DecodeFullFrame(encoded)
			if err != nil {
				t.Fatalf("DecodeFullFrame failed: %v", err)
			}
			if fh2.SeqNo != tc.seqNo || len(ops2) != len(tc.ops) {
				t.Fatalf("DecodeFullFrame result mismatch")
			}
		})
	}
}

// 5. Key Length Boundary Tests
func TestKeyLengthBoundaries(t *testing.T) {
	t.Run("KeyLen_0_Rejected", func(t *testing.T) {
		_, err := format.EncodeFrame(1, []format.Operation{
			{Type: format.OpPut, Key: []byte{}, Value: []byte("val")},
		})
		if !errors.Is(err, format.ErrKeyEmpty) {
			t.Fatalf("expected ErrKeyEmpty, got %v", err)
		}
	})

	t.Run("KeyLen_1_Accepted", func(t *testing.T) {
		frame, err := format.EncodeFrame(1, []format.Operation{
			{Type: format.OpPut, Key: []byte("k"), Value: []byte("v")},
		})
		if err != nil {
			t.Fatalf("unexpected error for 1-byte key: %v", err)
		}
		if len(frame) == 0 {
			t.Fatal("empty frame returned")
		}
	})

	t.Run("KeyLen_1024_Accepted", func(t *testing.T) {
		maxKey := bytes.Repeat([]byte("k"), 1024)
		frame, err := format.EncodeFrame(1, []format.Operation{
			{Type: format.OpPut, Key: maxKey, Value: []byte("v")},
		})
		if err != nil {
			t.Fatalf("unexpected error for 1024-byte key: %v", err)
		}
		if len(frame) == 0 {
			t.Fatal("empty frame returned")
		}
	})

	t.Run("KeyLen_1025_Rejected", func(t *testing.T) {
		overKey := bytes.Repeat([]byte("k"), 1025)
		_, err := format.EncodeFrame(1, []format.Operation{
			{Type: format.OpPut, Key: overKey, Value: []byte("v")},
		})
		if !errors.Is(err, format.ErrKeyTooLarge) {
			t.Fatalf("expected ErrKeyTooLarge, got %v", err)
		}
	})
}

// 6. Value Length Boundary Tests
func TestValueLengthBoundaries(t *testing.T) {
	t.Run("ValLen_0_AcceptedForPut", func(t *testing.T) {
		frame, err := format.EncodeFrame(1, []format.Operation{
			{Type: format.OpPut, Key: []byte("key"), Value: []byte{}},
		})
		if err != nil {
			t.Fatalf("unexpected error for 0-byte value: %v", err)
		}
		if len(frame) == 0 {
			t.Fatal("empty frame returned")
		}
	})

	t.Run("ValLen_1MB_AcceptedForPut", func(t *testing.T) {
		maxVal := make([]byte, 1024*1024)
		frame, err := format.EncodeFrame(1, []format.Operation{
			{Type: format.OpPut, Key: []byte("key"), Value: maxVal},
		})
		if err != nil {
			t.Fatalf("unexpected error for 1MB value: %v", err)
		}
		expectedLen := format.FrameHeaderSize + format.RecordHeaderSize + 3 + 1024*1024 + format.FrameTrailerSize
		if len(frame) != expectedLen {
			t.Fatalf("expected frame length %d, got %d", expectedLen, len(frame))
		}
	})

	t.Run("ValLen_1MB_Plus1_RejectedForPut", func(t *testing.T) {
		overVal := make([]byte, 1024*1024+1)
		_, err := format.EncodeFrame(1, []format.Operation{
			{Type: format.OpPut, Key: []byte("key"), Value: overVal},
		})
		if !errors.Is(err, format.ErrValueTooLarge) {
			t.Fatalf("expected ErrValueTooLarge, got %v", err)
		}
	})

	t.Run("DelWithNonZeroValue_RejectedOnEncode", func(t *testing.T) {
		_, err := format.EncodeFrame(1, []format.Operation{
			{Type: format.OpDel, Key: []byte("key"), Value: []byte("nonzero")},
		})
		if !errors.Is(err, format.ErrDelNonZeroValue) {
			t.Fatalf("expected ErrDelNonZeroValue, got %v", err)
		}
	})

	t.Run("DelWithNonZeroValue_RejectedOnDecode", func(t *testing.T) {
		// Craft malformed payload: OpType=DEL (0x02), KeyLen=1 ("k"), ValLen=3 ("bad")
		malformedPayload := []byte{
			0x02,       // OpType DEL
			0x01, 0x00, // KeyLen 1
			0x03, 0x00, 0x00, 0x00, // ValLen 3 (INVALID!)
			'k',           // Key
			'b', 'a', 'd', // Value
		}
		fakeTrailer := make([]byte, 4)
		binary.LittleEndian.PutUint32(fakeTrailer, format.ChecksumCastagnoli(malformedPayload))
		_, err := format.DecodePayloadAndTrailer(malformedPayload, fakeTrailer, 1)
		if !errors.Is(err, format.ErrDelNonZeroValue) {
			t.Fatalf("expected ErrDelNonZeroValue on decode, got %v", err)
		}
	})
}

// 7. Batch Operation Count Boundaries
func TestBatchOpCountBoundaries(t *testing.T) {
	t.Run("OpCount_0_Rejected", func(t *testing.T) {
		_, err := format.EncodeFrame(1, []format.Operation{})
		if !errors.Is(err, format.ErrBatchEmpty) {
			t.Fatalf("expected ErrBatchEmpty, got %v", err)
		}
	})

	t.Run("OpCount_10000_Accepted", func(t *testing.T) {
		ops := make([]format.Operation, 10000)
		for i := 0; i < 10000; i++ {
			ops[i] = format.Operation{Type: format.OpPut, Key: []byte("k"), Value: []byte{}}
		}
		frame, err := format.EncodeFrame(1, ops)
		if err != nil {
			t.Fatalf("unexpected error for 10000 ops: %v", err)
		}
		expectedPayload := 10000 * (format.RecordHeaderSize + 1 + 0)
		expectedTotal := format.FrameHeaderSize + expectedPayload + format.FrameTrailerSize
		if len(frame) != expectedTotal {
			t.Fatalf("expected frame size %d, got %d", expectedTotal, len(frame))
		}
	})

	t.Run("OpCount_10001_Rejected", func(t *testing.T) {
		ops := make([]format.Operation, 10001)
		for i := 0; i < 10001; i++ {
			ops[i] = format.Operation{Type: format.OpPut, Key: []byte("k"), Value: []byte{}}
		}
		_, err := format.EncodeFrame(1, ops)
		if !errors.Is(err, format.ErrBatchTooLarge) {
			t.Fatalf("expected ErrBatchTooLarge, got %v", err)
		}
	})
}

// 8. Corruption, Truncation, and Anti-OOM Tests
func TestFrameCorruptionAndAntiOOM(t *testing.T) {
	goldenBytes := testutil.MustDecodeHex(testutil.GoldenPutFrameHex)

	t.Run("ShortFrameHeader", func(t *testing.T) {
		short := goldenBytes[:20]
		_, err := format.DecodeFrameHeader(short)
		if !errors.Is(err, format.ErrFrameHeaderTooShort) {
			t.Fatalf("expected ErrFrameHeaderTooShort, got %v", err)
		}
	})

	t.Run("InvalidFrameMagic", func(t *testing.T) {
		bad := bytes.Clone(goldenBytes)
		bad[0] ^= 0xFF
		_, err := format.DecodeFrameHeader(bad[:format.FrameHeaderSize])
		if !errors.Is(err, format.ErrInvalidFrameMagic) {
			t.Fatalf("expected ErrInvalidFrameMagic, got %v", err)
		}
	})

	t.Run("CorruptFrameHeaderCRC", func(t *testing.T) {
		bad := bytes.Clone(goldenBytes)
		bad[4] ^= 0x01 // Bit flip in Header CRC
		_, err := format.DecodeFrameHeader(bad[:format.FrameHeaderSize])
		if !errors.Is(err, format.ErrCorruptFrameHeader) {
			t.Fatalf("expected ErrCorruptFrameHeader, got %v", err)
		}
	})

	t.Run("PayloadLengthModified_DetectedByHeaderCRC", func(t *testing.T) {
		bad := bytes.Clone(goldenBytes)
		bad[8] ^= 0x01 // Mutate payload length without updating header CRC
		_, err := format.DecodeFrameHeader(bad[:format.FrameHeaderSize])
		if !errors.Is(err, format.ErrCorruptFrameHeader) {
			t.Fatalf("expected ErrCorruptFrameHeader on tampered payload length, got %v", err)
		}
	})

	t.Run("CorruptPayloadByte_DetectedByTrailerCRC", func(t *testing.T) {
		bad := bytes.Clone(goldenBytes)
		bad[35] ^= 0x01 // Mutate 'h' in 'hello'
		fh, err := format.DecodeFrameHeader(bad[:format.FrameHeaderSize])
		if err != nil {
			t.Fatalf("header should be valid, got: %v", err)
		}
		payload := bad[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
		trailer := bad[format.FrameHeaderSize+fh.PayloadLen:]
		_, err = format.DecodePayloadAndTrailer(payload, trailer, fh.OpCount)
		if !errors.Is(err, format.ErrCorruptPayload) {
			t.Fatalf("expected ErrCorruptPayload on tampered payload byte, got %v", err)
		}
	})

	t.Run("CorruptTrailerCRC", func(t *testing.T) {
		bad := bytes.Clone(goldenBytes)
		bad[len(bad)-1] ^= 0x01 // Bit flip in trailer CRC
		fh, err := format.DecodeFrameHeader(bad[:format.FrameHeaderSize])
		if err != nil {
			t.Fatalf("header should be valid, got: %v", err)
		}
		payload := bad[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
		trailer := bad[format.FrameHeaderSize+fh.PayloadLen:]
		_, err = format.DecodePayloadAndTrailer(payload, trailer, fh.OpCount)
		if !errors.Is(err, format.ErrCorruptPayload) {
			t.Fatalf("expected ErrCorruptPayload on tampered trailer CRC, got %v", err)
		}
	})

	t.Run("ShortTrailer", func(t *testing.T) {
		fh, _ := format.DecodeFrameHeader(goldenBytes[:format.FrameHeaderSize])
		payload := goldenBytes[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
		shortTrailer := goldenBytes[format.FrameHeaderSize+fh.PayloadLen : format.FrameHeaderSize+fh.PayloadLen+2]
		_, err := format.DecodePayloadAndTrailer(payload, shortTrailer, fh.OpCount)
		if !errors.Is(err, format.ErrTrailerTooShort) {
			t.Fatalf("expected ErrTrailerTooShort, got %v", err)
		}
	})

	t.Run("TrailingPayloadGarbage", func(t *testing.T) {
		// Valid payload for 1 op + 5 trailing bytes
		op := format.Operation{Type: format.OpPut, Key: []byte("k"), Value: []byte("v")}
		validPayload := make([]byte, format.RecordHeaderSize+len(op.Key)+len(op.Value))
		format.EncodeRecordTo(validPayload, op)
		garbagePayload := append(validPayload, []byte("extra")...)
		trailer := make([]byte, 4)
		binary.LittleEndian.PutUint32(trailer, format.ChecksumCastagnoli(garbagePayload))
		_, err := format.DecodePayloadAndTrailer(garbagePayload, trailer, 1)
		if !errors.Is(err, format.ErrTrailingPayloadBytes) {
			t.Fatalf("expected ErrTrailingPayloadBytes, got %v", err)
		}
	})

	t.Run("AntiOOM_MaxUint64PayloadLength", func(t *testing.T) {
		buf := make([]byte, format.FrameHeaderSize)
		binary.LittleEndian.PutUint32(buf[0:4], format.FrameMagic)
		binary.LittleEndian.PutUint64(buf[8:16], math.MaxUint64)
		binary.LittleEndian.PutUint64(buf[16:24], 1)
		binary.LittleEndian.PutUint32(buf[24:28], 1)
		crc := format.ChecksumCastagnoli(buf[8:28])
		binary.LittleEndian.PutUint32(buf[4:8], crc)

		_, err := format.DecodeFrameHeader(buf)
		if !errors.Is(err, format.ErrPayloadTooLarge) {
			t.Fatalf("expected ErrPayloadTooLarge on math.MaxUint64, got %v", err)
		}
	})

	t.Run("AntiOOM_Exceeds64MB", func(t *testing.T) {
		buf := make([]byte, format.FrameHeaderSize)
		binary.LittleEndian.PutUint32(buf[0:4], format.FrameMagic)
		binary.LittleEndian.PutUint64(buf[8:16], 65*1024*1024)
		binary.LittleEndian.PutUint64(buf[16:24], 1)
		binary.LittleEndian.PutUint32(buf[24:28], 100)
		crc := format.ChecksumCastagnoli(buf[8:28])
		binary.LittleEndian.PutUint32(buf[4:8], crc)

		_, err := format.DecodeFrameHeader(buf)
		if !errors.Is(err, format.ErrPayloadTooLarge) {
			t.Fatalf("expected ErrPayloadTooLarge on 65MB payload, got %v", err)
		}
	})

	t.Run("PayloadLengthUnderflow_LessThanMinOpSize", func(t *testing.T) {
		buf := make([]byte, format.FrameHeaderSize)
		binary.LittleEndian.PutUint32(buf[0:4], format.FrameMagic)
		binary.LittleEndian.PutUint64(buf[8:16], 7) // min for 1 op is 8 bytes
		binary.LittleEndian.PutUint64(buf[16:24], 1)
		binary.LittleEndian.PutUint32(buf[24:28], 1)
		crc := format.ChecksumCastagnoli(buf[8:28])
		binary.LittleEndian.PutUint32(buf[4:8], crc)

		_, err := format.DecodeFrameHeader(buf)
		if !errors.Is(err, format.ErrPayloadLengthOutOfBounds) {
			t.Fatalf("expected ErrPayloadLengthOutOfBounds, got %v", err)
		}
	})
}

// 9. ValueFileOffset Arithmetic Tests
func TestValueFileOffset(t *testing.T) {
	// Frame starts at offset 32 (right after 32B DB Header)
	frameStart := uint64(32)
	payloadOffset := uint32(0)
	keyLen := uint16(5) // key "hello"
	// Expected offset: 32 (frameStart) + 28 (frame header) + 0 (payloadOffset) + 7 (record header) + 5 (keyLen) = 72
	expectedOffset := uint64(72)
	actualOffset := format.ValueFileOffset(frameStart, payloadOffset, keyLen)
	if actualOffset != expectedOffset {
		t.Fatalf("ValueFileOffset mismatch: got %d, want %d", actualOffset, expectedOffset)
	}

	// Second subrecord at payloadOffset 17, keyLen 3
	// Expected: 32 + 28 + 17 + 7 + 3 = 87
	expectedOffset2 := uint64(87)
	actualOffset2 := format.ValueFileOffset(frameStart, 17, 3)
	if actualOffset2 != expectedOffset2 {
		t.Fatalf("ValueFileOffset subrecord 2 mismatch: got %d, want %d", actualOffset2, expectedOffset2)
	}
}

// 10. DecodedRecord Byte Offset Locator Verification
func TestDecodePayloadRecords_Locators(t *testing.T) {
	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("k1"), Value: []byte("val1")},
		{Type: format.OpDel, Key: []byte("k2"), Value: nil},
		{Type: format.OpPut, Key: []byte("k3_long"), Value: []byte("val3_longer")},
	}
	frame, err := format.EncodeFrame(100, ops)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	fh, err := format.DecodeFrameHeader(frame[:format.FrameHeaderSize])
	if err != nil {
		t.Fatalf("DecodeFrameHeader failed: %v", err)
	}

	payload := frame[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
	trailer := frame[format.FrameHeaderSize+fh.PayloadLen:]
	records, err := format.DecodePayloadRecords(payload, trailer, fh.OpCount)
	if err != nil {
		t.Fatalf("DecodePayloadRecords failed: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Verify Record 0: "k1" -> "val1"
	if records[0].PayloadOffset != 0 {
		t.Fatalf("record 0 offset mismatch: got %d, want 0", records[0].PayloadOffset)
	}
	if records[0].KeyLen != 2 || records[0].ValueLen != 4 {
		t.Fatalf("record 0 lens mismatch: keyLen %d, valLen %d", records[0].KeyLen, records[0].ValueLen)
	}

	// Verify Record 1: DEL "k2"
	rec0Len := uint32(format.RecordHeaderSize + 2 + 4) // 13
	if records[1].PayloadOffset != rec0Len {
		t.Fatalf("record 1 offset mismatch: got %d, want %d", records[1].PayloadOffset, rec0Len)
	}
	if records[1].KeyLen != 2 || records[1].ValueLen != 0 {
		t.Fatalf("record 1 lens mismatch: keyLen %d, valLen %d", records[1].KeyLen, records[1].ValueLen)
	}

	// Verify Record 2: "k3_long" -> "val3_longer"
	rec1Len := uint32(format.RecordHeaderSize + 2 + 0) // 9
	if records[2].PayloadOffset != rec0Len+rec1Len {
		t.Fatalf("record 2 offset mismatch: got %d, want %d", records[2].PayloadOffset, rec0Len+rec1Len)
	}
	if records[2].KeyLen != 7 || records[2].ValueLen != 11 {
		t.Fatalf("record 2 lens mismatch: keyLen %d, valLen %d", records[2].KeyLen, records[2].ValueLen)
	}
}
