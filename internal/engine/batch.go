package engine

import (
	"math"

	"kvdb/internal/format"
)

// Batch applies 1..10,000 operations atomically within a single UABE frame (Phase E4).
// It strictly guarantees:
// 1. Pre-flight bounds and operation validation.
// 2. Pre-write quota validation against total frame size.
// 3. Serialization into a single contiguous UABE frame.
// 4. Physical writeAtFull followed by fsync barrier before any index publication.
// 5. Atomic KeyDir publication with exact physical locators and Last-Operation-Wins resolution.
// 6. Reader isolation: readers see either 100% pre-batch or 100% post-batch state.
func (e *Engine) Batch(ops []format.Operation) error {
	// Step 1: Pre-flight bounds and operation validation
	if len(ops) < 1 {
		return format.ErrBatchEmpty
	}
	if len(ops) > format.MaxBatchOps {
		return format.ErrBatchTooLarge
	}

	for i := range ops {
		if err := format.ValidateOperation(ops[i]); err != nil {
			return err
		}
	}

	payloadLen, err := format.CalculatePayloadLength(ops)
	if err != nil {
		return err
	}

	totalFrameSize := format.TotalFrameSize(payloadLen)

	// Step 2: Acquire exclusive write lock
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrClosed
	}
	if e.poisoned {
		return ErrEnginePoisoned
	}

	// Step 3: Quota validation with overflow safety
	if totalFrameSize > math.MaxUint64-e.validEOF {
		return ErrDiskFull
	}
	if e.maxBytes > 0 && e.validEOF+totalFrameSize > e.maxBytes {
		return ErrDiskFull
	}

	// Step 4-6: Frame encoding, position-independent write, and physical fsync barrier
	frameBytes, err := e.appendFrameLocked(ops)
	if err != nil {
		return err
	}

	// Step 7: Atomic in-memory KeyDir publication
	// Directly compute payload offsets in O(N) without redundant re-decoding or heap churn
	currPayloadOffset := uint32(0)
	for i := range ops {
		op := ops[i]
		if op.Type == format.OpPut {
			valOffset := format.ValueFileOffset(e.validEOF, currPayloadOffset, uint16(len(op.Key)))
			e.keyDir[string(op.Key)] = NewKeyDirEntry(valOffset, uint32(len(op.Value)), e.nextSeqNo)
		} else if op.Type == format.OpDel {
			delete(e.keyDir, string(op.Key))
		}
		currPayloadOffset += uint32(format.RecordHeaderSize + len(op.Key) + len(op.Value))
	}

	// Step 8: Metadata advancement
	e.validEOF += uint64(len(frameBytes))
	e.nextSeqNo++

	// Step 9-10: Lock released by defer; return nil
	return nil
}

