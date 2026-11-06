package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kvdb/internal/format"
)

// Helper to calculate SHA-256 of an open file
func fileSHA256(t *testing.T, f *os.File) [32]byte {
	t.Helper()
	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat failed in fileSHA256: %v", err)
	}
	buf := make([]byte, fi.Size())
	if _, err := f.ReadAt(buf, 0); err != nil {
		t.Fatalf("ReadAt failed in fileSHA256: %v", err)
	}
	return sha256.Sum256(buf)
}

// ----------------------------------------------------------------------------
// 1. Torn Tail Truncation at EOF across ALL Permutations
// ----------------------------------------------------------------------------

func TestChallengerM3_2_TornTail_EOF_AllPermutations(t *testing.T) {
	cases := []struct {
		name        string
		setupTorn   func(t *testing.T, f *os.File, validSize uint64) (totalSize uint64)
		expectedKey string // key that was partially written and must NOT be recovered
	}{
		{
			name: "1Byte_Trailing_Garbage",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				_, _ = f.WriteAt([]byte{0x7F}, int64(validSize))
				return validSize + 1
			},
			expectedKey: "",
		},
		{
			name: "PartialHeader_1Byte",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				_, _ = f.WriteAt([]byte{0x45}, int64(validSize))
				return validSize + 1
			},
			expectedKey: "",
		},
		{
			name: "PartialHeader_4Bytes_MagicOnly",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				_, _ = f.WriteAt(frameMagicBytes, int64(validSize))
				return validSize + 4
			},
			expectedKey: "",
		},
		{
			name: "PartialHeader_8Bytes_MagicAndCRC",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				var b [8]byte
				copy(b[:4], frameMagicBytes)
				binary.LittleEndian.PutUint32(b[4:8], 0x12345678)
				_, _ = f.WriteAt(b[:], int64(validSize))
				return validSize + 8
			},
			expectedKey: "",
		},
		{
			name: "PartialHeader_16Bytes",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				tornHdr := make([]byte, 16)
				copy(tornHdr[:4], frameMagicBytes)
				_, _ = f.WriteAt(tornHdr, int64(validSize))
				return validSize + 16
			},
			expectedKey: "",
		},
		{
			name: "PartialHeader_27Bytes_OneByteShort",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("torn_27"), Value: []byte("val")}}
				fullFrame, _ := format.EncodeFrame(99, tornOps)
				_, _ = f.WriteAt(fullFrame[:27], int64(validSize))
				return validSize + 27
			},
			expectedKey: "torn_27",
		},
		{
			name: "CorruptHeader_AtEOF_BadMagic",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("torn_magic"), Value: []byte("val")}}
				fullFrame, _ := format.EncodeFrame(99, tornOps)
				fullFrame[0] ^= 0xFF // corrupt magic
				_, _ = f.WriteAt(fullFrame[:format.FrameHeaderSize], int64(validSize))
				return validSize + uint64(format.FrameHeaderSize)
			},
			expectedKey: "torn_magic",
		},
		{
			name: "CorruptHeader_AtEOF_BadCRC",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("torn_crc"), Value: []byte("val")}}
				fullFrame, _ := format.EncodeFrame(99, tornOps)
				fullFrame[5] ^= 0xAA // corrupt header CRC
				_, _ = f.WriteAt(fullFrame[:format.FrameHeaderSize], int64(validSize))
				return validSize + uint64(format.FrameHeaderSize)
			},
			expectedKey: "torn_crc",
		},
		{
			name: "PartialPayload_0Bytes_HeaderOnly",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("torn_p0"), Value: bytes.Repeat([]byte("A"), 50)}}
				fullFrame, _ := format.EncodeFrame(99, tornOps)
				_, _ = f.WriteAt(fullFrame[:format.FrameHeaderSize], int64(validSize))
				return validSize + uint64(format.FrameHeaderSize)
			},
			expectedKey: "torn_p0",
		},
		{
			name: "PartialPayload_10BytesWritten",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("torn_p10"), Value: bytes.Repeat([]byte("B"), 80)}}
				fullFrame, _ := format.EncodeFrame(99, tornOps)
				_, _ = f.WriteAt(fullFrame[:format.FrameHeaderSize+10], int64(validSize))
				return validSize + uint64(format.FrameHeaderSize+10)
			},
			expectedKey: "torn_p10",
		},
		{
			name: "PartialPayload_FullPayloadWritten_MissingTrailer",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("torn_notrailer"), Value: []byte("v")}}
				fullFrame, _ := format.EncodeFrame(99, tornOps)
				payloadLen := len(fullFrame) - format.FrameTrailerSize
				_, _ = f.WriteAt(fullFrame[:payloadLen], int64(validSize))
				return validSize + uint64(payloadLen)
			},
			expectedKey: "torn_notrailer",
		},
		{
			name: "TruncatedTrailer_1ByteWritten",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("torn_tr1"), Value: []byte("v")}}
				fullFrame, _ := format.EncodeFrame(99, tornOps)
				trailerPartial := len(fullFrame) - format.FrameTrailerSize + 1
				_, _ = f.WriteAt(fullFrame[:trailerPartial], int64(validSize))
				return validSize + uint64(trailerPartial)
			},
			expectedKey: "torn_tr1",
		},
		{
			name: "TruncatedTrailer_3BytesWritten",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("torn_tr3"), Value: []byte("v")}}
				fullFrame, _ := format.EncodeFrame(99, tornOps)
				trailerPartial := len(fullFrame) - format.FrameTrailerSize + 3
				_, _ = f.WriteAt(fullFrame[:trailerPartial], int64(validSize))
				return validSize + uint64(trailerPartial)
			},
			expectedKey: "torn_tr3",
		},
		{
			name: "CorruptTrailerCRC_BitFlipByte0",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("torn_crc0"), Value: []byte("v")}}
				fullFrame, _ := format.EncodeFrame(99, tornOps)
				fullFrame[len(fullFrame)-4] ^= 0x01
				_, _ = f.WriteAt(fullFrame, int64(validSize))
				return validSize + uint64(len(fullFrame))
			},
			expectedKey: "torn_crc0",
		},
		{
			name: "CorruptTrailerCRC_BitFlipByte3",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("torn_crc3"), Value: []byte("v")}}
				fullFrame, _ := format.EncodeFrame(99, tornOps)
				fullFrame[len(fullFrame)-1] ^= 0x80
				_, _ = f.WriteAt(fullFrame, int64(validSize))
				return validSize + uint64(len(fullFrame))
			},
			expectedKey: "torn_crc3",
		},
		{
			name: "CorruptPayload_BitFlipInValue",
			setupTorn: func(t *testing.T, f *os.File, validSize uint64) uint64 {
				tornOps := []format.Operation{{Type: format.OpPut, Key: []byte("torn_valflip"), Value: []byte("intact_val")}}
				fullFrame, _ := format.EncodeFrame(99, tornOps)
				fullFrame[format.FrameHeaderSize+10] ^= 0x02
				_, _ = f.WriteAt(fullFrame, int64(validSize))
				return validSize + uint64(len(fullFrame))
			},
			expectedKey: "torn_valflip",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "torn_tail.db")

			f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
			if err != nil {
				t.Fatalf("OpenFile failed: %v", err)
			}
			defer f.Close()

			writeValidHeader(t, f)

			// Write 3 valid baseline frames
			ops1 := []format.Operation{{Type: format.OpPut, Key: []byte("k1"), Value: []byte("v1")}}
			ops2 := []format.Operation{{Type: format.OpPut, Key: []byte("k2"), Value: []byte("v2")}}
			ops3 := []format.Operation{{Type: format.OpPut, Key: []byte("k3"), Value: []byte("v3")}}

			f1, _ := format.EncodeFrame(1, ops1)
			f2, _ := format.EncodeFrame(2, ops2)
			f3, _ := format.EncodeFrame(3, ops3)

			_, _ = f.WriteAt(f1, 32)
			_, _ = f.WriteAt(f2, int64(32+len(f1)))
			_, _ = f.WriteAt(f3, int64(32+len(f1)+len(f2)))
			validSize := uint64(32 + len(f1) + len(f2) + len(f3))

			// Inject the specific torn tail
			totalSize := tc.setupTorn(t, f, validSize)
			_ = f.Sync()

			// Run recovery
			res, err := RecoverLog(f, totalSize)
			if err != nil {
				t.Fatalf("RecoverLog failed unexpectedly on torn tail: %v", err)
			}

			// Invariant 1: Torn tail was repaired
			if !res.TornTailFixed {
				t.Errorf("Expected TornTailFixed == true")
			}

			// Invariant 2: ValidEOF truncated back to validSize
			if res.ValidEOF != validSize {
				t.Errorf("ValidEOF mismatch: expected %d, got %d", validSize, res.ValidEOF)
			}

			// Invariant 3: Physical file truncated on disk
			fi, err := f.Stat()
			if err != nil {
				t.Fatalf("Stat failed: %v", err)
			}
			if uint64(fi.Size()) != validSize {
				t.Errorf("Physical file size mismatch: expected %d, got %d", validSize, fi.Size())
			}

			// Invariant 4: Prior valid keys are intact
			for _, k := range []string{"k1", "k2", "k3"} {
				if _, ok := res.KeyDir[k]; !ok {
					t.Errorf("Committed key %s missing from recovered KeyDir", k)
				}
			}

			// Invariant 5: Torn key must NOT be present
			if tc.expectedKey != "" {
				if _, ok := res.KeyDir[tc.expectedKey]; ok {
					t.Errorf("Torn key %s unexpectedly present in KeyDir!", tc.expectedKey)
				}
			}
		})
	}
}

