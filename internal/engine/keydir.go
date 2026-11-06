package engine

import (
	"kvdb/internal/format"
)

// KeyDirEntry stores in-memory index locators pointing directly to value bytes in the single data file.
type KeyDirEntry struct {
	ValueOffset uint64 // Absolute byte offset of value in data.db
	FileOffset  uint64 // Alias for ValueOffset for backward compatibility
	ValueLen    uint32 // Length of value in bytes
	SeqNo       uint64 // Sequence number of the operation that established this entry
}

// NewKeyDirEntry constructs a KeyDirEntry ensuring both ValueOffset and FileOffset are populated.
func NewKeyDirEntry(valOffset uint64, valLen uint32, seqNo uint64) KeyDirEntry {
	return KeyDirEntry{
		ValueOffset: valOffset,
		FileOffset:  valOffset,
		ValueLen:    valLen,
		SeqNo:       seqNo,
	}
}

// SingleOpValueOffset computes the absolute value file offset for a single PUT frame at frameStart.
func SingleOpValueOffset(frameStart uint64, keyLen int) uint64 {
	return format.ValueFileOffset(frameStart, 0, uint16(keyLen))
}

// KeyCount returns the number of live keys in the database index under a read lock.
func (e *Engine) KeyCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.keyDir)
}

// ValidEOF returns the current durable EOF offset under a read lock.
func (e *Engine) ValidEOF() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.validEOF
}

// NextSeqNo returns the next sequence number to be assigned under a read lock.
func (e *Engine) NextSeqNo() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.nextSeqNo
}

