package protocol

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"sync"

	"kvdb/internal/engine"
	"kvdb/internal/format"
)

// Server implements the streaming stdin/stdout protocol server.
type Server struct {
	eng      *engine.Engine
	in       io.Reader
	out      io.Writer
	errOut   io.Writer
	writerMu sync.Mutex
	engMu    sync.RWMutex
}

// NewServer initializes a Server instance.
func NewServer(eng *engine.Engine, in io.Reader, out io.Writer, errOut io.Writer) *Server {
	if errOut == nil {
		errOut = os.Stderr
	}
	return &Server{
		eng:    eng,
		in:     in,
		out:    out,
		errOut: errOut,
	}
}

// Run executes the streaming serve loop until EOF or unrecoverable error.
func (s *Server) Run() error {
	reader := bufio.NewReaderSize(s.in, 1024*1024) // 1 MiB buffer
	outBuf := bufio.NewWriter(s.out)

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			resp, shouldExit := s.dispatchSafe(line)
			if len(resp) > 0 {
				s.writerMu.Lock()
				_, writeErr := outBuf.WriteString(resp)
				if writeErr == nil {
					writeErr = outBuf.Flush()
				}
				s.writerMu.Unlock()

				if writeErr != nil {
					fmt.Fprintf(s.errOut, "failed writing response to stdout: %v\n", writeErr)
					return writeErr // Broken pipe or write error
				}
			}
			if shouldExit {
				return nil
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				// Clean EOF shutdown
				s.engMu.Lock()
				if s.eng != nil {
					_ = s.eng.Close()
					s.eng = nil
				}
				s.engMu.Unlock()
				return nil
			}
			return fmt.Errorf("stdin read error: %w", err)
		}
	}
}

// dispatchSafe wraps dispatch with zero-panic recovery.
func (s *Server) dispatchSafe(line string) (resp string, shouldExit bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(s.errOut, "PANIC recovered in serve: %v\nstack:\n%s\n", r, debug.Stack())
			resp = fmt.Sprintf("ERR - %s internal panic: %v\n", ErrCodeInternalError, r)
			shouldExit = false
		}
	}()
	return s.dispatch(line)
}

// dispatch processes a single request line and returns the wire response string.
func (s *Server) dispatch(line string) (string, bool) {
	req, err := ParseRequest(line)
	if err != nil {
		var opErr *OpError
		if errors.As(err, &opErr) {
			return fmt.Sprintf("ERR %s %s %s\n", opErr.OpID, opErr.Code, opErr.Message), false
		}
		parts := strings.Split(strings.TrimSpace(line), " ")
		opID := "-"
		if len(parts) >= 2 && isValidOpID(parts[1]) {
			opID = parts[1]
		}
		code := ErrCodeInvalidRequest
		if strings.Contains(err.Error(), "exceeds max") {
			code = ErrCodePayloadTooLarge
		}
		return fmt.Sprintf("ERR %s %s %s\n", opID, code, err.Error()), false
	}

	s.engMu.RLock()
	eng := s.eng
	s.engMu.RUnlock()

	switch req.Kind {
	case CmdPing:
		return fmt.Sprintf("PONG %s\n", req.OpID), false

	case CmdPut:
		if eng == nil {
			return fmt.Sprintf("ERR %s %s database not opened\n", req.OpID, ErrCodeInvalidRequest), false
		}
		if err := eng.Put(req.Key, req.Value); err != nil {
			return formatErrorResponse(req.OpID, err), false
		}
		return fmt.Sprintf("ACK %s\n", req.OpID), false

	case CmdGet:
		if eng == nil {
			return fmt.Sprintf("ERR %s %s database not opened\n", req.OpID, ErrCodeInvalidRequest), false
		}
		val, found, err := eng.Get(req.Key)
		if err != nil {
			return formatErrorResponse(req.OpID, err), false
		}
		if !found {
			return fmt.Sprintf("NOTFOUND %s\n", req.OpID), false
		}
		return fmt.Sprintf("VALUE %s %s\n", req.OpID, encodeHex(val)), false

	case CmdDel:
		if eng == nil {
			return fmt.Sprintf("ERR %s %s database not opened\n", req.OpID, ErrCodeInvalidRequest), false
		}
		if err := eng.Delete(req.Key); err != nil {
			return formatErrorResponse(req.OpID, err), false
		}
		return fmt.Sprintf("ACK %s\n", req.OpID), false

	case CmdBatch:
		if eng == nil {
			return fmt.Sprintf("ERR %s %s database not opened\n", req.OpID, ErrCodeInvalidRequest), false
		}
		if err := eng.Batch(req.BatchOps); err != nil {
			return formatErrorResponse(req.OpID, err), false
		}
		return fmt.Sprintf("ACK %s\n", req.OpID), false

	case CmdCheck:
		if eng == nil {
			return fmt.Sprintf("ERR %s %s database not opened\n", req.OpID, ErrCodeInvalidRequest), false
		}
		if err := eng.Check(); err != nil {
			return fmt.Sprintf("ERR %s CORRUPT %v\n", req.OpID, err), false
		}
		return fmt.Sprintf("OK %s\n", req.OpID), false

	case CmdDump:
		if eng == nil {
			return fmt.Sprintf("ERR %s %s database not opened\n", req.OpID, ErrCodeInvalidRequest), false
		}
		pairs, err := eng.Dump()
		if err != nil {
			return formatErrorResponse(req.OpID, err), false
		}
		return fmt.Sprintf("ACK %s %d\n", req.OpID, len(pairs)), false

	case CmdOpen:
		s.engMu.Lock()
		defer s.engMu.Unlock()
		if s.eng != nil {
			_ = s.eng.Close()
			s.eng = nil
		}
		newEng, err := engine.Open(req.Path, engine.Options{MaxBytes: req.MaxBytes})
		if err != nil {
			return formatErrorResponse(req.OpID, err), false
		}
		s.eng = newEng
		return fmt.Sprintf("OK %s\n", req.OpID), false

	case CmdClose:
		s.engMu.Lock()
		defer s.engMu.Unlock()
		if s.eng != nil {
			_ = s.eng.Close()
			s.eng = nil
		}
		return fmt.Sprintf("OK %s\n", req.OpID), true

	default:
		return fmt.Sprintf("ERR %s %s unknown command\n", req.OpID, ErrCodeInvalidRequest), false
	}
}

func formatErrorResponse(opID string, err error) string {
	code := ErrCodeInternalError
	if errors.Is(err, engine.ErrDiskFull) {
		code = ErrCodeQuotaExceeded
	} else if errors.Is(err, engine.ErrLocked) {
		code = ErrCodeLocked
	} else if errors.Is(err, format.ErrValueTooLarge) || errors.Is(err, format.ErrBatchTooLarge) {
		code = ErrCodePayloadTooLarge
	} else if errors.Is(err, format.ErrKeyEmpty) || errors.Is(err, format.ErrKeyTooLarge) || errors.Is(err, format.ErrBatchEmpty) {
		code = ErrCodeInvalidRequest
	} else if strings.Contains(err.Error(), "CORRUPT") {
		code = "CORRUPT"
	} else if strings.Contains(err.Error(), "IO_") {
		code = ErrCodeIOError
	}
	return fmt.Sprintf("ERR %s %s %v\n", opID, code, err)
}