// ----------------------------------------------------------------------------
// 2. Strict Fail-Closed Rejection on Middle-of-File Corruption
// ----------------------------------------------------------------------------

func TestChallengerM3_2_MiddleCorruption_FailClosed_AllPermutations(t *testing.T) {
	cases := []struct {
		name          string
		corruptFrame1 func(f1 []byte)
		expectedErr   error
	}{
		{
			name: "Magic_Byte0_Corrupted",
			corruptFrame1: func(f1 []byte) {
				f1[0] ^= 0xFF
			},
			expectedErr: format.ErrInvalidFrameMagic,
		},
		{
			name: "Magic_Byte3_Corrupted",
			corruptFrame1: func(f1 []byte) {
				f1[3] ^= 0xFF
			},
			expectedErr: format.ErrInvalidFrameMagic,
		},
		{
			name: "HeaderCRC_Byte4_Corrupted",
			corruptFrame1: func(f1 []byte) {
				f1[4] ^= 0x01
			},
			expectedErr: format.ErrCorruptFrameHeader,
		},
		{
			name: "HeaderCRC_Byte7_Corrupted",
			corruptFrame1: func(f1 []byte) {
				f1[7] ^= 0x80
			},
			expectedErr: format.ErrCorruptFrameHeader,
		},
		{
			name: "PayloadLen_BitFlip",
			corruptFrame1: func(f1 []byte) {
				f1[8] ^= 0x01 // alters payload length, invalidating header CRC
			},
			expectedErr: format.ErrCorruptFrameHeader,
		},
		{
			name: "SeqNo_BitFlip",
			corruptFrame1: func(f1 []byte) {
				f1[16] ^= 0x01 // alters sequence number, invalidating header CRC
			},
			expectedErr: format.ErrCorruptFrameHeader,
		},
		{
			name: "OpCount_BitFlip",
			corruptFrame1: func(f1 []byte) {
				f1[24] ^= 0x01 // alters op count, invalidating header CRC
			},
			expectedErr: format.ErrCorruptFrameHeader,
		},
		{
			name: "Payload_ValueByte_Flipped",
			corruptFrame1: func(f1 []byte) {
				f1[format.FrameHeaderSize+8] ^= 0x01
			},
			expectedErr: format.ErrCorruptPayload,
		},
		{
			name: "TrailerCRC_Byte0_Flipped",
			corruptFrame1: func(f1 []byte) {
				f1[len(f1)-4] ^= 0x01
			},
			expectedErr: format.ErrCorruptPayload,
		},
		{
			name: "TrailerCRC_Byte3_Flipped",
			corruptFrame1: func(f1 []byte) {
				f1[len(f1)-1] ^= 0x80
			},
			expectedErr: format.ErrCorruptPayload,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "middle_corrupt.db")

			f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
			if err != nil {
				t.Fatalf("OpenFile failed: %v", err)
			}
			defer f.Close()

			writeValidHeader(t, f)

			// Frame 1 (target of corruption)
			ops1 := []format.Operation{{Type: format.OpPut, Key: []byte("k1"), Value: []byte("val_middle_corruption_test")}}
			f1, _ := format.EncodeFrame(1, ops1)

			// Frame 2 (valid frame ahead, proving Frame 1 is in the middle)
			ops2 := []format.Operation{{Type: format.OpPut, Key: []byte("k2_ahead"), Value: []byte("val2")}}
			f2, _ := format.EncodeFrame(2, ops2)

			// Apply specific corruption to Frame 1
			tc.corruptFrame1(f1)

			_, _ = f.WriteAt(f1, 32)
			_, _ = f.WriteAt(f2, int64(32+len(f1)))
			totalSize := uint64(32 + len(f1) + len(f2))
			_ = f.Sync()

			beforeHash := fileSHA256(t, f)

			// Recovery MUST fail closed!
			_, err = RecoverLog(f, totalSize)
			if err == nil {
				t.Fatalf("RecoverLog should have failed closed on middle corruption")
			}
			if !errors.Is(err, tc.expectedErr) {
				t.Errorf("Expected error wrapping %v, got %v", tc.expectedErr, err)
			}

			// INVARIANT: File size on disk MUST NOT BE TRUNCATED
			fi, _ := f.Stat()
			if uint64(fi.Size()) != totalSize {
				t.Fatalf("VIOLATION: File was truncated from %d to %d bytes on middle corruption!", totalSize, fi.Size())
			}

			// INVARIANT: File contents MUST NOT BE MODIFIED
			afterHash := fileSHA256(t, f)
			if beforeHash != afterHash {
				t.Fatalf("VIOLATION: File byte contents were mutated during failed recovery!")
			}
		})
	}
}

