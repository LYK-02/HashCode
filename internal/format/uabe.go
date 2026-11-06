package format

import (
	"encoding/binary"
	"math"
)

const (
	FrameMagic        = uint32(0x55414245) // "UABE" in little-endian
	FrameHeaderLen    = 28
	FrameHeaderSize   = FrameHeaderLen
	FrameTrailerLen   = 4
	FrameTrailerSize  = FrameTrailerLen
	FrameEnvelopeLen  = FrameHeaderLen + FrameTrailerLen // 32 bytes
	FrameEnvelopeSize = FrameEnvelopeLen
)

// FrameHeader holds decoded metadata of a 28-byte UABE frame header.
type FrameHeader struct {
	Magic          uint32
	HeaderCRC      uint32
	HeaderCRC32C   uint32 // Alias for HeaderCRC
	PayloadLen     uint64
	PayloadLength  uint64 // Alias for PayloadLen
	SeqNo          uint64
	SequenceNumber uint64 // Alias for SeqNo
	OpCount        uint32
}

// TotalFrameSize computes the total on-disk frame length (32 + payloadLen).
func TotalFrameSize(payloadLen uint64) uint64 {
	return uint64(FrameEnvelopeSize) + payloadLen
}

// EncodeFrame serializes operations into a full UABE frame (28B header, payload, 4B trailer).
func EncodeFrame(seqNo uint64, ops []Operation) ([]byte, error) {
	if len(ops) == 0 {
		return nil, ErrBatchEmpty
	}
	if len(ops) > MaxBatchOps {
		return nil, ErrBatchTooLarge
	}

	payloadLen, err := CalculatePayloadLength(ops)
	if err != nil {
		return nil, err
	}

	totalLen := uint64(FrameEnvelopeSize) + payloadLen
	if totalLen > uint64(math.MaxInt) {
		return nil, ErrPayloadOverflow
	}
	buf := make([]byte, totalLen)

	// 1. Frame Header fields (bytes 0..28)
	binary.LittleEndian.PutUint32(buf[0:4], FrameMagic)
	binary.LittleEndian.PutUint64(buf[8:16], payloadLen)
	binary.LittleEndian.PutUint64(buf[16:24], seqNo)
	binary.LittleEndian.PutUint32(buf[24:28], uint32(len(ops)))

	// Header CRC32C over bytes 8..28
	hdrCRC := ChecksumCastagnoli(buf[8:28])
	binary.LittleEndian.PutUint32(buf[4:8], hdrCRC)

	// 2. Payload Subrecords (bytes 28 .. 28+payloadLen)
	written, err := EncodePayload(ops, buf[FrameHeaderSize:FrameHeaderSize+payloadLen])
	if err != nil {
		return nil, err
	}
	if uint64(written) != payloadLen {
		return nil, ErrPayloadOverflow
	}

	// 3. Frame Trailer CRC32C (trailing 4 bytes over payload)
	trailerCRC := ChecksumCastagnoli(buf[FrameHeaderSize : FrameHeaderSize+payloadLen])
	binary.LittleEndian.PutUint32(buf[FrameHeaderSize+payloadLen:totalLen], trailerCRC)

	return buf, nil
}

// DecodeFrameHeader verifies magic, header CRC32C, and bounded op/payload limits before payload allocation.
func DecodeFrameHeader(b []byte) (*FrameHeader, error) {
	if len(b) < FrameHeaderSize {
		return nil, ErrFrameHeaderTooShort
	}
	magic := binary.LittleEndian.Uint32(b[0:4])
	if magic != FrameMagic {
		return nil, ErrInvalidFrameMagic
	}
	storedHeaderCRC := binary.LittleEndian.Uint32(b[4:8])
	computedHeaderCRC := ChecksumCastagnoli(b[8:28])
	if storedHeaderCRC != computedHeaderCRC {
		return nil, ErrCorruptFrameHeader
	}

	payloadLen := binary.LittleEndian.Uint64(b[8:16])
	seqNo := binary.LittleEndian.Uint64(b[16:24])
	opCount := binary.LittleEndian.Uint32(b[24:28])

	if opCount < 1 {
		return nil, ErrBatchEmpty
	}
	if opCount > MaxBatchOps {
		return nil, ErrBatchTooLarge
	}

	minPayload := uint64(opCount) * uint64(MinRecordSize)
	if payloadLen < minPayload {
		return nil, ErrPayloadLengthOutOfBounds
	}
	if payloadLen > MaxPayloadLen {
		return nil, ErrPayloadTooLarge
	}
	maxPayloadForOps := uint64(opCount) * uint64(MaxRecordSize)
	if payloadLen > maxPayloadForOps {
		return nil, ErrPayloadLengthOutOfBounds
	}

	return &FrameHeader{
		Magic:          magic,
		HeaderCRC:      storedHeaderCRC,
		HeaderCRC32C:   storedHeaderCRC,
		PayloadLen:     payloadLen,
		PayloadLength:  payloadLen,
		SeqNo:          seqNo,
		SequenceNumber: seqNo,
		OpCount:        opCount,
	}, nil
}

// DecodePayloadRecords verifies trailer CRC32C over payload and returns parsed records with byte locators.
func DecodePayloadRecords(payload []byte, trailer []byte, opCount uint32) ([]DecodedRecord, error) {
	if len(trailer) < FrameTrailerSize {
		return nil, ErrTrailerTooShort
	}
	storedTrailerCRC := binary.LittleEndian.Uint32(trailer[0:4])
	computedTrailerCRC := ChecksumCastagnoli(payload)
	if storedTrailerCRC != computedTrailerCRC {
		return nil, ErrCorruptPayload
	}

	return DecodePayloadRecordsOnly(payload, opCount)
}

// DecodePayloadAndTrailer verifies trailer CRC32C over payload and parses operation subrecords.
func DecodePayloadAndTrailer(payload []byte, trailer []byte, opCount uint32) ([]Operation, error) {
	records, err := DecodePayloadRecords(payload, trailer, opCount)
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

// DecodeFullFrame decodes a contiguous slice containing frame header, payload, and trailer.
func DecodeFullFrame(b []byte) (*FrameHeader, []Operation, error) {
	fh, err := DecodeFrameHeader(b)
	if err != nil {
		return nil, nil, err
	}
	totalLen := FrameHeaderSize + fh.PayloadLen + FrameTrailerSize
	if uint64(len(b)) < totalLen {
		return nil, nil, ErrTruncatedFrame
	}

	payload := b[FrameHeaderSize : FrameHeaderSize+fh.PayloadLen]
	trailer := b[FrameHeaderSize+fh.PayloadLen : totalLen]

	ops, err := DecodePayloadAndTrailer(payload, trailer, fh.OpCount)
	if err != nil {
		return nil, nil, err
	}
	return fh, ops, nil
}
