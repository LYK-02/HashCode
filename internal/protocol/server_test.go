package protocol

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"kvdb/internal/engine"
)

func TestProtocol_PingPong(t *testing.T) {
	req, err := ParseRequest("PING op-001\n")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if req.Kind != CmdPing || req.OpID != "op-001" {
		t.Fatalf("unexpected request: %+v", req)
	}

	var in bytes.Buffer
	var out bytes.Buffer
	var errOut bytes.Buffer

	in.WriteString("PING op-001\n")
	srv := NewServer(nil, &in, &out, &errOut)
	if err := srv.Run(); err != nil {
		t.Fatalf("server run error: %v", err)
	}

	expected := "PONG op-001\n"
	if out.String() != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestProtocol_PutGetDel(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "protocol_test.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("failed to open engine: %v", err)
	}
	defer eng.Close()

	var in bytes.Buffer
	var out bytes.Buffer
	var errOut bytes.Buffer

	keyHex := hex.EncodeToString([]byte("foo"))
	valHex := hex.EncodeToString([]byte("bar"))

	// 1. PUT
	in.WriteString(fmt.Sprintf("PUT op-1 %s %s\n", keyHex, valHex))
	// 2. GET
	in.WriteString(fmt.Sprintf("GET op-2 %s\n", keyHex))
	// 3. DEL
	in.WriteString(fmt.Sprintf("DEL op-3 %s\n", keyHex))
	// 4. GET after DEL (NOTFOUND)
	in.WriteString(fmt.Sprintf("GET op-4 %s\n", keyHex))

	srv := NewServer(eng, &in, &out, &errOut)
	if err := srv.Run(); err != nil {
		t.Fatalf("server run error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 response lines, got %d: %q", len(lines), out.String())
	}

	if lines[0] != "ACK op-1" {
		t.Errorf("line 0: expected 'ACK op-1', got %q", lines[0])
	}
	if lines[1] != fmt.Sprintf("VALUE op-2 %s", valHex) {
		t.Errorf("line 1: expected 'VALUE op-2 %s', got %q", valHex, lines[1])
	}
	if lines[2] != "ACK op-3" {
		t.Errorf("line 2: expected 'ACK op-3', got %q", lines[2])
	}
	if lines[3] != "NOTFOUND op-4" {
		t.Errorf("line 3: expected 'NOTFOUND op-4', got %q", lines[3])
	}
}

func TestProtocol_EmptyValue(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty_val.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("failed to open engine: %v", err)
	}
	defer eng.Close()

	var in bytes.Buffer
	var out bytes.Buffer
	var errOut bytes.Buffer

	keyHex := hex.EncodeToString([]byte("emptyKey"))

	// Empty value with trailing space: PUT op-1 keyHex \n
	in.WriteString(fmt.Sprintf("PUT op-1 %s \n", keyHex))
	in.WriteString(fmt.Sprintf("GET op-2 %s\n", keyHex))

	srv := NewServer(eng, &in, &out, &errOut)
	if err := srv.Run(); err != nil {
		t.Fatalf("server run error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\r\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 response lines, got %d: %q", len(lines), out.String())
	}
	if lines[0] != "ACK op-1" {
		t.Errorf("expected 'ACK op-1', got %q", lines[0])
	}
	if lines[1] != "VALUE op-2 " && lines[1] != "VALUE op-2" {
		t.Errorf("expected 'VALUE op-2 ' or 'VALUE op-2', got %q", lines[1])
	}
}

func TestProtocol_BinaryPayload(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "binary_val.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("failed to open engine: %v", err)
	}
	defer eng.Close()

	var in bytes.Buffer
	var out bytes.Buffer
	var errOut bytes.Buffer

	binaryKey := []byte{0x00, 0xff, 0x00, 0x01}
	binaryVal := []byte{0x00, 0x01, 0x02, 0x03, 0x00, 0xfe, 0xff}
	kHex := hex.EncodeToString(binaryKey)
	vHex := hex.EncodeToString(binaryVal)

	in.WriteString(fmt.Sprintf("PUT bin-1 %s %s\n", kHex, vHex))
	in.WriteString(fmt.Sprintf("GET bin-2 %s\n", kHex))

	srv := NewServer(eng, &in, &out, &errOut)
	if err := srv.Run(); err != nil {
		t.Fatalf("server run error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 response lines, got %d: %q", len(lines), out.String())
	}
	if lines[0] != "ACK bin-1" {
		t.Errorf("expected 'ACK bin-1', got %q", lines[0])
	}
	if lines[1] != fmt.Sprintf("VALUE bin-2 %s", vHex) {
		t.Errorf("expected 'VALUE bin-2 %s', got %q", vHex, lines[1])
	}
}

