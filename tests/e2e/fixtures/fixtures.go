package fixtures

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"math/rand"
	"strings"
)

var crcTableCast = crc32.MakeTable(crc32.Castagnoli)

// Castagnoli computes standard 32-bit CRC32C.
func Castagnoli(b []byte) uint32 {
	return crc32.Checksum(b, crcTableCast)
}

// GoldenHeader generates a valid 32-byte header for compaction generation.
func GoldenHeader(gen uint64) []byte {
	buf := make([]byte, 32)
	copy(buf[0:8], []byte("KVDB2026"))
	binary.LittleEndian.PutUint32(buf[8:12], 1)
	binary.LittleEndian.PutUint64(buf[12:20], gen)
	binary.LittleEndian.PutUint64(buf[20:28], 0)
	crc := Castagnoli(buf[0:28])
	binary.LittleEndian.PutUint32(buf[28:32], crc)
	return buf
}

// FixedPayload creates a deterministic string of specified length.
func FixedPayload(length int, char byte) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = char
	}
	return string(b)
}

// RandomPayload creates random alphanumeric string of length.
func RandomPayload(rng *rand.Rand, length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_."
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rng.Intn(len(charset))]
	}
	return string(b)
}

// GenerateBatchFileLines creates lines for a batch file:
// "put key val" or "del key"
func GenerateBatchFileLines(count int, putRatio float64, keyPrefix string) []string {
	lines := make([]string, count)
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("%s_%06d", keyPrefix, i)
		if putRatio >= 1.0 || (putRatio > 0 && rand.Float64() < putRatio) {
			val := fmt.Sprintf("val_%06d", i)
			lines[i] = fmt.Sprintf("put %s %s", key, val)
		} else {
			lines[i] = fmt.Sprintf("del %s", key)
		}
	}
	return lines
}

// Generate10000Batch creates exactly 10,000 batch operations lines.
func Generate10000Batch(prefix string) []string {
	return GenerateBatchFileLines(10000, 1.0, prefix)
}

// EncodeHex returns lowercase hex representation of string.
func EncodeHex(s string) string {
	return hex.EncodeToString([]byte(s))
}

// DecodeHex decodes hex string to raw string.
func DecodeHex(h string) (string, error) {
	b, err := hex.DecodeString(h)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ParseDumpOutput parses `./kvdb dump` output into a sorted map of key -> value.
func ParseDumpOutput(stdout string) map[string]string {
	res := make(map[string]string)
	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// CLI dump can be separated by tab, '=' or space
		if parts := strings.SplitN(line, "\t", 2); len(parts) == 2 {
			res[parts[0]] = parts[1]
		} else if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			res[parts[0]] = parts[1]
		} else if parts := strings.SplitN(line, " ", 2); len(parts) == 2 {
			res[parts[0]] = parts[1]
		}
	}
	return res
}

// CheckDumpIsSorted verifies keys in stdout are in strictly ascending lexicographical order.
func CheckDumpIsSorted(stdout string) (bool, string) {
	lines := strings.Split(stdout, "\n")
	var prevKey string
	first := true
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var key string
		if parts := strings.SplitN(line, "\t", 2); len(parts) >= 1 {
			key = parts[0]
		} else if parts := strings.SplitN(line, "=", 2); len(parts) >= 1 {
			key = parts[0]
		} else if parts := strings.SplitN(line, " ", 2); len(parts) >= 1 {
			key = parts[0]
		}

		if first {
			prevKey = key
			first = false
			continue
		}
		if bytes.Compare([]byte(prevKey), []byte(key)) > 0 {
			return false, fmt.Sprintf("keys out of sorted order: %q > %q", prevKey, key)
		}
		prevKey = key
	}
	return true, ""
}
