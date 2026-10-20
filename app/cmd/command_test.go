package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests replace stdout and must not run in parallel.
func captureQueryOutput(t *testing.T, run func() error) (string, error) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	original := os.Stdout
	os.Stdout = out
	defer func() { os.Stdout = original }()
	commandErr := run()
	if _, err := out.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), commandErr
}

func TestSQLCommandOutput(t *testing.T) {
	path := commandFixture(t, false, false)
	for _, tc := range []struct{ query, want string }{
		{"SELECT id,name FROM people", "1|Ada\n2|Bob\n"},
		{"SELECT name,id,name FROM people WHERE id = 1", "Ada|1|Ada\n"},
		{"SELECT * FROM people", "Ada|1\nBob|2\n"},
		{"SELECT rowid,_rowid_,oid FROM people WHERE oid = 2", "2|2|2\n"},
		{"SELECT name FROM PEOPLE WHERE name = 'Ada'", "Ada\n"},
		{"SELECT name FROM people WHERE id = 99", ""},
		{"SELECT COUNT(*) FROM people", "2\n"},
		{"SELECT COUNT(*) FROM people WHERE id >= 2", "1\n"},
		{"SELECT COUNT(*) FROM people WHERE id = 99", "0\n"},
		{"SELECT COUNT(*) FROM people WHERE name = NULL", "0\n"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			got, err := captureQueryOutput(t, func() error { return Sql(path, tc.query) })
			if err != nil || got != tc.want {
				t.Fatalf("output=%q err=%v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestSQLCommandEmptyTable(t *testing.T) {
	path := commandFixture(t, true, false)
	for _, tc := range []struct{ query, want string }{
		{"SELECT * FROM people", ""},
		{"SELECT COUNT(*) FROM people", "0\n"},
		{"SELECT COUNT(*) FROM people WHERE id > 0", "0\n"},
	} {
		got, err := captureQueryOutput(t, func() error { return Sql(path, tc.query) })
		if err != nil || got != tc.want {
			t.Fatalf("%s: output=%q err=%v", tc.query, got, err)
		}
	}
}

func TestSQLCommandErrors(t *testing.T) {
	for _, empty := range []bool{false, true} {
		path := commandFixture(t, empty, false)
		for _, tc := range []struct{ query, message string }{
			{"SELECT name FROM missing", "does not exist"},
			{"SELECT missing FROM people", "unknown column"},
			{"SELECT name FROM people WHERE missing = 1", "unknown column"},
			{"SELECT COUNT(*) FROM people WHERE missing = 1", "unknown column"},
			{"SELECT FROM", ""},
			{"SELECT name FROM people ORDER BY name", "unsupported"},
		} {
			t.Run(fmt.Sprintf("empty=%v/%s", empty, tc.query), func(t *testing.T) {
				got, err := captureQueryOutput(t, func() error { return Sql(path, tc.query) })
				if err == nil || !strings.Contains(err.Error(), tc.message) || got != "" {
					t.Fatalf("output=%q err=%v; want error containing %q", got, err, tc.message)
				}
			})
		}
	}
	path := writeCommandDB(t, [][]any{{"table", "people", "people", int64(2), "CREATE TABLE people(id INTEGER PRIMARY KEY) WITHOUT ROWID"}}, fixtureLeaf(t, 0, nil))
	_, err := captureQueryOutput(t, func() error { return Sql(path, "SELECT * FROM people") })
	if err == nil || !strings.Contains(err.Error(), "WITHOUT ROWID") {
		t.Fatalf("unsupported table: %v", err)
	}
}

func TestSQLCommandMultiPageCount(t *testing.T) {
	path := commandFixture(t, false, true)
	got, err := captureQueryOutput(t, func() error { return Sql(path, "SELECT COUNT(*) FROM people WHERE id >= 2") })
	if err != nil || got != "1\n" {
		t.Fatalf("output=%q err=%v; want one aggregate", got, err)
	}
}

func TestMetadataCommands(t *testing.T) {
	for _, tc := range []struct {
		name   string
		schema [][]any
		tables string
		count  int
	}{
		{"mixed", [][]any{
			{"table", "apples", "apples", int64(2), nil},
			{"table", "sqlite_sequence", "sqlite_sequence", int64(3), nil},
			{"index", "idx_apples", "apples", int64(4), nil},
			{"view", "fruit", "fruit", int64(0), nil},
			{"table", "oranges", "oranges", int64(5), nil},
		}, "apples oranges\n", 2},
		{"single", [][]any{{"table", "apples", "apples", int64(2), nil}}, "apples\n", 1},
		{"internal only", [][]any{{"table", "sqlite_sequence", "sqlite_sequence", int64(2), nil}}, "\n", 0},
		{"empty", nil, "\n", 0},
	} {
		path := writeCommandDB(t, tc.schema)
		t.Run(tc.name+"/dbinfo", func(t *testing.T) {
			got, err := captureQueryOutput(t, func() error { return DBInfo(path) })
			want := fmt.Sprintf("database page size: 4096\nnumber of tables: %d\n", tc.count)
			if err != nil || got != want {
				t.Fatalf("output=%q err=%v; want %q", got, err, want)
			}
		})
		t.Run(tc.name+"/tables", func(t *testing.T) {
			got, err := captureQueryOutput(t, func() error { return Tables(path) })
			if err != nil || got != tc.tables {
				t.Fatalf("output=%q err=%v; want %q", got, err, tc.tables)
			}
		})
	}
}

func TestCommandsFileErrors(t *testing.T) {
	for _, command := range []struct {
		name string
		run  func(string) error
	}{
		{"dbinfo", DBInfo}, {"tables", Tables},
		{"select", func(path string) error { return Sql(path, "SELECT * FROM people") }},
	} {
		t.Run(command.name, func(t *testing.T) {
			missing := filepath.Join(t.TempDir(), "missing.db")
			got, err := captureQueryOutput(t, func() error { return command.run(missing) })
			if !errors.Is(err, os.ErrNotExist) || got != "" {
				t.Fatalf("missing file: output=%q err=%v", got, err)
			}
			corrupt := filepath.Join(t.TempDir(), "corrupt.db")
			if err := os.WriteFile(corrupt, []byte("bad"), 0600); err != nil {
				t.Fatal(err)
			}
			got, err = captureQueryOutput(t, func() error { return command.run(corrupt) })
			if !errors.Is(err, io.ErrUnexpectedEOF) || got != "" {
				t.Fatalf("corrupt file: output=%q err=%v", got, err)
			}
		})
	}
}