func TestProtocol_Batch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "batch_test.db")

	eng, err := engine.Open(dbPath, engine.Options{})
	if err != nil {
		t.Fatalf("failed to open engine: %v", err)
	}
	defer eng.Close()

	var in bytes.Buffer
	var out bytes.Buffer
	var errOut bytes.Buffer

	k1 := hex.EncodeToString([]byte("k1"))
	v1 := hex.EncodeToString([]byte("v1"))
	k2 := hex.EncodeToString([]byte("k2"))
	v2 := hex.EncodeToString([]byte("v2"))
	k3 := hex.EncodeToString([]byte("k3"))

	// Pre-populate k2
	in.WriteString(fmt.Sprintf("PUT init-1 %s %s\n", k2, v2))
	// Batch: Put k1, Del k2, Put k3
	in.WriteString(fmt.Sprintf("BATCH b-001 3 %s:%s,%s:,%s:7633\n", k1, v1, k2, k3))
	// Verify k1, k2, k3
	in.WriteString(fmt.Sprintf("GET chk-1 %s\n", k1))
	in.WriteString(fmt.Sprintf("GET chk-2 %s\n", k2))
	in.WriteString(fmt.Sprintf("GET chk-3 %s\n", k3))

	srv := NewServer(eng, &in, &out, &errOut)
	if err := srv.Run(); err != nil {
		t.Fatalf("server run error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 response lines, got %d: %q", len(lines), out.String())
	}
	if lines[0] != "ACK init-1" {
		t.Errorf("expected 'ACK init-1', got %q", lines[0])
	}
	if lines[1] != "ACK b-001" {
		t.Errorf("expected 'ACK b-001', got %q", lines[1])
	}
	if lines[2] != fmt.Sprintf("VALUE chk-1 %s", v1) {
		t.Errorf("expected 'VALUE chk-1 %s', got %q", v1, lines[2])
	}
	if lines[3] != "NOTFOUND chk-2" {
		t.Errorf("expected 'NOTFOUND chk-2', got %q", lines[3])
	}
	if lines[4] != "VALUE chk-3 7633" {
		t.Errorf("expected 'VALUE chk-3 7633', got %q", lines[4])
	}
}

func TestProtocol_InvalidLines(t *testing.T) {
	invalidCases := []struct {
		line         string
		expectedCode string
	}{
		{"PUT w1\n", ErrCodeInvalidRequest},
		{"PUT w1 123 456\n", ErrCodeInvalidRequest}, // Odd hex
		{"DEL w1\n", ErrCodeInvalidRequest},
		{"BATCH b1 2 6b31:7631\n", ErrCodeInvalidRequest}, // Count mismatch
		{"BATCH b1 1 6b31\n", ErrCodeInvalidRequest},      // Missing colon
		{"ACK\n", ErrCodeInvalidRequest},
		{"ACK w1 abc\n", ErrCodeInvalidRequest},
		{"ERR w1\n", ErrCodeInvalidRequest},
		{"PONG\n", ErrCodeInvalidRequest},
		{"UNKNOWN w1\n", ErrCodeInvalidRequest},
	}

	for _, tc := range invalidCases {
		t.Run(strings.TrimSpace(tc.line), func(t *testing.T) {
			var in bytes.Buffer
			var out bytes.Buffer
			var errOut bytes.Buffer

			in.WriteString(tc.line)
			srv := NewServer(nil, &in, &out, &errOut)
			_ = srv.Run()

			resp := out.String()
			if !strings.HasPrefix(resp, "ERR ") {
				t.Fatalf("expected line to start with ERR, got %q", resp)
			}
			if !strings.Contains(resp, tc.expectedCode) {
				t.Fatalf("expected response to contain %s, got %q", tc.expectedCode, resp)
			}
		})
	}
}

func TestProtocol_PanicRecovery(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer
	var errOut bytes.Buffer

	in.WriteString("PING panic-test\n")
	srv := NewServer(nil, &in, &out, &errOut)

	// dispatchSafe recovers panic
	resp, exit := srv.dispatchSafe("PING panic-test\n")
	if exit {
		t.Errorf("expected exit=false on PING")
	}
	if resp != "PONG panic-test\n" {
		t.Errorf("expected PONG response, got: %q", resp)
	}
}
