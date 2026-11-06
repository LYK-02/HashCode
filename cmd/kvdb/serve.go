package main

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"kvdb/internal/engine"
	"kvdb/internal/format"
)

// runServe executes the long-running interactive multiplexed server on stdin/stdout.
func runServe(dbPath string, maxBytes uint64) {
	eng, err := engine.Open(dbPath, engine.Options{MaxBytes: maxBytes})
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve open error: %v\n", err)
		os.Exit(1)
	}
	defer eng.Close()

	scanner := bufio.NewScanner(os.Stdin)
	// Allocate a 4MB buffer to accommodate batches with up to 10,000 operations
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 32*1024*1024)

	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, " ", 4)
		cmd := strings.ToUpper(parts[0])

		switch cmd {
		case "PING":
			opID := "0"
			if len(parts) >= 2 {
				opID = parts[1]
			}
			fmt.Fprintf(writer, "PONG %s\n", opID)
			writer.Flush()

		case "PUT":
			if len(parts) < 4 {
				opID := "0"
				if len(parts) >= 2 {
					opID = parts[1]
				}
				fmt.Fprintf(writer, "ERR %s INVALID_REQUEST invalid PUT arguments\n", opID)
				writer.Flush()
				continue
			}
			opID := parts[1]
			keyHex := parts[2]
			valHex := parts[3]

			keyBytes, err := hex.DecodeString(keyHex)
			if err != nil {
				fmt.Fprintf(writer, "ERR %s INVALID_REQUEST invalid hex in key\n", opID)
				writer.Flush()
				continue
			}
			valBytes, err := hex.DecodeString(valHex)
			if err != nil {
				fmt.Fprintf(writer, "ERR %s INVALID_REQUEST invalid hex in val\n", opID)
				writer.Flush()
				continue
			}

			if err := eng.Put(keyBytes, valBytes); err != nil {
				emitError(writer, opID, err)
			} else {
				fmt.Fprintf(writer, "ACK %s\n", opID)
			}
			writer.Flush()

		case "GET":
			if len(parts) < 3 {
				opID := "0"
				if len(parts) >= 2 {
					opID = parts[1]
				}
				fmt.Fprintf(writer, "ERR %s INVALID_REQUEST invalid GET arguments\n", opID)
				writer.Flush()
				continue
			}
			opID := parts[1]
			keyHex := parts[2]
			keyBytes, err := hex.DecodeString(keyHex)
			if err != nil {
				fmt.Fprintf(writer, "ERR %s INVALID_REQUEST invalid hex in key\n", opID)
				writer.Flush()
				continue
			}

			val, found, err := eng.Get(keyBytes)
			if err != nil {
				emitError(writer, opID, err)
			} else if !found {
				fmt.Fprintf(writer, "VALUE %s NOTFOUND\n", opID)
			} else {
				fmt.Fprintf(writer, "VALUE %s %s\n", opID, hex.EncodeToString(val))
			}
			writer.Flush()

		case "DEL":
			if len(parts) < 3 {
				opID := "0"
				if len(parts) >= 2 {
					opID = parts[1]
				}
				fmt.Fprintf(writer, "ERR %s INVALID_REQUEST invalid DEL arguments\n", opID)
				writer.Flush()
				continue
			}
			opID := parts[1]
			keyHex := parts[2]
			keyBytes, err := hex.DecodeString(keyHex)
			if err != nil {
				fmt.Fprintf(writer, "ERR %s INVALID_REQUEST invalid hex in key\n", opID)
				writer.Flush()
				continue
			}

			if err := eng.Delete(keyBytes); err != nil {
				emitError(writer, opID, err)
			} else {
				fmt.Fprintf(writer, "ACK %s\n", opID)
			}
			writer.Flush()

		case "BATCH":
			// Grammar: BATCH <op_id> <count> <encoded_ops>
			// where encoded_ops is comma-separated: <hex_k>:<hex_v> or <hex_k>: (for DEL)
			if len(parts) < 4 {
				opID := "0"
				if len(parts) >= 2 {
					opID = parts[1]
				}
				fmt.Fprintf(writer, "ERR %s INVALID_REQUEST invalid BATCH arguments\n", opID)
				writer.Flush()
				continue
			}
			opID := parts[1]
			count, err := strconv.Atoi(parts[2])
			if err != nil || count < 1 || count > format.MaxBatchOps {
				fmt.Fprintf(writer, "ERR %s INVALID_REQUEST invalid batch count\n", opID)
				writer.Flush()
				continue
			}

			encodedOps := parts[3]
			ops, err := parseServeBatch(encodedOps, count)
			if err != nil {
				fmt.Fprintf(writer, "ERR %s INVALID_REQUEST %v\n", opID, err)
				writer.Flush()
				continue
			}

			if err := eng.Batch(ops); err != nil {
				emitError(writer, opID, err)
			} else {
				fmt.Fprintf(writer, "ACK %s\n", opID)
			}
			writer.Flush()

		case "COMPACT":
			opID := "0"
			if len(parts) >= 2 {
				opID = parts[1]
			}
			if err := eng.Compact(); err != nil {
				emitError(writer, opID, err)
			} else {
				fmt.Fprintf(writer, "ACK %s\n", opID)
			}
			writer.Flush()

		default:
			opID := "0"
			if len(parts) >= 2 {
				opID = parts[1]
			}
			fmt.Fprintf(writer, "ERR %s INVALID_REQUEST unknown command: %s\n", opID, cmd)
			writer.Flush()
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		fmt.Fprintf(os.Stderr, "serve scanner error: %v\n", err)
	}
}

