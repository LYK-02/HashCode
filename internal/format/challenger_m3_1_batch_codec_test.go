package format_test

import (
	"bytes"
	"errors"
	"testing"

	"kvdb/internal/format"
)

// ----------------------------------------------------------------------------
// Challenger M3-1: Format & Codec Boundary & Corruption Empirical Harness
// ----------------------------------------------------------------------------

func TestChallengerM3_1_Format_BatchBoundaryLimits(t *testing.T) {
	// 1. 0-op batch
	_, err := format.EncodeFrame(1, []format.Operation{})
	if !errors.Is(err, format.ErrBatchEmpty) {
		t.Fatalf("Expected ErrBatchEmpty, got: %v", err)
	}

	// 2. 10,001-op batch
	ops10001 := make([]format.Operation, 10001)
	for i := range ops10001 {
		ops10001[i] = format.Operation{Type: format.OpPut, Key: []byte("k"), Value: []byte("v")}
	}
	_, err = format.EncodeFrame(1, ops10001)
	if !errors.Is(err, format.ErrBatchTooLarge) {
		t.Fatalf("Expected ErrBatchTooLarge, got: %v", err)
	}

	// 3. 10,000-op batch (max boundary accepted)
	ops10000 := make([]format.Operation, 10000)
	for i := range ops10000 {
		ops10000[i] = format.Operation{Type: format.OpPut, Key: []byte("k"), Value: []byte("v")}
	}
	buf, err := format.EncodeFrame(1, ops10000)
	if err != nil {
		t.Fatalf("10,000-op batch EncodeFrame failed: %v", err)
	}
	fh, err := format.DecodeFrameHeader(buf[:format.FrameHeaderSize])
	if err != nil || fh.OpCount != 10000 {
		t.Fatalf("DecodeFrameHeader mismatch: %v, count=%d", err, fh.OpCount)
	}
}

func TestChallengerM3_1_Format_OperationValidation(t *testing.T) {
	cases := []struct {
		name string
		op   format.Operation
		err  error
	}{
		{
			name: "KeyEmpty",
			op:   format.Operation{Type: format.OpPut, Key: []byte{}, Value: []byte("v")},
			err:  format.ErrKeyEmpty,
		},
		{
			name: "KeyTooLarge_1025",
			op:   format.Operation{Type: format.OpPut, Key: bytes.Repeat([]byte("K"), 1025), Value: []byte("v")},
			err:  format.ErrKeyTooLarge,
		},
		{
			name: "ValTooLarge_1MB_Plus_1",
			op:   format.Operation{Type: format.OpPut, Key: []byte("k"), Value: bytes.Repeat([]byte("V"), 1048577)},
			err:  format.ErrValueTooLarge,
		},
		{
			name: "DelWithNonZeroValue",
			op:   format.Operation{Type: format.OpDel, Key: []byte("k"), Value: []byte("non_zero")},
			err:  format.ErrDelNonZeroValue,
		},
		{
			name: "UnknownOpType",
			op:   format.Operation{Type: format.OpType(0x99), Key: []byte("k"), Value: nil},
			err:  format.ErrUnknownOpType,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := format.ValidateOperation(tc.op)
			if !errors.Is(err, tc.err) {
				t.Fatalf("ValidateOperation got %v, want %v", err, tc.err)
			}
			_, err = format.EncodeFrame(1, []format.Operation{tc.op})
			if !errors.Is(err, tc.err) {
				t.Fatalf("EncodeFrame got %v, want %v", err, tc.err)
			}
		})
	}
}

