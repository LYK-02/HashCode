package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// Ensure kvdb binary exists or build it
	binPath := os.Getenv("KVDB_BIN")
	if binPath == "" {
		cwd, err := os.Getwd()
		if err == nil {
			candidate := filepath.Join(cwd, "kvdb")
			if _, statErr := os.Stat(candidate); statErr == nil {
				binPath = candidate
			} else {
				candidateRoot := filepath.Join(cwd, "..", "..", "kvdb")
				if _, statErr2 := os.Stat(candidateRoot); statErr2 == nil {
					binPath = candidateRoot
				}
			}
		}
	}

	if binPath == "" || !fileExists(binPath) {
		// Attempt to build kvdb
		cmd := exec.Command("go", "build", "-o", "kvdb", "./cmd/kvdb")
		// Find root
		root := findProjectRoot()
		if root != "" {
			cmd.Dir = root
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to auto-build kvdb in %s: %v\n", root, err)
			} else {
				binPath = filepath.Join(root, "kvdb")
			}
		}
	}

	if binPath != "" {
		_ = os.Setenv("KVDB_BIN", binPath)
	}

	os.Exit(m.Run())
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
