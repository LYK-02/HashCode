package protocol

import (
	"fmt"
	"regexp"

	"kvdb/internal/format"
)

// Standard wire limits
const (
	MaxRawValueLen = 1024 * 1024        // 1 MiB raw value
	MaxHexValueLen = MaxRawValueLen * 2 // 2 MiB in hex characters
	MaxLineLength  = 64 * 1024 * 1024   // 64 MiB framing headroom for 10k batches
	MaxBatchItems  = 10000              // Maximum operations in a batch
)

// Standardized error codes matching Python torture harness
const (
	ErrCodeQuotaExceeded   = "QUOTA_EXCEEDED"
	ErrCodeInvalidRequest  = "INVALID_REQUEST"
	ErrCodePayloadTooLarge = "PAYLOAD_TOO_LARGE"
	ErrCodeIOError         = "IO_ERROR"
	ErrCodeLocked          = "LOCKED"
	ErrCodeInternalError   = "INTERNAL_ERROR"
	ErrCodeNotFound        = "NOTFOUND"
)

var (
	opIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)
)

// CommandKind defines the command token type.
type CommandKind string

const (
	CmdPing  CommandKind = "PING"
	CmdPut   CommandKind = "PUT"
	CmdGet   CommandKind = "GET"
	CmdDel   CommandKind = "DEL"
	CmdBatch CommandKind = "BATCH"
	CmdCheck CommandKind = "CHECK"
	CmdDump  CommandKind = "DUMP"
	CmdOpen  CommandKind = "OPEN"
	CmdClose CommandKind = "CLOSE"
)

// Request encapsulates a parsed streaming protocol request.
type Request struct {
	Kind     CommandKind
	OpID     string
	Key      []byte
	Value    []byte
	BatchOps []format.Operation
	Path     string
	MaxBytes uint64
}

// OpError carries op_id and standardized code for formatted error response.
type OpError struct {
	OpID    string
	Code    string
	Message string
}

func (e *OpError) Error() string {
	return fmt.Sprintf("ERR %s %s %s", e.OpID, e.Code, e.Message)
}
