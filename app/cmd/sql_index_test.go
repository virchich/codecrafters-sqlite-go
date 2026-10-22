package cmd

import (
	"encoding/binary"
	"testing"
)

// Convert the shared table fixture cells to index cells by removing rowid
// from the cell header; the index rowid remains inside the record payload.
func commandIndexLeaf(t *testing.T, rows [][]any) []byte {
	source := fixtureLeaf(t, 0, rows)
	p := make([]byte, 4096)
	p[0] = 10
	binary.BigEndian.PutUint16(p[3:], uint16(len(rows)))
	end := 4096
	for i := range rows {
		off := int(binary.BigEndian.Uint16(source[8+2*i:]))
		size := int(source[off]) // These small fixture payloads use one-byte sizes.
		cell := append([]byte{byte(size)}, source[off+2:off+2+size]...)
		end -= len(cell)
		copy(p[end:], cell)
		binary.BigEndian.PutUint16(p[8+2*i:], uint16(end))
	}
	binary.BigEndian.PutUint16(p[5:], uint16(end))
	return p
}

func TestSQLIndexedQueries(t *testing.T) {
	schema := [][]any{
		{"table", "companies", "companies", int64(2), "CREATE TABLE companies(id INTEGER PRIMARY KEY, name TEXT, country TEXT)"},
		{"index", "idx_country", "companies", int64(3), "CREATE INDEX idx_country ON companies(country)"},
	}
	rows := [][]any{{nil, "first", "eritrea"}, {nil, "second", "myanmar"}, {nil, "third", "eritrea"}}
	idx := commandIndexLeaf(t, [][]any{{"eritrea", int64(1)}, {"eritrea", int64(3)}, {"myanmar", int64(2)}})
	path := writeCommandDB(t, schema, fixtureLeaf(t, 0, rows), idx)
	for _, tc := range []struct{ q, want string }{
		{"SELECT id,name FROM companies WHERE country = 'eritrea'", "1|first\n3|third\n"},
		{"SELECT COUNT(*) FROM companies WHERE country = 'eritrea'", "2\n"},
		{"SELECT * FROM companies WHERE country = 'myanmar'", "2|second|myanmar\n"},
		{"SELECT id FROM companies WHERE country = 'missing'", ""},
		{"SELECT COUNT(*) FROM companies WHERE country = 'z'", "0\n"},
		{"SELECT id FROM companies WHERE country > 'eritrea'", "2\n"},
		{"SELECT name FROM companies WHERE id = 3", "third\n"},
		{"SELECT id FROM COMPANIES WHERE COUNTRY = 'eritrea'", "1\n3\n"},
	} {
		t.Run(tc.q, func(t *testing.T) {
			got, err := captureQueryOutput(t, func() error { return Sql(path, tc.q) })
			if err != nil || got != tc.want {
				t.Fatalf("got=%q err=%v want=%q", got, err, tc.want)
			}
		})
	}
}

func TestSQLIndexSkipsUnmatchedTableRecords(t *testing.T) {
	schema := [][]any{
		{"table", "companies", "companies", int64(2), "CREATE TABLE companies(id INTEGER PRIMARY KEY, name TEXT, country TEXT)"},
		{"index", "idx_country", "companies", int64(3), "CREATE INDEX idx_country ON companies(country)"},
	}
	leaf := fixtureLeaf(t, 0, [][]any{{nil, "unrelated", "egypt"}, {nil, "match", "eritrea"}})
	off := int(binary.BigEndian.Uint16(leaf[8:]))
	leaf[off+2] = 0 // A full table scan would try to decode this malformed record.
	idx := commandIndexLeaf(t, [][]any{{"egypt", int64(1)}, {"eritrea", int64(2)}})
	path := writeCommandDB(t, schema, leaf, idx)
	got, err := captureQueryOutput(t, func() error { return Sql(path, "SELECT id,name FROM companies WHERE country = 'eritrea'") })
	if err != nil || got != "2|match\n" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestSQLUnsupportedIndexFallsBack(t *testing.T) {
	for _, tc := range []struct{ name, column, index string }{
		{"descending", "country TEXT", "country DESC)"},
		{"partial", "country TEXT", "country) WHERE id > 1"},
		{"explicit collation", "country TEXT", "country COLLATE NOCASE)"},
		{"inherited collation", "country TEXT COLLATE NOCASE", "country)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := [][]any{
				{"table", "companies", "companies", int64(2), "CREATE TABLE companies(id INTEGER PRIMARY KEY, name TEXT, " + tc.column + ")"},
				{"index", "idx_country", "companies", int64(3), "CREATE INDEX idx_country ON companies(" + tc.index},
			}
			// Deliberately unreadable index page: fallback must never open it.
			path := writeCommandDB(t, schema, fixtureLeaf(t, 0, [][]any{{nil, "match", "eritrea"}}), make([]byte, 4096))
			got, err := captureQueryOutput(t, func() error { return Sql(path, "SELECT id,name FROM companies WHERE country = 'eritrea'") })
			if err != nil || got != "1|match\n" {
				t.Fatalf("got=%q err=%v", got, err)
			}
		})
	}
}
