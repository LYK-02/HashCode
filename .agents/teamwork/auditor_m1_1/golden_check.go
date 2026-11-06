package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"kvdb/internal/format"
	"kvdb/internal/testutil"
)

func main() {
	// Reconstruct PUT "hello" -> "world", seq 1
	f1, err := format.EncodeFrame(1, []format.Operation{
		{Type: format.OpPut, Key: []byte("hello"), Value: []byte("world")},
	})
	if err != nil || hex.EncodeToString(f1) != testutil.GoldenPutFrameHex {
		fmt.Fprintf(os.Stderr, "GoldenPutFrameHex mismatch: %s vs %s\n", hex.EncodeToString(f1), testutil.GoldenPutFrameHex)
		os.Exit(1)
	}

	// Reconstruct PUT "foo" -> "bar", seq 1
	f2, err := format.EncodeFrame(1, []format.Operation{
		{Type: format.OpPut, Key: []byte("foo"), Value: []byte("bar")},
	})
	if err != nil || hex.EncodeToString(f2) != testutil.GoldenPutFooBarFrameHex {
		fmt.Fprintf(os.Stderr, "GoldenPutFooBarFrameHex mismatch: %s vs %s\n", hex.EncodeToString(f2), testutil.GoldenPutFooBarFrameHex)
		os.Exit(1)
	}

	// Reconstruct PUT "foo" -> "", seq 2
	f3, err := format.EncodeFrame(2, []format.Operation{
		{Type: format.OpPut, Key: []byte("foo"), Value: []byte{}},
	})
	if err != nil || hex.EncodeToString(f3) != testutil.GoldenEmptyValPutFrameHex {
		fmt.Fprintf(os.Stderr, "GoldenEmptyValPutFrameHex mismatch: %s vs %s\n", hex.EncodeToString(f3), testutil.GoldenEmptyValPutFrameHex)
		os.Exit(1)
	}

	// Reconstruct DEL "foo", seq 3
	f4, err := format.EncodeFrame(3, []format.Operation{
		{Type: format.OpDel, Key: []byte("foo"), Value: nil},
	})
	if err != nil || hex.EncodeToString(f4) != testutil.GoldenDelFrameHex {
		fmt.Fprintf(os.Stderr, "GoldenDelFrameHex mismatch: %s vs %s\n", hex.EncodeToString(f4), testutil.GoldenDelFrameHex)
		os.Exit(1)
	}

	// Reconstruct Batch 3 ops, seq 42
	f5, err := format.EncodeFrame(42, []format.Operation{
		{Type: format.OpPut, Key: []byte("k1"), Value: []byte("v1")},
		{Type: format.OpDel, Key: []byte("k2"), Value: nil},
		{Type: format.OpPut, Key: []byte("k3"), Value: []byte{}},
	})
	if err != nil || hex.EncodeToString(f5) != testutil.GoldenBatchFrameHex {
		fmt.Fprintf(os.Stderr, "GoldenBatchFrameHex mismatch: %s vs %s\n", hex.EncodeToString(f5), testutil.GoldenBatchFrameHex)
		os.Exit(1)
	}

	fmt.Println("All 5 golden frames match independent bit-for-bit generation!")
}
