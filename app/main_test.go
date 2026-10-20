package main

import (
	"errors"
	"os"
	"path/filepath"
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
}
