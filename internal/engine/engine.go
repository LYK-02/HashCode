package engine

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"kvdb/internal/format"
)

var (
	ErrLocked         = errors.New("LOCKED: database lock held by another process")
	ErrClosed         = errors.New("CLOSED: engine instance has been closed")
	ErrNotFound       = errors.New("NOTFOUND: key not present in database")
	ErrDiskFull       = errors.New("DISK_FULL: file size limit exceeded")
	ErrEnginePoisoned = errors.New("ENGINE_POISONED: previous write rollback failed")
)

// Pair represents a live key-value pair returned by Dump.
type Pair struct {
	Key   string
	Value []byte
}

// Options configures engine operational parameters.
type Options struct {
	MaxBytes uint64
}

// Engine implements the single-file append-only key-value storage engine.
type Engine struct {
	mu            sync.RWMutex
	db            *os.File
	lockFile      *os.File
	path          string
	parentDir     string
	maxBytes      uint64
	validEOF      uint64
	nextSeqNo     uint64
	compactionGen uint64
	keyDir        map[string]KeyDirEntry
	closed        bool
	poisoned      bool
}

// Open canonicalizes path, acquires the sidecar lock, initializes/validates the data file header,
// replays valid log frames into KeyDir, and returns a ready Engine instance.
func Open(path string, opts Options) (*Engine, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	cleanPath := filepath.Clean(absPath)
	parentDir := filepath.Dir(cleanPath)

	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", parentDir, err)
	}

	lockPath := cleanPath + ".lock"
	lockFile, err := acquireLock(lockPath)
	if err != nil {
		return nil, err
	}

	// Clean up any un-renamed compaction temp file from previous crash
	_ = os.Remove(cleanPath + ".compact.tmp")

	// Defensive cleanup on initialization error
	cleanup := true
	defer func() {
		if cleanup {
			_ = releaseLock(lockFile)
		}
	}()

	// Persist or load logical maxBytes configuration in the lock sidecar
	if opts.MaxBytes > 0 {
		var mbBuf [8]byte
		binary.LittleEndian.PutUint64(mbBuf[:], opts.MaxBytes)
		_, _ = lockFile.WriteAt(mbBuf[:], 0)
	} else {
		var mbBuf [8]byte
		if n, _ := lockFile.ReadAt(mbBuf[:], 0); n == 8 {
			opts.MaxBytes = binary.LittleEndian.Uint64(mbBuf[:])
		}
	}

	dataFile, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open data file %s: %w", cleanPath, err)
	}
	defer func() {
		if cleanup {
			_ = dataFile.Close()
		}
	}()

	fi, err := dataFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat data file: %w", err)
	}

	var (
		validEOF      uint64
		nextSeqNo     uint64 = 1
		compactionGen uint64 = 0
		keyDir               = make(map[string]KeyDirEntry)
	)

	if fi.Size() == 0 {
		// New database: write 32-byte DB Header (Gen 0)
		hdr := format.NewHeader(0)
		hdrBytes, err := format.EncodeHeader(&hdr)
		if err != nil {
			return nil, fmt.Errorf("failed to encode db header: %w", err)
		}
		if err := writeAtFull(dataFile, hdrBytes, 0); err != nil {
			return nil, fmt.Errorf("failed to write db header: %w", err)
		}
		if err := dataFile.Sync(); err != nil {
			return nil, fmt.Errorf("failed to sync db header: %w", err)
		}
		if err := syncDir(parentDir); err != nil {
			return nil, fmt.Errorf("failed to sync parent dir: %w", err)
		}
		validEOF = uint64(format.HeaderSize)
	} else if fi.Size() < int64(format.HeaderSize) {
		// Non-empty file with incomplete header
		return nil, format.ErrHeaderTooShort
	} else {
		// Existing database: read and validate header at offset 0
		hdrBuf := make([]byte, format.HeaderSize)
		if err := readAtFull(dataFile, hdrBuf, 0); err != nil {
			return nil, fmt.Errorf("failed to read db header: %w", err)
		}
		hdr, err := format.DecodeHeader(hdrBuf)
		if err != nil {
			return nil, err // Fail closed: ErrCorruptHeader, ErrInvalidHeaderMagic, etc.
		}
		compactionGen = hdr.CompactionGen
		validEOF = uint64(format.HeaderSize)

		// Perform log recovery & torn-tail repair via internal/engine/recovery.go
		recResult, err := RecoverLog(dataFile, uint64(fi.Size()))
		if err != nil {
			return nil, err
		}
		keyDir = recResult.KeyDir
		validEOF = recResult.ValidEOF
		nextSeqNo = recResult.NextSeqNo
	}

	// Engine successfully initialized; transfer ownership of descriptors
	cleanup = false

	return &Engine{
		path:          cleanPath,
		parentDir:     parentDir,
		db:            dataFile,
		lockFile:      lockFile,
		maxBytes:      opts.MaxBytes,
		validEOF:      validEOF,
		nextSeqNo:     nextSeqNo,
		compactionGen: compactionGen,
		keyDir:        keyDir,
		closed:        false,
	}, nil
}

