package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"db"}, {"db", ".unknown"}, {"db", ".tables", "extra"}} {
		if err := run(args); err == nil {
			t.Fatalf("run(%q) succeeded", args)
		}
	}
	missing := filepath.Join(t.TempDir(), "missing.db")
	for _, command := range []string{".tables", ".dbinfo", "SELECT name FROM people"} {
		if err := run([]string{missing, command}); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: %v", command, err)
		}
	}
	corrupt := filepath.Join(t.TempDir(), "corrupt.db")
	if err := os.WriteFile(corrupt, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{".tables", ".dbinfo", "SELECT name FROM people"} {
		if err := run([]string{corrupt, command}); err == nil || !strings.Contains(err.Error(), "read database header") {
			t.Fatalf("%s: %v", command, err)
		}
	}
}
