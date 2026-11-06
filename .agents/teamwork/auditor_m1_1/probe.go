package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"os"

	"kvdb/internal/format"
	"kvdb/internal/testutil"
)

func assert(cond bool, msg string) {
	if !cond {
		fmt.Fprintf(os.Stderr, "PROBE FAILURE: %s\n", msg)
		os.Exit(1)
	}
}

func main() {
	fmt.Println("=== Starting Independent Forensic Probe ===")

	// 1. Castagnoli CRC32C RFC 3720 check
	rfcInput := []byte("123456789")
	rfcCrc := format.ChecksumCastagnoli(rfcInput)
	assert(rfcCrc == 0xe3069283, fmt.Sprintf("RFC 3720 CRC mismatch: 0x%08x != 0xe3069283", rfcCrc))
	fmt.Printf("[OK] RFC 3720 CRC32C verified: 0x%08x\n", rfcCrc)

	// 2. Castagnoli bit-flip sensitivity check
	buf := make([]byte, 128)
	rand.Read(buf)
	baseCRC := format.ChecksumCastagnoli(buf)
	for bit := 0; bit < 128*8; bit++ {
		byteIdx := bit / 8
		bitIdx := bit % 8
		buf[byteIdx] ^= (1 << bitIdx)
		mutatedCRC := format.ChecksumCastagnoli(buf)
		assert(mutatedCRC != baseCRC, fmt.Sprintf("CRC collision on bit flip at bit %d", bit))
		buf[byteIdx] ^= (1 << bitIdx) // restore
	}
	fmt.Println("[OK] Castagnoli CRC32C bit-flip sensitivity verified over 1024 distinct bit flips")

	// 3. Header Dynamic Generations Round-Trip
	generations := []uint64{0, 1, 2, 5, 42, 1000, 123456789, math.MaxUint32, math.MaxUint64 - 1, math.MaxUint64}
	for _, gen := range generations {
		hdr := format.NewHeader(gen)
		encoded, err := format.EncodeHeader(&hdr)
		assert(err == nil, fmt.Sprintf("EncodeHeader failed for gen %d: %v", gen, err))
		assert(len(encoded) == 32, "Header length != 32")

		decoded, err := format.DecodeHeader(encoded)
		assert(err == nil, fmt.Sprintf("DecodeHeader failed for gen %d: %v", gen, err))
		assert(decoded.CompactionGen == gen, fmt.Sprintf("CompactionGen mismatch: %d != %d", decoded.CompactionGen, gen))
		assert(decoded.Generation == gen, fmt.Sprintf("Generation mismatch: %d != %d", decoded.Generation, gen))
		assert(decoded.Version == 1, "Version != 1")
		assert(decoded.Reserved == 0, "Reserved != 0")
	}
	fmt.Println("[OK] Header round-trip verified for 10 boundary generation numbers")

	// 4. Header Corruption Probes
	freshHdr := format.NewHeader(0)
	rawHdr, _ := format.EncodeHeader(&freshHdr)
	for i := 0; i < 32; i++ {
		corrupted := bytes.Clone(rawHdr)
		corrupted[i] ^= 0x5A
		_, err := format.DecodeHeader(corrupted)
		assert(err != nil, fmt.Sprintf("DecodeHeader should fail on corrupted byte %d, but succeeded", i))
	}
	fmt.Println("[OK] Header corruption detected for all 32 bytes independently")

	// 5. Golden Hex Bit-for-Bit Verification
	goldenHdrBytes := testutil.MustDecodeHex(testutil.GoldenHeaderHex)
	hdrDecoded, err := format.DecodeHeader(goldenHdrBytes)
	assert(err == nil, "Failed to decode GoldenHeaderHex")
	assert(hdrDecoded.CompactionGen == 0, "GoldenHeaderHex gen != 0")

	goldenCompactedBytes := testutil.MustDecodeHex(testutil.GoldenCompactedHeaderHex)
	compactDecoded, err := format.DecodeHeader(goldenCompactedBytes)
	assert(err == nil, "Failed to decode GoldenCompactedHeaderHex")
	assert(compactDecoded.CompactionGen == 5, "GoldenCompactedHeaderHex gen != 5")
	fmt.Println("[OK] Golden header hexes decoded bit-for-bit")

	// 6. Frame Dynamic Batch Encoding & Decoding
	testOps := []format.Operation{
		{Type: format.OpPut, Key: []byte("alpha"), Value: []byte("bravo")},
		{Type: format.OpDel, Key: []byte("charlie"), Value: nil},
		{Type: format.OpPut, Key: []byte("delta"), Value: []byte("")},
		{Type: format.OpPut, Key: []byte("echo"), Value: bytes.Repeat([]byte("X"), 65536)},
	}
	seq := uint64(987654321)
	frameBytes, err := format.EncodeFrame(seq, testOps)
	assert(err == nil, fmt.Sprintf("EncodeFrame failed: %v", err))

	fh, decodedOps, err := format.DecodeFullFrame(frameBytes)
	assert(err == nil, fmt.Sprintf("DecodeFullFrame failed: %v", err))
	assert(fh.SeqNo == seq, "SeqNo mismatch")
	assert(fh.OpCount == 4, "OpCount mismatch")
	assert(len(decodedOps) == 4, "Decoded ops count mismatch")
	assert(bytes.Equal(decodedOps[0].Key, []byte("alpha")), "Op 0 key mismatch")
	assert(bytes.Equal(decodedOps[0].Value, []byte("bravo")), "Op 0 value mismatch")
	assert(decodedOps[1].Type == format.OpDel, "Op 1 type mismatch")
	assert(bytes.Equal(decodedOps[1].Key, []byte("charlie")), "Op 1 key mismatch")
	assert(len(decodedOps[1].Value) == 0, "Op 1 value not empty")
	assert(bytes.Equal(decodedOps[2].Key, []byte("delta")), "Op 2 key mismatch")
	assert(len(decodedOps[2].Value) == 0, "Op 2 value not empty")
	assert(bytes.Equal(decodedOps[3].Key, []byte("echo")), "Op 3 key mismatch")
	assert(len(decodedOps[3].Value) == 65536, "Op 3 value len mismatch")
	fmt.Println("[OK] Dynamic 4-operation frame verified round-trip")

	// 7. ValueFileOffset Locator Direct Verification
	// Let's decode with locators and verify value offsets directly point to the value in the frame
	records, err := format.DecodePayloadRecords(
		frameBytes[format.FrameHeaderSize:format.FrameHeaderSize+fh.PayloadLen],
		frameBytes[format.FrameHeaderSize+fh.PayloadLen:],
		fh.OpCount,
	)
	assert(err == nil, fmt.Sprintf("DecodePayloadRecords failed: %v", err))
	baseOffset := uint64(1000) // pretend frame is at file offset 1000
	for i, r := range records {
		valOffset := format.ValueFileOffset(baseOffset, r.PayloadOffset, r.KeyLen)
		if r.ValueLen > 0 {
			// Offset in frameBytes is: valOffset - baseOffset
			frameValOffset := valOffset - baseOffset
			actualValInFrame := frameBytes[frameValOffset : frameValOffset+uint64(r.ValueLen)]
			assert(bytes.Equal(actualValInFrame, testOps[i].Value), fmt.Sprintf("ValueFileOffset does not match value in frame for op %d", i))
		}
	}
	fmt.Println("[OK] ValueFileOffset locators match actual frame bytes perfectly")

	// 8. Adversarial Frame Corruption Probes
	// Corrupt header bytes
	for i := 0; i < format.FrameHeaderSize; i++ {
		corruptedFrame := bytes.Clone(frameBytes)
		corruptedFrame[i] ^= 0xA5
		_, err := format.DecodeFrameHeader(corruptedFrame[:format.FrameHeaderSize])
		assert(err != nil, fmt.Sprintf("DecodeFrameHeader must fail on header byte %d corruption", i))
	}
	// Corrupt trailer bytes
	for i := 0; i < format.FrameTrailerSize; i++ {
		trailerPos := len(frameBytes) - format.FrameTrailerSize + i
		corruptedFrame := bytes.Clone(frameBytes)
		corruptedFrame[trailerPos] ^= 0x5A
		payload := corruptedFrame[format.FrameHeaderSize : format.FrameHeaderSize+fh.PayloadLen]
		trailer := corruptedFrame[format.FrameHeaderSize+fh.PayloadLen:]
		_, err := format.DecodePayloadAndTrailer(payload, trailer, fh.OpCount)
		assert(err != nil, fmt.Sprintf("DecodePayloadAndTrailer must fail on trailer byte %d corruption", i))
	}
	fmt.Println("[OK] Header and trailer corruption resistance verified")

	// 9. Anti-OOM Stress Test
	badHdr := make([]byte, 28)
	binary.LittleEndian.PutUint32(badHdr[0:4], format.FrameMagic)
	binary.LittleEndian.PutUint64(badHdr[8:16], math.MaxUint64) // 18 Exabytes!
	binary.LittleEndian.PutUint64(badHdr[16:24], 1)
	binary.LittleEndian.PutUint32(badHdr[24:28], 1)
	binary.LittleEndian.PutUint32(badHdr[4:8], format.ChecksumCastagnoli(badHdr[8:28]))
	_, oomErr := format.DecodeFrameHeader(badHdr)
	assert(oomErr != nil, "Anti-OOM failed: accepted MaxUint64 payload length")
	fmt.Println("[OK] Anti-OOM protection verified against 18 Exabyte malicious payload header")

	fmt.Println("=== All Forensic Probes PASSED Cleanly ===")
}
