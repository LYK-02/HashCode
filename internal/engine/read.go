package engine

import (
	"os"

	"kvdb/internal/format"
)

// Get retrieves the value associated with key, returning (value, true, nil) or (nil, false, nil).
func (e *Engine) Get(key []byte) ([]byte, bool, error) {
	if len(key) < format.MinKeyLen {
		return nil, false, format.ErrKeyEmpty
	}
	if len(key) > format.MaxKeyLen {
		return nil, false, format.ErrKeyTooLarge
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return nil, false, ErrClosed
	}

	entry, found := e.keyDir[string(key)]
	if !found {
		return nil, false, nil
	}

	// 0-byte value: key is present, return empty slice without disk I/O
	if entry.ValueLen == 0 {
		return []byte{}, true, nil
	}

	buf := make([]byte, entry.ValueLen)
	if err := readAtFull(e.db, buf, int64(entry.ValueOffset)); err != nil {
		return nil, false, err
	}

	return buf, true, nil
}

// GetMulti retrieves values for multiple keys atomically under a single reader lock.
func (e *Engine) GetMulti(keys [][]byte) ([][]byte, []bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return nil, nil, ErrClosed
	}

	vals := make([][]byte, len(keys))
	founds := make([]bool, len(keys))

	for i, key := range keys {
		if len(key) < format.MinKeyLen {
			return nil, nil, format.ErrKeyEmpty
		}
		if len(key) > format.MaxKeyLen {
			return nil, nil, format.ErrKeyTooLarge
		}

		entry, found := e.keyDir[string(key)]
		if !found {
			founds[i] = false
			continue
		}
		founds[i] = true

		if entry.ValueLen == 0 {
			vals[i] = []byte{}
			continue
		}

		buf := make([]byte, entry.ValueLen)
		if err := readAtFull(e.db, buf, int64(entry.ValueOffset)); err != nil {
			return nil, nil, err
		}
		vals[i] = buf
	}

	return vals, founds, nil
}

// readAtFull loops until all bytes in buf are read from f at off.
func readAtFull(f *os.File, buf []byte, off int64) error {
	read := 0
	for read < len(buf) {
		n, err := f.ReadAt(buf[read:], off+int64(read))
		read += n
		if err != nil {
			return err
		}
	}
	return nil
}
