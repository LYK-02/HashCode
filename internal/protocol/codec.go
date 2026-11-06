package protocol

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"kvdb/internal/format"
)

// ParseRequest parses a single wire line into a typed Request.
func ParseRequest(line string) (*Request, error) {
	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 {
		return nil, &OpError{OpID: "-", Code: ErrCodeInvalidRequest, Message: "empty request line"}
	}
	if len(line) > MaxLineLength {
		return nil, &OpError{OpID: "-", Code: ErrCodePayloadTooLarge, Message: fmt.Sprintf("line exceeds maximum length of %d bytes", MaxLineLength)}
	}

	parts := strings.Split(line, " ")
	if len(parts) < 1 || len(parts[0]) == 0 {
		return nil, &OpError{OpID: "-", Code: ErrCodeInvalidRequest, Message: "missing command token"}
	}

	cmd := CommandKind(strings.ToUpper(parts[0]))
	switch cmd {
	case CmdPing:
		if len(parts) != 2 {
			opID := "-"
			if len(parts) >= 2 && isValidOpID(parts[1]) {
				opID = parts[1]
			}
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: "PING expects exactly 1 argument (<op_id>)"}
		}
		opID := parts[1]
		if !isValidOpID(opID) {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("invalid op_id format: %q", opID)}
		}
		return &Request{Kind: CmdPing, OpID: opID}, nil

	case CmdPut:
		// Syntax: PUT <op_id> <hex_key> [hex_val]
		if len(parts) < 3 {
			opID := "-"
			if len(parts) == 2 && isValidOpID(parts[1]) {
				opID = parts[1]
			}
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: "PUT expects at least <op_id> and <hex_key>"}
		}
		opID := parts[1]
		if !isValidOpID(opID) {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("invalid op_id format: %q", opID)}
		}
		k, err := decodeHex(parts[2])
		if err != nil {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("invalid hex key: %v", err)}
		}
		if len(k) < format.MinKeyLen {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: "key must not be empty"}
		}
		if len(k) > format.MaxKeyLen {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("key length %d exceeds max %d", len(k), format.MaxKeyLen)}
		}

		var v []byte
		if len(parts) >= 4 {
			if len(parts) > 4 {
				// Verify remaining tokens are just empty space
				for _, extra := range parts[4:] {
					if extra != "" {
						return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: "too many arguments for PUT"}
					}
				}
			}
			vHex := parts[3]
			v, err = decodeHex(vHex)
			if err != nil {
				return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("invalid hex value: %v", err)}
			}
		} else {
			v = []byte{}
		}
		if len(v) > MaxRawValueLen {
			return nil, &OpError{OpID: opID, Code: ErrCodePayloadTooLarge, Message: fmt.Sprintf("value length %d exceeds max %d", len(v), MaxRawValueLen)}
		}

		return &Request{Kind: CmdPut, OpID: opID, Key: k, Value: v}, nil

	case CmdGet:
		if len(parts) != 3 {
			opID := "-"
			if len(parts) >= 2 && isValidOpID(parts[1]) {
				opID = parts[1]
			}
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: "GET expects <op_id> <hex_key>"}
		}
		opID := parts[1]
		if !isValidOpID(opID) {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("invalid op_id format: %q", opID)}
		}
		k, err := decodeHex(parts[2])
		if err != nil {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("invalid hex key: %v", err)}
		}
		return &Request{Kind: CmdGet, OpID: opID, Key: k}, nil

	case CmdDel:
		if len(parts) != 3 {
			opID := "-"
			if len(parts) >= 2 && isValidOpID(parts[1]) {
				opID = parts[1]
			}
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: "DEL expects <op_id> <hex_key>"}
		}
		opID := parts[1]
		if !isValidOpID(opID) {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("invalid op_id format: %q", opID)}
		}
		k, err := decodeHex(parts[2])
		if err != nil {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("invalid hex key: %v", err)}
		}
		return &Request{Kind: CmdDel, OpID: opID, Key: k}, nil

	case CmdBatch:
		// Syntax: BATCH <op_id> <count> <k1:v1,k2:,k3:v3>
		if len(parts) != 4 {
			opID := "-"
			if len(parts) >= 2 && isValidOpID(parts[1]) {
				opID = parts[1]
			}
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: "BATCH expects 4 tokens (<op_id> <count> <items>)"}
		}
		opID := parts[1]
		if !isValidOpID(opID) {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("invalid op_id format: %q", opID)}
		}
		count, err := strconv.Atoi(parts[2])
		if err != nil || count < 1 {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("invalid batch count: %q", parts[2])}
		}
		if count > MaxBatchItems {
			return nil, &OpError{OpID: opID, Code: ErrCodePayloadTooLarge, Message: fmt.Sprintf("batch count %d exceeds max %d", count, MaxBatchItems)}
		}

		itemsStr := parts[3]
		if len(itemsStr) == 0 {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: "batch items string cannot be empty"}
		}
		rawItems := strings.Split(itemsStr, ",")
		if len(rawItems) != count {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("batch count mismatch: header specifies %d, found %d items", count, len(rawItems))}
		}

		ops := make([]format.Operation, 0, count)
		for _, itm := range rawItems {
			idx := strings.IndexByte(itm, ':')
			if idx == -1 {
				return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("malformed batch item (missing colon): %q", itm)}
			}
			kHex := itm[:idx]
			vHex := itm[idx+1:]
			k, err := decodeHex(kHex)
			if err != nil {
				return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("invalid batch item key hex: %v", err)}
			}
			if len(k) < format.MinKeyLen {
				return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: "batch item key cannot be empty"}
			}
			if len(k) > format.MaxKeyLen {
				return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("batch item key length %d exceeds max %d", len(k), format.MaxKeyLen)}
			}

			if len(vHex) == 0 {
				// Deletion
				ops = append(ops, format.Operation{Type: format.OpDel, Key: k})
			} else {
				// Put
				v, err := decodeHex(vHex)
				if err != nil {
					return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("invalid batch item value hex: %v", err)}
				}
				if len(v) > MaxRawValueLen {
					return nil, &OpError{OpID: opID, Code: ErrCodePayloadTooLarge, Message: fmt.Sprintf("batch item value exceeds max %d", MaxRawValueLen)}
				}
				ops = append(ops, format.Operation{Type: format.OpPut, Key: k, Value: v})
			}
		}
		return &Request{Kind: CmdBatch, OpID: opID, BatchOps: ops}, nil

	case CmdCheck:
		opID := "chk"
		if len(parts) >= 2 && isValidOpID(parts[1]) {
			opID = parts[1]
		}
		return &Request{Kind: CmdCheck, OpID: opID}, nil

	case CmdDump:
		opID := "dump"
		if len(parts) >= 2 && isValidOpID(parts[1]) {
			opID = parts[1]
		}
		return &Request{Kind: CmdDump, OpID: opID}, nil

	case CmdOpen:
		// Syntax: OPEN <op_id> <path> [--max-bytes N]
		opID := "open"
		pathIdx := 1
		if len(parts) >= 3 && isValidOpID(parts[1]) {
			opID = parts[1]
			pathIdx = 2
		}
		if pathIdx >= len(parts) {
			return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: "OPEN expects <path>"}
		}
		dbPath := parts[pathIdx]
		var mb uint64
		for i := pathIdx + 1; i < len(parts); i++ {
			if parts[i] == "--max-bytes" && i+1 < len(parts) {
				m, _ := strconv.ParseUint(parts[i+1], 10, 64)
				mb = m
				i++
			}
		}
		return &Request{Kind: CmdOpen, OpID: opID, Path: dbPath, MaxBytes: mb}, nil

	case CmdClose:
		opID := "close"
		if len(parts) >= 2 && isValidOpID(parts[1]) {
			opID = parts[1]
		}
		return &Request{Kind: CmdClose, OpID: opID}, nil

	default:
		opID := "-"
		if len(parts) >= 2 && isValidOpID(parts[1]) {
			opID = parts[1]
		}
		return nil, &OpError{OpID: opID, Code: ErrCodeInvalidRequest, Message: fmt.Sprintf("unknown command: %s", parts[0])}
	}
}

// Helper utilities for hex conversion
func decodeHex(s string) ([]byte, error) {
	if len(s) == 0 {
		return []byte{}, nil
	}
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd-length hex string (length %d)", len(s))
	}
	return hex.DecodeString(s)
}

func encodeHex(b []byte) string {
	return hex.EncodeToString(b)
}

func isValidOpID(id string) bool {
	return len(id) > 0 && opIDPattern.MatchString(id)
}