func TestChallengerM3_1_Format_TornFrameSimulations_B1_B6(t *testing.T) {
	// Construct valid 5-op batch
	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("key_1"), Value: []byte("val_1")},
		{Type: format.OpPut, Key: []byte("key_2"), Value: []byte("val_2")},
		{Type: format.OpDel, Key: []byte("key_1")},
		{Type: format.OpPut, Key: []byte("key_3"), Value: []byte("val_3")},
		{Type: format.OpPut, Key: []byte("key_4"), Value: []byte("val_4")},
	}
	frame, err := format.EncodeFrame(100, ops)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	// 1. Partial header (< 28 bytes)
	for hLen := 1; hLen < format.FrameHeaderSize; hLen++ {
		_, err := format.DecodeFrameHeader(frame[:hLen])
		if !errors.Is(err, format.ErrFrameHeaderTooShort) {
			t.Fatalf("Offset %d: expected ErrFrameHeaderTooShort, got %v", hLen, err)
		}
	}

	// 2. Corrupt header CRC
	corruptHdr := make([]byte, format.FrameHeaderSize)
	copy(corruptHdr, frame[:format.FrameHeaderSize])
	corruptHdr[5] ^= 0x55
	_, err = format.DecodeFrameHeader(corruptHdr)
	if !errors.Is(err, format.ErrCorruptFrameHeader) {
		t.Fatalf("Expected ErrCorruptFrameHeader, got %v", err)
	}

	// 3. Partial payload truncation (Group B1 simulation)
	fh, err := format.DecodeFrameHeader(frame[:format.FrameHeaderSize])
	if err != nil {
		t.Fatalf("DecodeFrameHeader failed: %v", err)
	}
	payload := frame[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
	trailer := frame[format.FrameHeaderSize+fh.PayloadLen:]

	// Truncated payload with full trailer
	_, err = format.DecodePayloadRecords(payload[:len(payload)/2], trailer, fh.OpCount)
	if !errors.Is(err, format.ErrCorruptPayload) {
		t.Fatalf("Expected ErrCorruptPayload for truncated payload, got %v", err)
	}

	// 4. Missing trailer (Group B2 simulation)
	_, err = format.DecodePayloadRecords(payload, []byte{}, fh.OpCount)
	if !errors.Is(err, format.ErrTrailerTooShort) {
		t.Fatalf("Expected ErrTrailerTooShort, got %v", err)
	}

	// 5. Torn trailer (Group B3 simulation)
	for tLen := 1; tLen < format.FrameTrailerSize; tLen++ {
		_, err = format.DecodePayloadRecords(payload, trailer[:tLen], fh.OpCount)
		if !errors.Is(err, format.ErrTrailerTooShort) {
			t.Fatalf("Trailer len %d: expected ErrTrailerTooShort, got %v", tLen, err)
		}
	}

	// 6. Corrupt trailer CRC bit flip (Group B3 simulation)
	corruptTrailer := make([]byte, 4)
	copy(corruptTrailer, trailer)
	corruptTrailer[2] ^= 0xFF
	_, err = format.DecodePayloadRecords(payload, corruptTrailer, fh.OpCount)
	if !errors.Is(err, format.ErrCorruptPayload) {
		t.Fatalf("Expected ErrCorruptPayload for flipped trailer CRC, got %v", err)
	}

	// 7. Full valid decode
	records, err := format.DecodePayloadRecords(payload, trailer, fh.OpCount)
	if err != nil {
		t.Fatalf("Valid DecodePayloadRecords failed: %v", err)
	}
	if len(records) != 5 {
		t.Fatalf("Expected 5 decoded records, got %d", len(records))
	}
}

func TestChallengerM3_1_Format_IntraBatchLocators(t *testing.T) {
	// Verify ValueFileOffset formula matches exact physical layout
	frameStart := uint64(1024)
	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("alpha"), Value: []byte("val_alpha")},
		{Type: format.OpPut, Key: []byte("beta"), Value: []byte("val_beta")},
		{Type: format.OpDel, Key: []byte("alpha")},
		{Type: format.OpPut, Key: []byte("alpha"), Value: []byte("final_alpha")},
	}
	frame, err := format.EncodeFrame(1, ops)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	fh, _ := format.DecodeFrameHeader(frame[:format.FrameHeaderSize])
	records, err := format.DecodePayloadRecords(
		frame[format.FrameHeaderSize:format.FrameHeaderSize+fh.PayloadLen],
		frame[format.FrameHeaderSize+fh.PayloadLen:],
		fh.OpCount,
	)
	if err != nil {
		t.Fatalf("DecodePayloadRecords failed: %v", err)
	}

	for i, r := range records {
		expectedOffset := frameStart + format.FrameHeaderSize + uint64(r.PayloadOffset) + format.RecordHeaderSize + uint64(r.KeyLen)
		computedOffset := format.ValueFileOffset(frameStart, r.PayloadOffset, r.KeyLen)
		if computedOffset != expectedOffset {
			t.Fatalf("Record %d: offset mismatch %d != %d", i, computedOffset, expectedOffset)
		}

		if r.Type == format.OpPut {
			// Verify that slice points directly to value bytes in frame
			relativeValOffset := int(computedOffset - frameStart)
			valSlice := frame[relativeValOffset : relativeValOffset+int(r.ValueLen)]
			if !bytes.Equal(valSlice, r.Value) {
				t.Fatalf("Record %d: value bytes mismatch: %s != %s", i, valSlice, r.Value)
			}
		}
	}
}


