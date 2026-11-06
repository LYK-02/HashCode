package format

import (
	"encoding/binary"
	"math"
)

type OpType uint8

const (
	OpPut OpType = 0x01
	OpDel OpType = 0x02

	OpTypePut OpType = OpPut
	OpTypeDel OpType = OpDel

	RecordHeaderSize = 7 // 1 (OpType) + 2 (KeyLen) + 4 (ValueLen)
	OpHeaderSize     = RecordHeaderSize
	MinKeyLen        = 1
	MaxKeyLen        = 1024
	MinValueLen      = 0
	MaxValLen        = 1048576 // 1 MiB (1 << 20)
	MaxValueLen      = MaxValLen
	MaxBatchOps      = 10000
	MaxPayloadLen    = 64 * 1024 * 1024 // 64 MiB guard limit

	MinRecordSize = RecordHeaderSize + MinKeyLen + MinValueLen // 8 bytes
	MaxRecordSize = RecordHeaderSize + MaxKeyLen + MaxValueLen // 1,049,607 bytes
)

// Operation represents a single key-value mutation (PUT or DEL).
type Operation struct {
	Type  OpType
	Key   []byte
	Value []byte
}

// DecodedRecord preserves the decoded operation along with byte offset locators for KeyDir indexing.
type DecodedRecord struct {
	Type          OpType
	Key           []byte
	Value         []byte
	PayloadOffset uint32 // Offset relative to the start of payload
	KeyLen        uint16
	ValueLen      uint32
}

// ValidateOperation validates key length, value length, and operation type invariants.
func ValidateOperation(op Operation) error {
	if op.Type != OpPut && op.Type != OpDel {
		return ErrUnknownOpType
	}
	if len(op.Key) < MinKeyLen {
		return ErrKeyEmpty
	}
	if len(op.Key) > MaxKeyLen {
		return ErrKeyTooLarge
	}
	if op.Type == OpDel {
		if len(op.Value) != 0 {
			return ErrDelNonZeroValue
		}
	} else {
		if len(op.Value) > MaxValLen {
			return ErrValueTooLarge
		}
	}
	return nil
}

// RecordSize calculates the exact encoded size of an operation subrecord.
func RecordSize(op Operation) int {
	return RecordHeaderSize + len(op.Key) + len(op.Value)
}

// CalculatePayloadLength computes the total byte length of all operation subrecords in a batch.
func CalculatePayloadLength(ops []Operation) (uint64, error) {
	if len(ops) < 1 {
		return 0, ErrBatchEmpty
	}
	if len(ops) > MaxBatchOps {
		return 0, ErrBatchTooLarge
	}
	var total uint64
	for i := range ops {
		if err := ValidateOperation(ops[i]); err != nil {
			return 0, err
		}
		subLen := uint64(RecordSize(ops[i]))
		if math.MaxUint64-total < subLen {
			return 0, ErrPayloadOverflow
		}
		total += subLen
	}
	if total > MaxPayloadLen {
		return 0, ErrPayloadTooLarge
	}
	return total, nil
}

// EncodeRecordTo encodes a single operation subrecord into buf and returns bytes written.
func EncodeRecordTo(buf []byte, op Operation) (int, error) {
	if err := ValidateOperation(op); err != nil {
		return 0, err
	}
	needed := RecordSize(op)
	if len(buf) < needed {
		return 0, ErrPayloadUnderflow
	}
	buf[0] = byte(op.Type)
	binary.LittleEndian.PutUint16(buf[1:3], uint16(len(op.Key)))
	binary.LittleEndian.PutUint32(buf[3:7], uint32(len(op.Value)))
	copy(buf[7:7+len(op.Key)], op.Key)
	if len(op.Value) > 0 {
		copy(buf[7+len(op.Key):needed], op.Value)
	}
	return needed, nil
}

// EncodePayload encodes a slice of operations sequentially into dst.
func EncodePayload(ops []Operation, dst []byte) (int, error) {
	offset := 0
	for i := range ops {
		written, err := EncodeRecordTo(dst[offset:], ops[i])
		if err != nil {
			return 0, err
		}
		offset += written
	}
	return offset, nil
}

// DecodePayloadRecordsOnly parses all operation subrecords in payload and records their byte offsets.
func DecodePayloadRecordsOnly(payload []byte, opCount uint32) ([]DecodedRecord, error) {
	if opCount < 1 {
		return nil, ErrBatchEmpty
	}
	if opCount > MaxBatchOps {
		return nil, ErrBatchTooLarge
	}

	minPayload := uint64(opCount) * uint64(MinRecordSize)
	if uint64(len(payload)) < minPayload {
		return nil, ErrPayloadUnderflow
	}

	records := make([]DecodedRecord, 0, opCount)
	cursor := 0
	payloadLen := len(payload)

	for i := uint32(0); i < opCount; i++ {
		if cursor+RecordHeaderSize > payloadLen {
			return nil, ErrPayloadUnderflow
		}
		opType := OpType(payload[cursor])
		keyLen := binary.LittleEndian.Uint16(payload[cursor+1 : cursor+3])
		valLen := binary.LittleEndian.Uint32(payload[cursor+3 : cursor+7])

		if opType != OpPut && opType != OpDel {
			return nil, ErrUnknownOpType
		}
		if keyLen < MinKeyLen {
			return nil, ErrKeyEmpty
		}
		if keyLen > MaxKeyLen {
			return nil, ErrKeyTooLarge
		}
		if opType == OpDel {
			if valLen != 0 {
				return nil, ErrDelNonZeroValue
			}
		} else {
			if valLen > MaxValLen {
				return nil, ErrValueTooLarge
			}
		}

		subrecordLen := RecordHeaderSize + int(keyLen) + int(valLen)
		if cursor+subrecordLen > payloadLen {
			return nil, ErrPayloadUnderflow
		}

		keyStart := cursor + RecordHeaderSize
		keyEnd := keyStart + int(keyLen)
		valStart := keyEnd
		valEnd := valStart + int(valLen)

		// Make defensive copies of key and value slices
		keyCopy := make([]byte, keyLen)
		copy(keyCopy, payload[keyStart:keyEnd])

		var valCopy []byte
		if valLen > 0 {
			valCopy = make([]byte, valLen)
			copy(valCopy, payload[valStart:valEnd])
		} else {
			valCopy = []byte{}
		}

		records = append(records, DecodedRecord{
			Type:          opType,
			Key:           keyCopy,
			Value:         valCopy,
			PayloadOffset: uint32(cursor),
			KeyLen:        keyLen,
			ValueLen:      valLen,
		})

		cursor += subrecordLen
	}

	if cursor != payloadLen {
		return nil, ErrTrailingPayloadBytes
	}

	return records, nil
}

// DecodePayload parses payload bytes into a slice of Operations.
func DecodePayload(payload []byte, opCount uint32) ([]Operation, error) {
	records, err := DecodePayloadRecordsOnly(payload, opCount)
	if err != nil {
		return nil, err
	}
	ops := make([]Operation, len(records))
	for i, r := range records {
		ops[i] = Operation{
			Type:  r.Type,
			Key:   r.Key,
			Value: r.Value,
		}
	}
	return ops, nil
}

// ValueFileOffset calculates the absolute file offset for KeyDir indexing.
func ValueFileOffset(frameStart uint64, payloadOffset uint32, keyLen uint16) uint64 {
	return frameStart + 28 + uint64(payloadOffset) + uint64(RecordHeaderSize) + uint64(keyLen)
}