// Close flushes buffers, syncs data file to physical disk, closes descriptors,
// and releases the flock. The lock sidecar is preserved on disk.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return ErrClosed
	}
	e.closed = true

	var firstErr error

	if e.db != nil {
		if err := e.db.Sync(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to sync db: %w", err)
		}
		if err := e.db.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close db: %w", err)
		}
		e.db = nil
	}

	if e.lockFile != nil {
		if err := releaseLock(e.lockFile); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to release lock: %w", err)
		}
		e.lockFile = nil
	}

	return firstErr
}

// syncDir forces the directory entry containing the database file to physical disk.
func syncDir(dirPath string) error {
	d, err := os.Open(dirPath)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Dump returns all live key-value pairs sorted lexicographically.
func (e *Engine) Dump() ([]Pair, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.closed {
		return nil, ErrClosed
	}

	pairs := make([]Pair, 0, len(e.keyDir))
	for k, entry := range e.keyDir {
		if entry.ValueLen == 0 {
			pairs = append(pairs, Pair{Key: k, Value: []byte{}})
			continue
		}
		buf := make([]byte, entry.ValueLen)
		if err := readAtFull(e.db, buf, int64(entry.ValueOffset)); err != nil {
			return nil, err
		}
		pairs = append(pairs, Pair{Key: k, Value: buf})
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Key < pairs[j].Key
	})
	return pairs, nil
}

// Check verifies the structural integrity of the database header and all frames.
func (e *Engine) Check() error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.closed {
		return ErrClosed
	}
	return CheckFile(e.path)
}

// CheckFile verifies the structural integrity of the database header and all frames on disk.
func CheckFile(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	cleanPath := filepath.Clean(absPath)

	f, err := os.Open(cleanPath)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}

	if fi.Size() < format.HeaderSize {
		return format.ErrHeaderTooShort
	}

	hdrBuf := make([]byte, format.HeaderSize)
	if _, err := f.ReadAt(hdrBuf, 0); err != nil {
		return err
	}
	if _, err := format.DecodeHeader(hdrBuf); err != nil {
		return err
	}

	offset := uint64(format.HeaderSize)
	fileSize := uint64(fi.Size())
	for offset < fileSize {
		if offset+format.FrameHeaderSize > fileSize {
			return format.ErrFrameHeaderTooShort
		}
		hdr := make([]byte, format.FrameHeaderSize)
		if _, err := f.ReadAt(hdr, int64(offset)); err != nil {
			return err
		}
		fh, err := format.DecodeFrameHeader(hdr)
		if err != nil {
			return err
		}
		totalSize := format.TotalFrameSize(fh.PayloadLen)
		if offset+totalSize > fileSize {
			return format.ErrTruncatedFrame
		}
		payload := make([]byte, fh.PayloadLen)
		if _, err := f.ReadAt(payload, int64(offset+format.FrameHeaderSize)); err != nil {
			return err
		}
		trailer := make([]byte, format.FrameTrailerSize)
		if _, err := f.ReadAt(trailer, int64(offset+format.FrameHeaderSize+fh.PayloadLen)); err != nil {
			return err
		}
		if _, err := format.DecodePayloadRecords(payload, trailer, fh.OpCount); err != nil {
			return err
		}
		offset += totalSize
	}
	return nil
}
