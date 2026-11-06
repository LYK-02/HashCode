package testutil

import (
	"encoding/hex"
)

const (
	// Golden 32-byte DB Header (KVDB2026, Version 1, Gen 0, Reserved 0, CRC32C 0x6708c580)
	GoldenHeaderHex = "4b56444232303236010000000000000000000000000000000000000080c50867"

	// Golden 32-byte Compacted DB Header (KVDB2026, Version 1, Gen 5, Reserved 0, CRC32C 0x53006495)
	GoldenCompactedHeaderHex = "4b56444232303236010000000500000000000000000000000000000095640053"

	// Golden 49-byte PUT Frame (Seq 1, OpCount 1, PUT "hello" -> "world", HdrCRC 0x1db93fc8, TrailerCRC 0x05e1ece7)
	GoldenPutFrameHex = "45424155c83fb91d11000000000000000100000000000000010000000105000500000068656c6c6f776f726c64e7ece105"

	// Golden 45-byte PUT Frame (Seq 1, OpCount 1, PUT "foo" -> "bar", HdrCRC 0x054ecae6, TrailerCRC 0xdd783b3a)
	GoldenPutFooBarFrameHex = "45424155e6ca4e050d0000000000000001000000000000000100000001030003000000666f6f6261723a3b78dd"

	// Golden 42-byte PUT Frame with Empty Value (Seq 2, OpCount 1, PUT "foo" -> "", HdrCRC 0xd2177085, TrailerCRC 0x9e848a80)
	GoldenEmptyValPutFrameHex = "45424155857017d20a0000000000000002000000000000000100000001030000000000666f6f808a849e"

	// Golden 42-byte DEL Frame (Seq 3, OpCount 1, DEL "foo", HdrCRC 0xe366a4b5, TrailerCRC 0xc692709e)
	GoldenDelFrameHex = "45424155b5a466e30a0000000000000003000000000000000100000002030000000000666f6f9e7092c6"

	// Golden 61-byte 3-Op Batch Frame (Seq 42, OpCount 3, PUT k1->v1, DEL k2, PUT k3->"", HdrCRC 0x2f8112a3, TrailerCRC 0x9fb329b2)
	GoldenBatchFrameHex = "45424155a312812f1d000000000000002a0000000000000003000000010200020000006b317631020200000000006b32010200000000006b33b229b39f"
)

// MustDecodeHex decodes a hex string into bytes and panics if invalid (for test initialization).
func MustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}