// ----------------------------------------------------------------------------
// 3. Middle-of-File Corruption Followed by Non-Frame Garbage (No Valid Frame Ahead)
// ----------------------------------------------------------------------------

func TestChallengerM3_2_MiddleCorruption_CorruptPayload_FollowedByGarbage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "payload_corrupt_with_garbage.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)

	// Frame 1 with corrupt trailer CRC
	ops1 := []format.Operation{{Type: format.OpPut, Key: []byte("k1"), Value: []byte("v1")}}
	f1, _ := format.EncodeFrame(1, ops1)
	f1[len(f1)-1] ^= 0x01
	_, _ = f.WriteAt(f1, 32)

	// Append 15 non-frame garbage bytes (e.g. 0xEE)
	garbage := bytes.Repeat([]byte{0xEE}, 15)
	_, _ = f.WriteAt(garbage, int64(32+len(f1)))
	totalSize := uint64(32 + len(f1) + len(garbage))
	_ = f.Sync()

	beforeHash := fileSHA256(t, f)

	// Recovery must fail closed because offset + totalFrameSize < fileSize
	_, err = RecoverLog(f, totalSize)
	if err == nil {
		t.Fatalf("Expected RecoverLog to fail closed on corrupt payload with trailing garbage")
	}
	if !errors.Is(err, format.ErrCorruptPayload) {
		t.Errorf("Expected ErrCorruptPayload, got %v", err)
	}

	// File MUST NOT be truncated or mutated
	fi, _ := f.Stat()
	if uint64(fi.Size()) != totalSize {
		t.Fatalf("File size mutated: expected %d, got %d", totalSize, fi.Size())
	}
	afterHash := fileSHA256(t, f)
	if beforeHash != afterHash {
		t.Fatalf("File bytes modified on error return!")
	}
}

