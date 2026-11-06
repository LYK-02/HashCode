package engine

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"kvdb/internal/format"
)

const (
	// scanChunkSize defines the buffer size used for chunked forward corruption scanning.
	scanChunkSize = 64 * 1024 // 64 KiB
)

// frameMagicBytes defines the little-endian representation of FrameMagic (0x55414245).
var frameMagicBytes = []byte{0x45, 0x42, 0x41, 0x55}

// RecoveryResult encapsulates the state reconstructed by replaying the append-only log.
type RecoveryResult struct {
	KeyDir        map[string]KeyDirEntry
	ValidEOF      uint64
	NextSeqNo     uint64
	RecoveredOps  uint64
	TornTailFixed bool
}

// RecoverLog sequentially scans frames from byte 32 to fileSize, populating KeyDir,
// automatically truncating torn tails at EOF, and failing closed on middle corruption.
func RecoverLog(dataFile *os.File, fileSize uint64) (*RecoveryResult, error) {
	if fileSize < uint64(format.HeaderSize) {
		return nil, format.ErrHeaderTooShort
	}

	// Estimate initial capacity to minimize map rehashing during recovery.
	var initialCap int
	if fileSize > uint64(format.HeaderSize) {
		// Minimum record size is 8 bytes. For typical key-values (16-64 bytes),
		// dividing by 16 gives an accurate capacity estimate with headroom to avoid map rehashing.
		estimatedKeys := int((fileSize - uint64(format.HeaderSize)) / 16)
		if estimatedKeys > 1250000 {
			estimatedKeys = 1250000
		}
		if estimatedKeys > 0 {
			initialCap = estimatedKeys
		}
	}
	keyDir := make(map[string]KeyDirEntry, initialCap)

	offset := uint64(format.HeaderSize)
	var nextSeqNo uint64 = 1
	var recoveredOps uint64 = 0
	tornTailFixed := false

	// Reusable buffers to minimize heap allocations during large replays
	fhdrBuf := make([]byte, format.FrameHeaderSize)
	var bodyBuf []byte

	for offset < fileSize {
		// Case T1: Incomplete trailing frame header at EOF (< 28 bytes remaining)
		if offset+format.FrameHeaderSize > fileSize {
			if err := repairTornTail(dataFile, offset, fileSize, "incomplete frame header at EOF"); err != nil {
				return nil, err
			}
			tornTailFixed = true
			break
		}

		// Read 28-byte UABE frame header
		if err := readAtFull(dataFile, fhdrBuf, int64(offset)); err != nil {
			return nil, fmt.Errorf("recovery read frame header failed at offset %d: %w", offset, err)
		}

		fh, err := format.DecodeFrameHeader(fhdrBuf)
		if err != nil {
			// Case T2: Frame header is invalid (corrupted magic, bad CRC, invalid ops/payload bounds)
			hasAhead, scanErr := hasValidFrameAhead(dataFile, offset+1, fileSize)
			if scanErr != nil {
				return nil, scanErr
			}
			if hasAhead {
				// Middle-of-file corruption: fail closed immediately!
				return nil, fmt.Errorf("%w: %w", format.ErrCorruptFrameHeader, err)
			}
			// Corrupt frame header at tail: truncate torn tail and sync
			if err := repairTornTail(dataFile, offset, fileSize, err.Error()); err != nil {
				return nil, err
			}
			tornTailFixed = true
			break
		}

		totalFrameSize := format.TotalFrameSize(fh.PayloadLen)

		// Case T3: Frame claims to extend past EOF (partial payload or missing trailer)
		if offset+totalFrameSize > fileSize {
			// Torn payload / trailer write at EOF
			if err := repairTornTail(dataFile, offset, fileSize, "frame extends past EOF"); err != nil {
				return nil, err
			}
			tornTailFixed = true
			break
		}

		// Read payload + trailer in a single contiguous read system call
		bodyLen := fh.PayloadLen + format.FrameTrailerSize
		if uint64(cap(bodyBuf)) < bodyLen {
			bodyBuf = make([]byte, bodyLen)
		} else {
			bodyBuf = bodyBuf[:bodyLen]
		}
		if err := readAtFull(dataFile, bodyBuf, int64(offset+format.FrameHeaderSize)); err != nil {
			return nil, fmt.Errorf("recovery read frame body failed at offset %d: %w", offset, err)
		}
		payloadBuf := bodyBuf[:fh.PayloadLen]
		trailerBuf := bodyBuf[fh.PayloadLen:]

		// Decode and apply payload records directly to KeyDir in-place
		opsApplied, err := applyPayloadToKeyDir(payloadBuf, trailerBuf, fh.OpCount, offset, fh.SeqNo, keyDir)
		if err != nil {
			// Case T4: Corrupt payload or bad trailer CRC
			if offset+totalFrameSize < fileSize {
				return nil, err
			}
			// At exact EOF (offset + totalFrameSize == fileSize): truncate torn tail and sync
			if err := repairTornTail(dataFile, offset, fileSize, err.Error()); err != nil {
				return nil, err
			}
			tornTailFixed = true
			break
		}

		offset += totalFrameSize
		if fh.SeqNo >= nextSeqNo {
			nextSeqNo = fh.SeqNo + 1
		}
		recoveredOps += opsApplied
	}

	return &RecoveryResult{
		KeyDir:        keyDir,
		ValidEOF:      offset,
		NextSeqNo:     nextSeqNo,
		RecoveredOps:  recoveredOps,
		TornTailFixed: tornTailFixed,
	}, nil
}

