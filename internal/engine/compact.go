package engine

import (
	"fmt"
	"os"
	"sort"

	"kvdb/internal/format"
)

// Compact triggers a crash-safe online compaction rewriting live records to a new generation (Phase E8).
// It strictly follows the 11-step compaction protocol:
// 1. Acquire exclusive write lock (no concurrent mutations).
// 2. Check engine state (closed / poisoned).
// 3. Create <db>.compact.tmp in parent directory.
// 4. Write valid 32-byte header with incremented compaction generation.
// 5. Read all live keys in deterministic lexicographical order.
// 6. Encode live values into valid UABE frames.
// 7. Physical write loop with quota checks.
// 8. Fsync temp file and close descriptor.
// 9. Atomically rename temp file over authoritative database path.
// 10. Sync parent directory to persist the directory entry.
// 11. Reopen new database descriptor and swap KeyDir/EOF/SeqNo/Gen under lock.
func (e *Engine) Compact() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrClosed
	}
	if e.poisoned {
		return ErrEnginePoisoned
	}

	tempPath := e.path + ".compact.tmp"
	_ = os.Remove(tempPath)

	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("failed to create compaction temp file %s: %w", tempPath, err)
	}

	success := false
	defer func() {
		if !success {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
		}
	}()

	// Step 4: Write header with incremented generation
	nextGen := e.compactionGen + 1
	hdr := format.NewHeader(nextGen)
	hdrBytes, err := format.EncodeHeader(&hdr)
	if err != nil {
		return fmt.Errorf("failed to encode compaction header: %w", err)
	}
	if err := writeAtFull(tempFile, hdrBytes, 0); err != nil {
		return fmt.Errorf("failed to write compaction header: %w", err)
	}

	// Step 5: Read live keys in deterministic sorted order
	liveKeys := make([]string, 0, len(e.keyDir))
	for k := range e.keyDir {
		liveKeys = append(liveKeys, k)
	}
	sort.Strings(liveKeys)

	// Step 6-7: Encode and write live records in bounded frames (up to 1,000 ops per frame)
	const batchChunkSize = 1000
	var tempEOF uint64 = uint64(format.HeaderSize)
	var nextSeq uint64 = 1
	newKeyDir := make(map[string]KeyDirEntry, len(e.keyDir))

	for i := 0; i < len(liveKeys); i += batchChunkSize {
		end := i + batchChunkSize
		if end > len(liveKeys) {
			end = len(liveKeys)
		}
		chunkKeys := liveKeys[i:end]
		ops := make([]format.Operation, len(chunkKeys))

		for j, k := range chunkKeys {
			entry := e.keyDir[k]
			var valBuf []byte
			if entry.ValueLen > 0 {
				valBuf = make([]byte, entry.ValueLen)
				if err := readAtFull(e.db, valBuf, int64(entry.ValueOffset)); err != nil {
					return fmt.Errorf("failed to read key %s during compaction: %w", k, err)
				}
			}
			ops[j] = format.Operation{
				Type:  format.OpPut,
				Key:   []byte(k),
				Value: valBuf,
			}
		}

		frameBytes, err := format.EncodeFrame(nextSeq, ops)
		if err != nil {
			return fmt.Errorf("failed to encode compaction frame %d: %w", nextSeq, err)
		}

		// Quota validation
		if e.maxBytes > 0 && tempEOF+uint64(len(frameBytes)) > e.maxBytes {
			return ErrDiskFull
		}

		if err := writeAtFull(tempFile, frameBytes, int64(tempEOF)); err != nil {
			return fmt.Errorf("failed to write compaction frame %d: %w", nextSeq, err)
		}

		// Populate new physical locators
		currPayloadOffset := uint32(0)
		for _, op := range ops {
			valOffset := format.ValueFileOffset(tempEOF, currPayloadOffset, uint16(len(op.Key)))
			newKeyDir[string(op.Key)] = NewKeyDirEntry(valOffset, uint32(len(op.Value)), nextSeq)
			currPayloadOffset += uint32(format.RecordHeaderSize + len(op.Key) + len(op.Value))
		}

		tempEOF += uint64(len(frameBytes))
		nextSeq++
	}

	// Step 8: Fsync temp file and close descriptor
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync compaction temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close compaction temp file: %w", err)
	}

	// Step 9: Atomically rename temp file over authoritative database path
	if err := os.Rename(tempPath, e.path); err != nil {
		return fmt.Errorf("failed to rename compaction temp file: %w", err)
	}

	// Step 10: Sync parent directory
	if err := syncDir(e.parentDir); err != nil {
		return fmt.Errorf("failed to sync parent directory after compaction: %w", err)
	}

	// Step 11: Reopen database descriptor and swap in-memory state
	_ = e.db.Close()
	newDB, err := os.OpenFile(e.path, os.O_RDWR, 0666)
	if err != nil {
		e.poisoned = true
		return fmt.Errorf("failed to reopen compacted database: %w", err)
	}

	e.db = newDB
	e.validEOF = tempEOF
	e.nextSeqNo = nextSeq
	e.compactionGen = nextGen
	e.keyDir = newKeyDir

	success = true
	return nil
}

// CompactionGen returns the current compaction generation.
func (e *Engine) CompactionGen() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.compactionGen
}