// ----------------------------------------------------------------------------
// 4. Recovery Idempotence Matrix
// ----------------------------------------------------------------------------

func TestChallengerM3_2_Recovery_Idempotence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "idempotence_matrix.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)

	// Append 20 valid frames with puts, overwrites, and deletes
	var validSize uint64 = 32
	for i := uint64(1); i <= 20; i++ {
		var ops []format.Operation
		if i%3 == 0 {
			ops = []format.Operation{{Type: format.OpDel, Key: []byte(fmt.Sprintf("key_%d", i-1))}}
		} else {
			ops = []format.Operation{
				{Type: format.OpPut, Key: []byte(fmt.Sprintf("key_%d", i)), Value: []byte(fmt.Sprintf("val_%d", i))},
			}
		}
		frameBytes, err := format.EncodeFrame(i, ops)
		if err != nil {
			t.Fatalf("EncodeFrame %d failed: %v", i, err)
		}
		_, _ = f.WriteAt(frameBytes, int64(validSize))
		validSize += uint64(len(frameBytes))
	}

	// Append torn write at tail (13 bytes of partial next frame)
	tornPartial := []byte{0x45, 0x42, 0x41, 0x55, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99}
	_, _ = f.WriteAt(tornPartial, int64(validSize))
	totalSize := validSize + uint64(len(tornPartial))
	_ = f.Sync()

	// Initial Recovery (Run 1): repairs torn tail
	res1, err := RecoverLog(f, totalSize)
	if err != nil {
		t.Fatalf("Run 1 RecoverLog failed: %v", err)
	}
	if !res1.TornTailFixed {
		t.Errorf("Run 1 TornTailFixed should be true")
	}
	if res1.ValidEOF != validSize {
		t.Errorf("Run 1 ValidEOF mismatch: expected %d, got %d", validSize, res1.ValidEOF)
	}

	fi, _ := f.Stat()
	if uint64(fi.Size()) != validSize {
		t.Errorf("Physical size not truncated to %d (got %d)", validSize, fi.Size())
	}

	// Subsequent Repeated Recoveries (Runs 2 through 10) must be 100% idempotent
	for run := 2; run <= 10; run++ {
		fiRun, _ := f.Stat()
		resRun, err := RecoverLog(f, uint64(fiRun.Size()))
		if err != nil {
			t.Fatalf("Run %d RecoverLog failed: %v", run, err)
		}

		if resRun.TornTailFixed {
			t.Errorf("Run %d TornTailFixed must be false on clean file", run)
		}
		if resRun.ValidEOF != res1.ValidEOF {
			t.Errorf("Run %d ValidEOF mismatch: expected %d, got %d", run, res1.ValidEOF, resRun.ValidEOF)
		}
		if resRun.NextSeqNo != res1.NextSeqNo {
			t.Errorf("Run %d NextSeqNo mismatch: expected %d, got %d", run, res1.NextSeqNo, resRun.NextSeqNo)
		}
		if len(resRun.KeyDir) != len(res1.KeyDir) {
			t.Errorf("Run %d KeyDir count mismatch: expected %d, got %d", run, len(res1.KeyDir), len(resRun.KeyDir))
		}
		for k, v1 := range res1.KeyDir {
			vRun, ok := resRun.KeyDir[k]
			if !ok {
				t.Errorf("Run %d key %s missing", run, k)
			}
			if vRun != v1 {
				t.Errorf("Run %d entry for key %s mismatched: %+v vs %+v", run, k, vRun, v1)
			}
		}
	}
}

