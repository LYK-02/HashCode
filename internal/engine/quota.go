package engine

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"

	"kvdb/internal/format"
)

// ExactPutFrameSize computes exact frame length for a PUT operation:
// FrameHeader (28B) + RecordHeader (7B) + keyLen + valLen + FrameTrailer (4B) = 39 + keyLen + valLen.
// O(1) time complexity, ZERO heap allocation.
func ExactPutFrameSize(keyLen, valLen int) (uint64, error) {
	if keyLen < format.MinKeyLen {
		return 0, format.ErrKeyEmpty
	}
	if keyLen > format.MaxKeyLen {
		return 0, format.ErrKeyTooLarge
	}
	if valLen > format.MaxValLen {
		return 0, format.ErrValueTooLarge
	}
	payloadLen := uint64(format.RecordHeaderSize + keyLen + valLen)
	return uint64(format.FrameEnvelopeSize) + payloadLen, nil
}

// ExactDelFrameSize computes exact frame length for a DEL operation:
// FrameHeader (28B) + RecordHeader (7B) + keyLen + FrameTrailer (4B) = 39 + keyLen.
// O(1) time complexity, ZERO heap allocation.
func ExactDelFrameSize(keyLen int) (uint64, error) {
	if keyLen < format.MinKeyLen {
		return 0, format.ErrKeyEmpty
	}
	if keyLen > format.MaxKeyLen {
		return 0, format.ErrKeyTooLarge
	}
	payloadLen := uint64(format.RecordHeaderSize + keyLen)
	return uint64(format.FrameEnvelopeSize) + payloadLen, nil
}

// ExactBatchFrameSize computes exact frame length for an atomic batch of operations.
// Validates all operations and sums payload subrecords in O(N) time with ZERO buffer allocation.
func ExactBatchFrameSize(ops []format.Operation) (uint64, error) {
	if len(ops) < 1 {
		return 0, format.ErrBatchEmpty
	}
	if len(ops) > format.MaxBatchOps {
		return 0, format.ErrBatchTooLarge
	}
	for i := range ops {
		if err := format.ValidateOperation(ops[i]); err != nil {
			return 0, err
		}
	}
	payloadLen, err := format.CalculatePayloadLength(ops)
	if err != nil {
		return 0, err
	}
	return uint64(format.FrameEnvelopeSize) + payloadLen, nil
}

// CheckQuota validates that adding frameSize to validEOF will not overflow uint64
// and will not exceed maxBytes (when maxBytes > 0).
// Returns ErrDiskFull immediately if the capacity check fails.
func CheckQuota(validEOF, frameSize, maxBytes uint64) error {
	// 64-bit unsigned overflow check
	if frameSize > math.MaxUint64-validEOF {
		return ErrDiskFull
	}
	// Logical quota check
	if maxBytes > 0 && validEOF+frameSize > maxBytes {
		return ErrDiskFull
	}
	return nil
}

// PersistQuota writes the 8-byte uint64 maxBytes to offset 0 of the lock sidecar and fsyncs.
func PersistQuota(lockFile *os.File, maxBytes uint64) error {
	if lockFile == nil {
		return nil
	}
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], maxBytes)
	if _, err := lockFile.WriteAt(buf[:], 0); err != nil {
		return fmt.Errorf("failed to write quota to lock file: %w", err)
	}
	if err := lockFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync quota to lock file: %w", err)
	}
	return nil
}

// LoadQuota reads the 8-byte uint64 maxBytes configuration from offset 0 of the lock sidecar.
func LoadQuota(lockFile *os.File) (uint64, error) {
	if lockFile == nil {
		return 0, nil
	}
	var buf [8]byte
	n, err := lockFile.ReadAt(buf[:], 0)
	if n < 8 {
		return 0, nil
	}
	if err != nil && err != io.EOF && err != os.ErrClosed {
		// Non-fatal if 8 bytes were read
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}

// ClassifyErrorCode maps any error to canonical stable error code taxonomy:
// INVALID_ARGUMENT, KEY_TOO_LARGE, VALUE_TOO_LARGE, BATCH_TOO_LARGE, DISK_FULL,
// IO_WRITE, IO_SYNC, IO_TRUNCATE, CORRUPT_HEADER, CORRUPT_FRAME, LOCKED, CLOSED, ENGINE_POISONED.
func ClassifyErrorCode(err error) (string, string) {
	if err == nil {
		return "OK", ""
	}
	msg := err.Error()
	for _, code := range []string{
		"DISK_FULL", "ENGINE_POISONED", "LOCKED", "CLOSED", "NOTFOUND",
		"INVALID_ARGUMENT", "KEY_TOO_LARGE", "VALUE_TOO_LARGE", "BATCH_TOO_LARGE",
		"IO_WRITE", "IO_SYNC", "IO_TRUNCATE", "IO_RENAME", "IO_DIR_SYNC",
		"CORRUPT_HEADER", "CORRUPT_FRAME",
	} {
		if len(msg) >= len(code) && msg[:len(code)] == code {
			detail := ""
			if len(msg) > len(code)+1 {
				detail = msg[len(code)+1:]
				if len(detail) > 0 && detail[0] == ' ' {
					detail = detail[1:]
				}
			}
			return code, detail
		}
	}
	return "INTERNAL_ERROR", msg
}
