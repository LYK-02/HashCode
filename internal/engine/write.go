package engine

import (
	"math"
	"os"

	"kvdb/internal/format"
)

// Put inserts or updates a key-value pair following the strict 10-step write protocol.
func (e *Engine) Put(key, val []byte) error {
	// Step 1: Input validation
	if len(key) < format.MinKeyLen {
		return format.ErrKeyEmpty
	}
	if len(key) > format.MaxKeyLen {
		return format.ErrKeyTooLarge
	}
	if len(val) > format.MaxValLen {
		return format.ErrValueTooLarge
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

	// Step 3-6: Encode, quota check, position-independent write, and physical fsync barrier
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
func (e *Engine) Delete(key []byte) error {
	// Step 1: Input validation
	if len(key) < format.MinKeyLen {
		return format.ErrKeyEmpty
	}
	if len(key) > format.MaxKeyLen {
		return format.ErrKeyTooLarge
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

	// Step 3-6: Encode, quota check, position-independent write, and physical fsync barrier
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

// appendFrameLocked handles serialization, quota check, writeAtFull, fsync barrier, and rollback.
// Must be called with e.mu held exclusively.
func (e *Engine) appendFrameLocked(ops []format.Operation) ([]byte, error) {
	frameBytes, err := format.EncodeFrame(e.nextSeqNo, ops)
	if err != nil {
		return nil, err
	}

	frameSize := uint64(len(frameBytes))

	// Quota validation with overflow safety
	if frameSize > math.MaxUint64-e.validEOF {
		return nil, ErrDiskFull
	}
	if e.maxBytes > 0 && e.validEOF+frameSize > e.maxBytes {
		return nil, ErrDiskFull
	}

	// Position-independent write loop
	if err := writeAtFull(e.db, frameBytes, int64(e.validEOF)); err != nil {
		e.rollbackAppendLocked(err)
		return nil, err
	}

	// Physical fsync barrier
	if err := e.db.Sync(); err != nil {
		e.rollbackAppendLocked(err)
		return nil, err
	}

	return frameBytes, nil
}

// rollbackAppendLocked attempts to truncate back to validEOF and sync after a failed write or sync.
func (e *Engine) rollbackAppendLocked(cause error) {
	truncErr := e.db.Truncate(int64(e.validEOF))
	syncErr := e.db.Sync()
	if truncErr != nil || syncErr != nil {
		e.poisoned = true
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

