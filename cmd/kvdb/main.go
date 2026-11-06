package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"kvdb/internal/engine"
	"kvdb/internal/format"
	"kvdb/internal/protocol"
)

const banner = `   _  ____      ______  ____  
 | |/ /\ \    / /  _ \|  _ \ 
 | ' /  \ \  / /| | | | |_) |
 |  <    \ \/ / | |_| |  _ < 
 |_|\_\   \__/  |____/|____/  Kill It and Prove It — Srinathon 3.0
`

func printUsage() {
	fmt.Fprintf(os.Stderr, "%s\n", banner)
	fmt.Fprintf(os.Stderr, "Usage: kvdb <command> [arguments...]\n")
	fmt.Fprintf(os.Stderr, "Available commands:\n")
	fmt.Fprintf(os.Stderr, "  open <path> [--max-bytes N]  Open/initialize database file\n")
	fmt.Fprintf(os.Stderr, "  put <path> <key> <val>        Insert or update key\n")
	fmt.Fprintf(os.Stderr, "  get <path> <key>              Retrieve value for key\n")
	fmt.Fprintf(os.Stderr, "  del <path> <key>              Delete key\n")
	fmt.Fprintf(os.Stderr, "  batch <path> <file>           Execute atomic batch mutations\n")
	fmt.Fprintf(os.Stderr, "  dump <path>                   Dump all live keys and values\n")
	fmt.Fprintf(os.Stderr, "  check <path>                  Check database integrity\n")
	fmt.Fprintf(os.Stderr, "  serve <path>                  Run interactive streaming server\n")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "open":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: kvdb open <path> [--max-bytes N]\n")
			os.Exit(1)
		}
		path := os.Args[2]
		var maxBytes uint64
		for i := 3; i < len(os.Args); i++ {
			if os.Args[i] == "--max-bytes" && i+1 < len(os.Args) {
				mb, err := strconv.ParseUint(os.Args[i+1], 10, 64)
				if err != nil {
					fmt.Fprintf(os.Stderr, "invalid --max-bytes value: %v\n", err)
					os.Exit(1)
				}
				maxBytes = mb
				i++
			}
		}

		eng, err := engine.Open(path, engine.Options{MaxBytes: maxBytes})
		if err != nil {
			fmt.Fprintf(os.Stderr, "open error: %v\n", err)
			os.Exit(1)
		}
		keyCount := eng.KeyCount()
		_ = eng.Close()
		fmt.Printf("Database opened: %d keys\n", keyCount)
		os.Exit(0)

	case "put":
		if len(os.Args) != 5 {
			fmt.Fprintf(os.Stderr, "Usage: kvdb put <path> <key> <val>\n")
			os.Exit(1)
		}
		path := os.Args[2]
		key := os.Args[3]
		val := os.Args[4]

		eng, err := engine.Open(path, engine.Options{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "open error: %v\n", err)
			os.Exit(1)
		}
		defer eng.Close()

		if err := eng.Put([]byte(key), []byte(val)); err != nil {
			fmt.Fprintf(os.Stderr, "put error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "get":
		if len(os.Args) != 4 {
			fmt.Fprintf(os.Stderr, "Usage: kvdb get <path> <key>\n")
			os.Exit(1)
		}
		path := os.Args[2]
		key := os.Args[3]

		eng, err := engine.Open(path, engine.Options{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "open error: %v\n", err)
			os.Exit(1)
		}
		defer eng.Close()

		val, found, err := eng.Get([]byte(key))
		if err != nil {
			fmt.Fprintf(os.Stderr, "get error: %v\n", err)
			os.Exit(1)
		}
		if !found {
			fmt.Println("NOTFOUND")
			os.Exit(0)
		}
		if len(val) == 0 {
			fmt.Println()
		} else {
			_, _ = os.Stdout.Write(val)
			fmt.Println()
		}
		os.Exit(0)

	case "del":
		if len(os.Args) != 4 {
			fmt.Fprintf(os.Stderr, "Usage: kvdb del <path> <key>\n")
			os.Exit(1)
		}
		path := os.Args[2]
		key := os.Args[3]

		eng, err := engine.Open(path, engine.Options{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "open error: %v\n", err)
			os.Exit(1)
		}
		defer eng.Close()

		if err := eng.Delete([]byte(key)); err != nil {
			fmt.Fprintf(os.Stderr, "del error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "batch":
		if len(os.Args) != 4 {
			fmt.Fprintf(os.Stderr, "Usage: kvdb batch <path> <file>\n")
			os.Exit(1)
		}
		path := os.Args[2]
		file := os.Args[3]

		ops, err := parseBatchFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		eng, err := engine.Open(path, engine.Options{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "open error: %v\n", err)
			os.Exit(1)
		}
		defer eng.Close()

		if err := eng.Batch(ops); err != nil {
			fmt.Fprintf(os.Stderr, "batch error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "dump":
		if len(os.Args) != 3 {
			fmt.Fprintf(os.Stderr, "Usage: kvdb dump <path>\n")
			os.Exit(1)
		}
		path := os.Args[2]

		eng, err := engine.Open(path, engine.Options{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "open error: %v\n", err)
			os.Exit(1)
		}
		defer eng.Close()

		pairs, err := eng.Dump()
		if err != nil {
			fmt.Fprintf(os.Stderr, "dump error: %v\n", err)
			os.Exit(1)
		}
		for _, p := range pairs {
			fmt.Printf("%s=%s\n", p.Key, string(p.Value))
		}
		os.Exit(0)

	case "check":
		if len(os.Args) != 3 {
			fmt.Fprintf(os.Stderr, "Usage: kvdb check <path>\n")
			os.Exit(1)
		}
		path := os.Args[2]

		if err := engine.CheckFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "check error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("OK")
		os.Exit(0)

	case "serve":
		var path string
		var maxBytes uint64
		if len(os.Args) >= 3 && !strings.HasPrefix(os.Args[2], "-") {
			path = os.Args[2]
		}
		for i := 2; i < len(os.Args); i++ {
			if os.Args[i] == "--max-bytes" && i+1 < len(os.Args) {
				mb, err := strconv.ParseUint(os.Args[i+1], 10, 64)
				if err != nil {
					fmt.Fprintf(os.Stderr, "invalid --max-bytes value: %v\n", err)
					os.Exit(1)
				}
				maxBytes = mb
				i++
			}
		}

		var eng *engine.Engine
		if path != "" {
			var err error
			eng, err = engine.Open(path, engine.Options{MaxBytes: maxBytes})
			if err != nil {
				fmt.Fprintf(os.Stderr, "open error: %v\n", err)
				os.Exit(1)
			}
			defer eng.Close()
		}

		srv := protocol.NewServer(eng, os.Stdin, os.Stdout, os.Stderr)
		if err := srv.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "serve error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "--help", "-h", "help":
		printUsage()
		os.Exit(0)

	case "banner":
		fmt.Println(banner)
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

// parseBatchFile reads and tokenizes line-oriented batch commands from file.
func parseBatchFile(file string) ([]format.Operation, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read batch file: %w", err)
	}

	var ops []format.Operation
	scanner := bufio.NewScanner(bytes.NewReader(content))
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 64*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		op, err := parseBatchLine(line)
		if err != nil {
			return nil, err
		}
		ops = append(ops, *op)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	return ops, nil
}

// parseBatchLine parses a single line into a format.Operation.
func parseBatchLine(line string) (*format.Operation, error) {
	line = strings.TrimSpace(line)
	idx := strings.IndexAny(line, " \t")
	var cmd, rest string
	if idx == -1 {
		cmd = line
		rest = ""
	} else {
		cmd = line[:idx]
		rest = strings.TrimSpace(line[idx+1:])
	}

	switch cmd {
	case "put":
		if rest == "" {
			return nil, fmt.Errorf("invalid put syntax in batch: %s", line)
		}
		key, rem, err := parseToken(rest)
		if err != nil {
			return nil, fmt.Errorf("invalid put key in batch: %w", err)
		}
		val, err := parseValueToken(rem)
		if err != nil {
			return nil, fmt.Errorf("invalid put value in batch: %w", err)
		}
		return &format.Operation{
			Type:  format.OpPut,
			Key:   key,
			Value: val,
		}, nil

	case "del":
		if rest == "" {
			return nil, fmt.Errorf("invalid del syntax in batch: %s", line)
		}
		key, rem, err := parseToken(rest)
		if err != nil {
			return nil, fmt.Errorf("invalid del key in batch: %w", err)
		}
		if strings.TrimSpace(rem) != "" {
			return nil, fmt.Errorf("invalid del syntax in batch: %s", line)
		}
		return &format.Operation{
			Type:  format.OpDel,
			Key:   key,
			Value: nil,
		}, nil

	default:
		return nil, fmt.Errorf("invalid operation in batch: %s", line)
	}
}

// parseToken extracts the first token (quoted or unquoted) and returns the rest of the string.
func parseToken(s string) ([]byte, string, error) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return nil, "", fmt.Errorf("empty token")
	}
	if s[0] == '"' {
		return unescapeQuoted(s)
	}
	idx := strings.IndexAny(s, " \t")
	if idx == -1 {
		return []byte(s), "", nil
	}
	return []byte(s[:idx]), s[idx+1:], nil
}

// parseValueToken extracts the value token, supporting empty values, quotes, or unquoted rest of line.
func parseValueToken(rem string) ([]byte, error) {
	rem = strings.TrimLeft(rem, " \t")
	if rem == "" {
		return []byte{}, nil
	}
	if rem[0] == '"' {
		val, remaining, err := unescapeQuoted(rem)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(remaining) != "" {
			return nil, fmt.Errorf("trailing characters after quoted value")
		}
		return val, nil
	}
	return []byte(rem), nil
}

// unescapeQuoted parses a leading quoted string handling \", \\, \n, \r, \t, and \xHH escapes.
func unescapeQuoted(s string) ([]byte, string, error) {
	if len(s) < 2 || s[0] != '"' {
		return nil, "", fmt.Errorf("expected opening quote")
	}
	var res bytes.Buffer
	i := 1
	for i < len(s) {
		c := s[i]
		if c == '"' {
			return res.Bytes(), s[i+1:], nil
		}
		if c == '\\' {
			if i+1 >= len(s) {
				return nil, "", fmt.Errorf("unfinished escape sequence")
			}
			nxt := s[i+1]
			switch nxt {
			case '"', '\\':
				res.WriteByte(nxt)
				i += 2
			case 'n':
				res.WriteByte('\n')
				i += 2
			case 'r':
				res.WriteByte('\r')
				i += 2
			case 't':
				res.WriteByte('\t')
				i += 2
			case 'x':
				if i+3 >= len(s) {
					return nil, "", fmt.Errorf("incomplete hex escape")
				}
				hexStr := s[i+2 : i+4]
				b, err := strconv.ParseUint(hexStr, 16, 8)
				if err != nil {
					return nil, "", fmt.Errorf("invalid hex escape: \\x%s", hexStr)
				}
				res.WriteByte(byte(b))
				i += 4
			default:
				res.WriteByte(c)
				res.WriteByte(nxt)
				i += 2
			}
		} else {
			res.WriteByte(c)
			i++
		}
	}
	return nil, "", fmt.Errorf("unclosed quote")
}

