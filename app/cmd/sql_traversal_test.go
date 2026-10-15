package cmd

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
	s "github.com/codecrafters-io/sqlite-starter-go/app/sql"
	"github.com/codecrafters-io/sqlite-starter-go/app/table"
)

// Synthetic pages avoid a runtime dependency on sqlite3 or repository databases.
// Each leaf contains (name TEXT, id INTEGER PRIMARY KEY) with a NULL id slot.
func leafFixture(id byte, name byte) []byte {
	buf := make([]byte, 512)
	copy(buf, []byte{13, 0, 0, 0, 1, 1, 250, 0})
	binary.BigEndian.PutUint16(buf[8:], 506)
	copy(buf[506:], []byte{4, id, 3, 15, 0, name})
	return buf
}

func interiorFixture(pageOne bool, left, right uint32, boundary byte) []byte {
	buf := make([]byte, 512)
	start := 0
	if pageOne {
		start = 100
	}
	copy(buf[start:], []byte{5, 0, 0, 0, 1, 1, 251, 0})
	binary.BigEndian.PutUint32(buf[start+8:], right)
	binary.BigEndian.PutUint16(buf[start+12:], 507)
	binary.BigEndian.PutUint32(buf[507:], left)
	buf[511] = boundary
	return buf
}

func openFixture(t *testing.T, pages ...[]byte) *os.File {
	t.Helper()
	var data []byte
	for _, page := range pages {
		data = append(data, page...)
	}
	path := filepath.Join(t.TempDir(), "tree.db")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// These tests must not run in parallel because command output uses os.Stdout.
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
	queryErr := run()
	if _, err := out.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), queryErr
}

func TestLeafQueryRowID(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"SELECT id,name FROM people WHERE id = 7", "7|A\n"},
		{"SELECT rowid,_rowid_,oid FROM people WHERE oid = 7", "7|7|7\n"},
		{"SELECT * FROM people", "A|7\n"},
		{"SELECT id FROM people WHERE rowid = 8", ""},
	} {
		t.Run(tc.query, func(t *testing.T) {
			f := openFixture(t, make([]byte, 512), leafFixture(7, 'A'))
			page, err := parser.NewPage(2, 512)
			if err != nil {
				t.Fatal(err)
			}
			if err := page.Parse(f); err != nil {
				t.Fatal(err)
			}
			schema, err := table.NewRowIDSchema([]string{"name", "id"}, 1)
			if err != nil {
				t.Fatal(err)
			}
			command, err := s.NewSqlLine(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			got, err := captureQueryOutput(t, func() error { return executeTableQuery(page, schema, &command, 512, f) })
			if err != nil || got != tc.want {
				t.Fatalf("output=%q err=%v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestInteriorTraversal(t *testing.T) {
	for _, tc := range []struct {
		name, query, want string
		nested            bool
	}{
		{"rightmost child", "SELECT id,name FROM people", "7|A\n8|B\n", false},
		{"aggregate count", "SELECT COUNT(*) FROM people", "3\n", false},
		{"nested interior", "SELECT id,name FROM people", "6|C\n7|A\n8|B\n", true},
		{"nested filtered count", "SELECT COUNT(*) FROM people WHERE id >= 7", "2\n", true},
		{"no matches", "SELECT COUNT(*) FROM people WHERE id = 99", "0\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages := [][]byte{interiorFixture(true, 2, 3, 7), leafFixture(7, 'A'), leafFixture(8, 'B')}
			if tc.name == "aggregate count" {
				// Two cell children expose per-page counting independently of
				// the missing rightmost-child traversal.
				root := interiorFixture(true, 2, 4, 6)
				binary.BigEndian.PutUint16(root[103:], 2)
				binary.BigEndian.PutUint16(root[105:], 502)
				binary.BigEndian.PutUint16(root[114:], 502)
				binary.BigEndian.PutUint32(root[502:], 3)
				root[506] = 7
				pages = [][]byte{root, leafFixture(6, 'C'), leafFixture(7, 'A'), leafFixture(8, 'B')}
			}
			if tc.nested {
				pages[1] = interiorFixture(false, 4, 5, 6)
				pages = append(pages, leafFixture(6, 'C'), leafFixture(7, 'A'))
			}
			f := openFixture(t, pages...)
			page, err := parser.NewPage(1, 512)
			if err != nil {
				t.Fatal(err)
			}
			if err := page.Parse(f); err != nil {
				t.Fatal(err)
			}
			schema, err := table.NewRowIDSchema([]string{"name", "id"}, 1)
			if err != nil {
				t.Fatal(err)
			}
			command, err := s.NewSqlLine(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			got, err := captureQueryOutput(t, func() error { return executeTableQuery(page, schema, &command, 512, f) })
			if err != nil || got != tc.want {
				t.Fatalf("output=%q err=%v; want %q", got, err, tc.want)
			}
		})
	}
}
