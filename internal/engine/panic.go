package engine

import (
	"fmt"
	"os"
	"runtime/debug"
)

// recoverPanicLocked catches unexpected panics during mutating engine calls (Put, Delete, Batch, Compact).
// It poisons the engine, logs the stack trace to os.Stderr, and sets *retErr.
func (e *Engine) recoverPanicLocked(action string, retErr *error) {
	if r := recover(); r != nil {
		if e != nil {
			e.poisoned = true
		}
		*retErr = fmt.Errorf("ENGINE_PANIC: %s: %v", action, r)
		fmt.Fprintf(os.Stderr, "FATAL: engine panic in %s: %v\nstack:\n%s\n", action, r, debug.Stack())
	}
}

// recoverPanicReadOnly catches unexpected panics during read-only engine calls (Get, Dump, Check).
// It sets *retErr and logs to os.Stderr without poisoning the engine.
func (e *Engine) recoverPanicReadOnly(action string, retErr *error) {
	if r := recover(); r != nil {
		*retErr = fmt.Errorf("ENGINE_PANIC: %s: %v", action, r)
		fmt.Fprintf(os.Stderr, "ERROR: engine panic in read %s: %v\nstack:\n%s\n", action, r, debug.Stack())
	}
}