// repairTornTail performs an idempotent physical truncation followed immediately by fsync.
// Diagnostic messages are written strictly to os.Stderr to protect stdout protocol isolation.
func repairTornTail(f *os.File, truncateTo, prevFileSize uint64, reason string) error {
	if err := f.Truncate(int64(truncateTo)); err != nil {
		return fmt.Errorf("failed to truncate torn frame tail (%s): %w", reason, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync torn tail truncation (%s): %w", reason, err)
	}
	fmt.Fprintf(os.Stderr, "[RECOVERY] Repaired torn tail at offset %d (file truncated from %d to %d bytes: %s)\n",
		truncateTo, prevFileSize, truncateTo, reason)
	return nil
}

// hasValidFrameAhead scans forward from start to fileSize using a 64 KiB chunked buffer
// and SIMD-accelerated bytes.Index to detect subsequent valid UABE frames.
func hasValidFrameAhead(f *os.File, start, fileSize uint64) (bool, error) {
	if start+format.FrameHeaderSize > fileSize {
		return false, nil
	}

	chunkBuf := make([]byte, scanChunkSize+format.FrameHeaderSize)
	currOffset := start

	for currOffset+format.FrameHeaderSize <= fileSize {
		toRead := scanChunkSize + format.FrameHeaderSize
		if currOffset+uint64(toRead) > fileSize {
			toRead = int(fileSize - currOffset)
		}

		n, err := f.ReadAt(chunkBuf[:toRead], int64(currOffset))
		if err != nil && err != io.EOF {
			return false, err
		}
		if n < format.FrameHeaderSize {
			break
		}

		validWindow := chunkBuf[:n]
		searchPos := 0

		for searchPos+format.FrameHeaderSize <= n {
			idx := bytes.Index(validWindow[searchPos:], frameMagicBytes)
			if idx < 0 {
				break
			}
			candidateOffset := searchPos + idx
			if candidateOffset+format.FrameHeaderSize > n {
				break
			}
			candidateHeader := validWindow[candidateOffset : candidateOffset+format.FrameHeaderSize]

			if _, decErr := format.DecodeFrameHeader(candidateHeader); decErr == nil {
				return true, nil
			}
			searchPos = candidateOffset + 1
		}

		advance := n - format.FrameHeaderSize + 1
		if advance <= 0 {
			break
		}
		currOffset += uint64(advance)
	}

	return false, nil
}

// applyPayloadToKeyDir validates trailer CRC and payload structure in-place,
// applying subrecords directly to KeyDir without allocating intermediate slice representations.
func applyPayloadToKeyDir(payload []byte, trailer []byte, opCount uint32, frameOffset, seqNo uint64, keyDir map[string]KeyDirEntry) (uint64, error) {
	if len(trailer) < format.FrameTrailerSize {
		return 0, format.ErrTrailerTooShort
	}
	storedTrailerCRC := binary.LittleEndian.Uint32(trailer[0:4])
	computedTrailerCRC := format.ChecksumCastagnoli(payload)
	if storedTrailerCRC != computedTrailerCRC {
		return 0, format.ErrCorruptPayload
	}

	if opCount < 1 {
		return 0, format.ErrBatchEmpty
	}
	if opCount > format.MaxBatchOps {
		return 0, format.ErrBatchTooLarge
	}

	minPayload := uint64(opCount) * uint64(format.MinRecordSize)
	if uint64(len(payload)) < minPayload {
		return 0, format.ErrPayloadUnderflow
	}

	cursor := 0
	payloadLen := len(payload)

	// Apply subrecords directly to KeyDir in a single pass
	for i := uint32(0); i < opCount; i++ {
		if cursor+format.RecordHeaderSize > payloadLen {
			return 0, format.ErrPayloadUnderflow
		}
		opType := format.OpType(payload[cursor])
		keyLen := int(binary.LittleEndian.Uint16(payload[cursor+1 : cursor+3]))
		valLen := binary.LittleEndian.Uint32(payload[cursor+3 : cursor+7])

		if opType != format.OpPut && opType != format.OpDel {
			return 0, format.ErrUnknownOpType
		}
		if keyLen < format.MinKeyLen {
			return 0, format.ErrKeyEmpty
		}
		if keyLen > format.MaxKeyLen {
			return 0, format.ErrKeyTooLarge
		}
		if opType == format.OpDel {
			if valLen != 0 {
				return 0, format.ErrDelNonZeroValue
			}
		} else {
			if valLen > format.MaxValLen {
				return 0, format.ErrValueTooLarge
			}
		}

		subrecordLen := format.RecordHeaderSize + keyLen + int(valLen)
		if cursor+subrecordLen > payloadLen {
			return 0, format.ErrPayloadUnderflow
		}

		key := string(payload[cursor+format.RecordHeaderSize : cursor+format.RecordHeaderSize+keyLen])
		if opType == format.OpPut {
			valOff := format.ValueFileOffset(frameOffset, uint32(cursor), uint16(keyLen))
			keyDir[key] = NewKeyDirEntry(valOff, valLen, seqNo)
		} else {
			delete(keyDir, key)
		}
		cursor += subrecordLen
	}

	if cursor != payloadLen {
		return 0, format.ErrTrailingPayloadBytes
	}

	return uint64(opCount), nil
}

