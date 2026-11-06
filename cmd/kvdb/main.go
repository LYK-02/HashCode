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
)

func printUsage() {
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

		content, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read batch file: %v\n", err)
			os.Exit(1)
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
			parts := strings.SplitN(line, " ", 3)
			switch parts[0] {
			case "put":
				if len(parts) < 2 {
					fmt.Fprintf(os.Stderr, "invalid put syntax in batch: %s\n", line)
					os.Exit(1)
				}
				val := ""
				if len(parts) == 3 {
					val = parts[2]
				}
				ops = append(ops, format.Operation{
					Type:  format.OpPut,
					Key:   []byte(parts[1]),
					Value: []byte(val),
				})
			case "del":
				if len(parts) < 2 {
					fmt.Fprintf(os.Stderr, "invalid del syntax in batch: %s\n", line)
					os.Exit(1)
				}
				ops = append(ops, format.Operation{
					Type:  format.OpDel,
					Key:   []byte(parts[1]),
					Value: nil,
				})
			default:
				fmt.Fprintf(os.Stderr, "invalid operation in batch: %s\n", line)
				os.Exit(1)
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "scanner error: %v\n", err)
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
		// M1 CLI router stub: verifies command routing; full implementation scheduled in M4
		fmt.Fprintf(os.Stdout, "Command '%s' acknowledged (M1 CLI router)\n", cmd)

	case "--help", "-h", "help":
		printUsage()
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}