// ----------------------------------------------------------------------------
// 5. Recovery Performance & Speed Benchmark
// ----------------------------------------------------------------------------

func TestChallengerM3_2_Recovery_Speed_Benchmark(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "speed_bench.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)

	// Write 10,000 operations organized in 100 frames (100 ops/frame)
	const (
		totalOps     = 10000
		opsPerFrame  = 100
		totalFrames  = totalOps / opsPerFrame
	)

	var validSize uint64 = 32
	for frameIdx := uint64(1); frameIdx <= totalFrames; frameIdx++ {
		ops := make([]format.Operation, opsPerFrame)
		for j := 0; j < opsPerFrame; j++ {
			k := fmt.Sprintf("bench_key_%05d_%03d", frameIdx, j)
			v := fmt.Sprintf("bench_val_%05d_%03d", frameIdx, j)
			ops[j] = format.Operation{Type: format.OpPut, Key: []byte(k), Value: []byte(v)}
		}
		frameBytes, err := format.EncodeFrame(frameIdx, ops)
		if err != nil {
			t.Fatalf("EncodeFrame %d failed: %v", frameIdx, err)
		}
		_, _ = f.WriteAt(frameBytes, int64(validSize))
		validSize += uint64(len(frameBytes))
	}
	_ = f.Sync()

	// Benchmark recovery execution time
	startTime := time.Now()
	res, err := RecoverLog(f, validSize)
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("RecoverLog failed: %v", err)
	}
	if res.RecoveredOps != totalOps {
		t.Errorf("RecoveredOps mismatch: expected %d, got %d", totalOps, res.RecoveredOps)
	}
	if len(res.KeyDir) != totalOps {
		t.Errorf("KeyDir count mismatch: expected %d, got %d", totalOps, len(res.KeyDir))
	}

	t.Logf("Empirical Recovery Benchmark: %d ops (%d frames, %d bytes) recovered in %v",
		totalOps, totalFrames, validSize, elapsed)

	// Invariant: 10,000 keys must recover in << 1.0s (Milestone requirement is <= 2.0s for 1M keys)
	if elapsed > 1*time.Second {
		t.Errorf("Recovery took too long: %v > 1.0s", elapsed)
	}
}

