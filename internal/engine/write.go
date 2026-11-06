package engine

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"kvdb/internal/format"
)

// Put inserts or updates a key-value pair following the strict 10-step write protocol,
// zero-panic recovery, and pre-write quota enforcement.
func (e *Engine) Put(key, val []byte) (retErr error) {
	defer e.recoverPanicLocked("Put", &retErr)

	// Step 1: Pre-flight bounds and zero-allocation frame size calculation
	frameSize, err := ExactPutFrameSize(len(key), len(val))
	if err != nil {
		return err
	}

	// Step 2: Acquire exclusive write lock
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrClosed
	}
	if e.poisoned {
		return ErrEnginePoisoned
	}

	// Step 3: Pre-write quota check (rejection before memory allocation or disk writes)
	if err := CheckQuota(e.validEOF, frameSize, e.maxBytes); err != nil {
		return err
	}

	// Step 4-6: Frame encode, position-independent write, and physical fsync barrier
	ops := []format.Operation{{
		Type:  format.OpPut,
		Key:   key,
		Value: val,
	}}
	frameBytes, err := e.appendFrameLocked(ops)
	if err != nil {
		return err
	}

	// Step 7: Index publication
	valOffset := format.ValueFileOffset(e.validEOF, 0, uint16(len(key)))
	e.keyDir[string(key)] = NewKeyDirEntry(valOffset, uint32(len(val)), e.nextSeqNo)

	// Step 8: Metadata update
	e.validEOF += uint64(len(frameBytes))
	e.nextSeqNo++

	// Step 9 & 10: Lock released by defer; return nil success
	return nil
}

// Delete appends a tombstone record removing the key. Deleting missing keys is valid and durable.
func (e *Engine) Delete(key []byte) (retErr error) {
	defer e.recoverPanicLocked("Delete", &retErr)

	// Step 1: Pre-flight bounds and zero-allocation frame size calculation
	frameSize, err := ExactDelFrameSize(len(key))
	if err != nil {
		return err
	}

	// Step 2: Acquire exclusive write lock
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrClosed
	}
	if e.poisoned {
		return ErrEnginePoisoned
	}

	// Step 3: Pre-write quota check
	if err := CheckQuota(e.validEOF, frameSize, e.maxBytes); err != nil {
		return err
	}

	// Step 4-6: Frame encode, position-independent write, and physical fsync barrier
	ops := []format.Operation{{
		Type:  format.OpDel,
		Key:   key,
		Value: nil,
	}}
	frameBytes, err := e.appendFrameLocked(ops)
	if err != nil {
		return err
	}

	// Step 7: Index publication (remove from KeyDir)
	delete(e.keyDir, string(key))

	// Step 8: Metadata update
	e.validEOF += uint64(len(frameBytes))
	e.nextSeqNo++

	// Step 9 & 10: Lock released by defer; return nil success
	return nil
}

// appendFrameLocked handles serialization, writeAtFull, fsync barrier, and rollback.
// Must be called with e.mu held exclusively.
func (e *Engine) appendFrameLocked(ops []format.Operation) ([]byte, error) {
	frameBytes, err := format.EncodeFrame(e.nextSeqNo, ops)
	if err != nil {
		return nil, err
	}

	// Position-independent write loop
	if err := writeAtFull(e.db, frameBytes, int64(e.validEOF)); err != nil {
		e.rollbackAppendLocked(err)
		if errors.Is(err, syscall.ENOSPC) {
			return nil, ErrDiskFull
		}
		return nil, fmt.Errorf("IO_WRITE: %w", err)
	}

	// Physical fsync barrier
	if err := e.db.Sync(); err != nil {
		e.rollbackAppendLocked(err)
		return nil, fmt.Errorf("IO_SYNC: %w", err)
	}

	return frameBytes, nil
}

// rollbackAppendLocked attempts to truncate back to validEOF and sync after a failed write or sync.
// Poisons engine on truncate or sync failure.
func (e *Engine) rollbackAppendLocked(cause error) {
	truncErr := e.db.Truncate(int64(e.validEOF))
	syncErr := e.db.Sync()
	if truncErr != nil || syncErr != nil {
		e.poisoned = true
		fmt.Fprintf(os.Stderr, "ENGINE_POISONED: rollback failed: validEOF=%d truncErr=%v syncErr=%v (cause: %v)\n",
			e.validEOF, truncErr, syncErr, cause)
	}
}

// writeAtFull loops until all bytes in buf are written to f at off.
func writeAtFull(f *os.File, buf []byte, off int64) error {
	written := 0
	for written < len(buf) {
		n, err := f.WriteAt(buf[written:], off+int64(written))
		written += n
		if err != nil {
			return err
		}
	}
	return nil
}