func TestChallengerM3_1_Format_IntraBatch_DifferentialOracle(t *testing.T) {
	// Differential oracle testing for intra-batch duplicate keys and ordering
	oracle := make(map[string][]byte)
	var ops []format.Operation

	put := func(k, v string) {
		ops = append(ops, format.Operation{Type: format.OpPut, Key: []byte(k), Value: []byte(v)})
		oracle[k] = []byte(v)
	}
	del := func(k string) {
		ops = append(ops, format.Operation{Type: format.OpDel, Key: []byte(k)})
		delete(oracle, k)
	}

	// 1. Definite sequence: PUT -> PUT -> DEL -> PUT (last wins)
	put("k_A", "val_1")
	put("k_A", "val_2")
	del("k_A")
	put("k_A", "val_3")

	// 2. Definite sequence: PUT -> DEL -> PUT -> DEL (absent)
	put("k_B", "val_1")
	del("k_B")
	put("k_B", "val_2")
	del("k_B")

	// 3. Delete non-existent
	del("k_nonexistent")

	// 4. Randomized updates across 20 distinct keys (500 ops)
	keys := []string{"k1", "k2", "k3", "k4", "k5", "k6", "k7", "k8", "k9", "k10",
		"k11", "k12", "k13", "k14", "k15", "k16", "k17", "k18", "k19", "k20"}
	for i := 0; i < 500; i++ {
		k := keys[i%len(keys)]
		if i%3 == 0 {
			del(k)
		} else {
			put(k, "v_"+k+"_"+string(rune(i)))
		}
	}

	frame, err := format.EncodeFrame(42, ops)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	fh, err := format.DecodeFrameHeader(frame[:format.FrameHeaderSize])
	if err != nil {
		t.Fatalf("DecodeFrameHeader failed: %v", err)
	}
	if fh.OpCount != uint32(len(ops)) {
		t.Fatalf("OpCount mismatch: %d != %d", fh.OpCount, len(ops))
	}

	payload := frame[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
	trailer := frame[format.FrameHeaderSize+fh.PayloadLen:]
	records, err := format.DecodePayloadRecords(payload, trailer, fh.OpCount)
	if err != nil {
		t.Fatalf("DecodePayloadRecords failed: %v", err)
	}

	// Replay records against state
	reconstructed := make(map[string][]byte)
	for _, r := range records {
		if r.Type == format.OpPut {
			reconstructed[string(r.Key)] = r.Value
		} else if r.Type == format.OpDel {
			delete(reconstructed, string(r.Key))
		}
	}

	if len(reconstructed) != len(oracle) {
		t.Fatalf("State size mismatch: got %d, want %d", len(reconstructed), len(oracle))
	}
	for k, wantVal := range oracle {
		gotVal, exists := reconstructed[k]
		if !exists {
			t.Fatalf("Key %s missing in reconstructed state", k)
		}
		if !bytes.Equal(gotVal, wantVal) {
			t.Fatalf("Value mismatch for %s: got %s, want %s", k, gotVal, wantVal)
		}
	}
}

func TestChallengerM3_1_Format_ByteByByte_Sweep(t *testing.T) {
	ops := []format.Operation{
		{Type: format.OpPut, Key: []byte("sw_1"), Value: []byte("val_1")},
		{Type: format.OpPut, Key: []byte("sw_2"), Value: []byte("val_2")},
		{Type: format.OpDel, Key: []byte("sw_1")},
	}
	frame, err := format.EncodeFrame(1, ops)
	if err != nil {
		t.Fatalf("EncodeFrame failed: %v", err)
	}

	// At every truncated cut, decoding must fail with typed error, never panic
	for cut := 1; cut < len(frame); cut++ {
		truncated := frame[:cut]
		if cut < format.FrameHeaderSize {
			_, err := format.DecodeFrameHeader(truncated)
			if !errors.Is(err, format.ErrFrameHeaderTooShort) {
				t.Fatalf("Cut %d: expected ErrFrameHeaderTooShort, got %v", cut, err)
			}
		} else {
			fh, err := format.DecodeFrameHeader(truncated[:format.FrameHeaderSize])
			if err != nil {
				t.Fatalf("Cut %d: valid header failed: %v", cut, err)
			}
			rem := truncated[format.FrameHeaderSize:]
			if uint64(len(rem)) < uint64(fh.PayloadLen)+uint64(format.FrameTrailerSize) {
				// Either truncated payload or truncated trailer
				var payload, trailer []byte
				if len(rem) <= int(fh.PayloadLen) {
					payload = rem
					trailer = nil
				} else {
					payload = rem[:fh.PayloadLen]
					trailer = rem[fh.PayloadLen:]
				}
				_, decErr := format.DecodePayloadRecords(payload, trailer, fh.OpCount)
				if decErr == nil {
					t.Fatalf("Cut %d: expected decode error on truncated payload/trailer, got nil", cut)
				}
			}
		}
	}
}

func TestChallengerM3_1_Format_10000Ops_ScaleStress(t *testing.T) {
	ops := make([]format.Operation, 10000)
	for i := range ops {
		ops[i] = format.Operation{
			Type:  format.OpPut,
			Key:   []byte("k"),
			Value: []byte("v"),
		}
	}

	frame, err := format.EncodeFrame(999, ops)
	if err != nil {
		t.Fatalf("Encode 10,000 ops failed: %v", err)
	}

	fh, err := format.DecodeFrameHeader(frame[:format.FrameHeaderSize])
	if err != nil {
		t.Fatalf("DecodeFrameHeader failed: %v", err)
	}
	if fh.OpCount != 10000 {
		t.Fatalf("OpCount mismatch: %d != 10000", fh.OpCount)
	}

	payload := frame[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
	trailer := frame[format.FrameHeaderSize+fh.PayloadLen:]
	records, err := format.DecodePayloadRecords(payload, trailer, fh.OpCount)
	if err != nil {
		t.Fatalf("DecodePayloadRecords failed: %v", err)
	}
	if len(records) != 10000 {
		t.Fatalf("Records length mismatch: %d != 10000", len(records))
	}
}
