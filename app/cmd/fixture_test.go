package cmd

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// Small synthetic databases keep command tests independent of sqlite3 and sample.db.
func fixtureVarint(n uint64) []byte {
	b := []byte{byte(n & 127)}
	for n >>= 7; n != 0; n >>= 7 {
		b = append([]byte{byte(n&127) | 128}, b...)
	}
	return b
}

func fixtureLeaf(t *testing.T, start int, rows [][]any) []byte {
	t.Helper()
	p := make([]byte, 4096)
	p[start] = 13
	binary.BigEndian.PutUint16(p[start+3:], uint16(len(rows)))
	end := len(p)
	for i, row := range rows {
		var types, body []byte
		for _, v := range row {
			switch v := v.(type) {
			case nil:
				types = append(types, 0)
			case string:
				types = append(types, fixtureVarint(uint64(13+len(v)*2))...)
				body = append(body, v...)
			case int64:
				types = append(types, 6)
				body = binary.BigEndian.AppendUint64(body, uint64(v))
			default:
				t.Fatalf("unsupported fixture value %T", v)
			}
		}
		if len(types)+1 >= 128 {
			t.Fatal("fixture record header too large")
		}
		payload := append([]byte{byte(len(types) + 1)}, types...)
		payload = append(payload, body...)
		cell := append(fixtureVarint(uint64(len(payload))), byte(i+1))
		cell = append(cell, payload...)
		end -= len(cell)
		if end < start+8+2*len(rows) {
			t.Fatal("fixture page overflow")
		}
		copy(p[end:], cell)
		binary.BigEndian.PutUint16(p[start+8+i*2:], uint16(end))
	}
	binary.BigEndian.PutUint16(p[start+5:], uint16(end))
	return p
}

func writeCommandDB(t *testing.T, schema [][]any, pages ...[]byte) string {
	t.Helper()
	first := fixtureLeaf(t, 100, schema)
	copy(first, "SQLite format 3\x00")
	binary.BigEndian.PutUint16(first[16:], 4096)
	first[18], first[19] = 1, 1
	data := first
	for _, p := range pages {
		data = append(data, p...)
	}
	path := filepath.Join(t.TempDir(), "commands.db")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func commandFixture(t *testing.T, empty, multiPage bool) string {
	t.Helper()
	schema := [][]any{{"table", "people", "people", int64(2), "CREATE TABLE people(name TEXT, id INTEGER PRIMARY KEY)"}}
	rows := [][]any{{"Ada", nil}, {"Bob", nil}}
	if empty {
		rows = nil
	}
	if !multiPage {
		return writeCommandDB(t, schema, fixtureLeaf(t, 0, rows))
	}
	root := make([]byte, 4096)
	root[0] = 5
	binary.BigEndian.PutUint16(root[3:], 1)
	binary.BigEndian.PutUint16(root[5:], 4091)
	binary.BigEndian.PutUint32(root[8:], 4)
	binary.BigEndian.PutUint16(root[12:], 4091)
	binary.BigEndian.PutUint32(root[4091:], 3)
	root[4095] = 1
	left, right := fixtureLeaf(t, 0, rows[:1]), fixtureLeaf(t, 0, rows[1:])
	off := int(binary.BigEndian.Uint16(right[8:]))
	right[off+1] = 2 // Second row's rowid; its payload-size varint is one byte.
	return writeCommandDB(t, schema, root, left, right)
}
