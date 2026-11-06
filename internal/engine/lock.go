//go:build !windows

package engine

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// acquireLock creates or opens the permanent sidecar lock file and acquires an exclusive non-blocking flock.
// On contention, it returns ErrLocked. The lock file is NEVER unlinked or deleted.
func acquireLock(lockPath string) (*os.File, error) {
	// Open sidecar with O_CREATE|O_RDWR without truncation
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file %s: %w", lockPath, err)
	}

	// Non-blocking exclusive advisory flock
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("failed to acquire flock on %s: %w", lockPath, err)
	}

	return f, nil
}

// releaseLock explicitly unlocks and closes the lock file descriptor.
// It deliberately leaves the sidecar file intact on the filesystem.
func releaseLock(f *os.File) error {
	if f == nil {
		return nil
	}
	var firstErr error
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		firstErr = err
	}
	if err := f.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}
