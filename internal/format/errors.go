package format

import "errors"

var (
	// Database Header Errors
	ErrHeaderTooShort         = errors.New("CORRUPT_HEADER: db header too short (< 32 bytes)")
	ErrShortHeader            = ErrHeaderTooShort
	ErrInvalidHeaderMagic     = errors.New("CORRUPT_HEADER: invalid db header magic (expected 'KVDB2026')")
	ErrUnsupportedVersion     = errors.New("CORRUPT_HEADER: unsupported database version (expected 1)")
	ErrReservedNonZero        = errors.New("CORRUPT_HEADER: db header reserved field must be zero")
	ErrCorruptHeader          = errors.New("CORRUPT_HEADER: db header CRC32C checksum mismatch")
	ErrHeaderChecksumMismatch = ErrCorruptHeader

	// UABE Frame Header Errors
	ErrFrameHeaderTooShort         = errors.New("CORRUPT_FRAME: frame header too short (< 28 bytes)")
	ErrShortFrameHeader            = ErrFrameHeaderTooShort
	ErrInvalidFrameMagic           = errors.New("CORRUPT_FRAME: invalid frame magic (expected 0x55414245)")
	ErrCorruptFrameHeader          = errors.New("CORRUPT_FRAME: frame header CRC32C checksum mismatch")
	ErrFrameHeaderChecksumMismatch = ErrCorruptFrameHeader
	ErrInvalidOpCount              = errors.New("INVALID_ARGUMENT: operation count must be between 1 and 10000")
	ErrBatchEmpty                  = errors.New("INVALID_ARGUMENT: batch operation count cannot be zero")
	ErrBatchTooLarge               = errors.New("INVALID_ARGUMENT: batch operation count exceeds 10000")
	ErrPayloadLengthOutOfBounds    = errors.New("INVALID_ARGUMENT: payload length out of bounds for op count")
	ErrPayloadTooLarge             = errors.New("INVALID_ARGUMENT: payload length exceeds maximum allowed bounds (64 MiB)")

	// Trailer & Payload Errors
	ErrTrailerTooShort         = errors.New("CORRUPT_FRAME: trailer too short (< 4 bytes)")
	ErrShortTrailer            = ErrTrailerTooShort
	ErrCorruptPayload          = errors.New("CORRUPT_FRAME: payload trailer CRC32C checksum mismatch")
	ErrPayloadChecksumMismatch = ErrCorruptPayload
	ErrCorruptFrame            = errors.New("CORRUPT_FRAME: frame envelope or data corrupt")
	ErrPayloadUnderflow        = errors.New("CORRUPT_FRAME: payload truncated before complete subrecord")
	ErrTruncatedFrame          = ErrPayloadUnderflow
	ErrSubrecordTruncated      = ErrPayloadUnderflow
	ErrShortPayload            = ErrPayloadUnderflow
	ErrTrailingPayloadBytes    = errors.New("CORRUPT_FRAME: trailing unparsed garbage bytes in payload")
	ErrTrailingPayloadGarbage  = ErrTrailingPayloadBytes
	ErrTrailingBytes           = ErrTrailingPayloadBytes

	// Operation Subrecord Errors
	ErrUnknownOpType   = errors.New("INVALID_ARGUMENT: unknown operation type (expected PUT=0x01 or DEL=0x02)")
	ErrInvalidOpType   = ErrUnknownOpType
	ErrKeyEmpty        = errors.New("INVALID_ARGUMENT: key length must be at least 1 byte")
	ErrKeyLengthZero   = ErrKeyEmpty
	ErrKeyTooLarge     = errors.New("INVALID_ARGUMENT: key length exceeds 1024 bytes")
	ErrValueTooLarge   = errors.New("INVALID_ARGUMENT: value length exceeds 1048576 bytes (1 MiB)")
	ErrDelNonZeroValue = errors.New("INVALID_ARGUMENT: DEL operation requires value length zero")
	ErrInvalidDelete   = ErrDelNonZeroValue
	ErrPayloadOverflow = errors.New("INVALID_ARGUMENT: calculated payload length exceeds maximum bounds")
)