// ----------------------------------------------------------------------------
// 6. Forward Scanner Cross-Chunk Buffer Boundary Stress Test
// ----------------------------------------------------------------------------

func TestChallengerM3_2_ForwardScanner_ChunkBoundary_Stress(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cross_chunk_boundary.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer f.Close()

	writeValidHeader(t, f)

	// Create garbage padding spanning exactly 64 KiB - 2 bytes so candidate header straddles chunk boundary
	offset1 := uint64(32)
	padLen := scanChunkSize - 2
	padBytes := bytes.Repeat([]byte{0x55}, padLen) // repetitive non-magic bytes
	_, _ = f.WriteAt(padBytes, int64(offset1))

	// Write candidate valid frame header straddling across the 64 KiB boundary
	frameOffset := offset1 + uint64(padLen)
	ops := []format.Operation{{Type: format.OpPut, Key: []byte("straddle_k"), Value: []byte("straddle_v")}}
	frameBytes, _ := format.EncodeFrame(1, ops)
	_, _ = f.WriteAt(frameBytes, int64(frameOffset))

	totalSize := frameOffset + uint64(len(frameBytes))
	_ = f.Sync()

	hasAhead, err := hasValidFrameAhead(f, offset1, totalSize)
	if err != nil {
		t.Fatalf("hasValidFrameAhead error: %v", err)
	}
	if !hasAhead {
		t.Fatalf("hasValidFrameAhead failed to detect valid frame straddling 64 KiB scanChunkSize boundary!")
	}
}