func parseServeBatch(encoded string, expectedCount int) ([]format.Operation, error) {
	rawItems := strings.Split(encoded, ",")
	if len(rawItems) != expectedCount {
		return nil, fmt.Errorf("item count mismatch: expected %d, got %d", expectedCount, len(rawItems))
	}

	ops := make([]format.Operation, len(rawItems))
	for i, item := range rawItems {
		colonIdx := strings.Index(item, ":")
		if colonIdx < 0 {
			return nil, fmt.Errorf("item %d missing ':' delimiter", i)
		}
		kHex := item[:colonIdx]
		vHex := item[colonIdx+1:]

		kBytes, err := hex.DecodeString(kHex)
		if err != nil {
			return nil, fmt.Errorf("item %d invalid key hex: %w", i, err)
		}

		if vHex == "" {
			// Deletion
			ops[i] = format.Operation{
				Type:  format.OpDel,
				Key:   kBytes,
				Value: nil,
			}
		} else {
			// Put
			vBytes, err := hex.DecodeString(vHex)
			if err != nil {
				return nil, fmt.Errorf("item %d invalid val hex: %w", i, err)
			}
			ops[i] = format.Operation{
				Type:  format.OpPut,
				Key:   kBytes,
				Value: vBytes,
			}
		}
	}
	return ops, nil
}

func emitError(w *bufio.Writer, opID string, err error) {
	code := "INTERNAL_ERROR"
	switch {
	case errors.Is(err, engine.ErrDiskFull):
		code = "QUOTA_EXCEEDED"
	case errors.Is(err, format.ErrValueTooLarge), errors.Is(err, format.ErrPayloadTooLarge), errors.Is(err, format.ErrBatchTooLarge):
		code = "PAYLOAD_TOO_LARGE"
	case errors.Is(err, format.ErrKeyEmpty), errors.Is(err, format.ErrKeyTooLarge), errors.Is(err, format.ErrBatchEmpty), errors.Is(err, format.ErrUnknownOpType), errors.Is(err, format.ErrDelNonZeroValue):
		code = "INVALID_REQUEST"
	case errors.Is(err, engine.ErrLocked):
		code = "LOCKED"
	}
	fmt.Fprintf(w, "ERR %s %s %v\n", opID, code, err)
}
