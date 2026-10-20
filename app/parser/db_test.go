package parser

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDBOpenClose(t *testing.T) {
	f := parserTestFile(t, databaseHeaderFixture(4096, 7))
	db, err := Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if db.Header.PageSize != 4096 || db.Header.ReservedBytes != 7 {
		t.Fatalf("header=%+v", db.Header)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.file.ReadAt(make([]byte, 1), 0); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("file not closed: %v", err)
	}
	if err := db.Close(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("second close: %v", err)
	}
}

func TestDBOpenErrors(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "missing.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	f := parserTestFile(t, []byte{1})
	if _, err := Open(f.Name()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated header: %v", err)
	}
}

func TestLookupTableIndexes(t *testing.T) {
	db := schemaLookupFixture(t, [][]any{
		{"index", "idx_people_name", "People", int64(3), "CREATE INDEX idx_people_name ON People(name)"},
		{"table", "People", "People", int64(2), "CREATE TABLE People(id INTEGER PRIMARY KEY, name TEXT)"},
		{"index", "sqlite_autoindex_People_1", "People", int64(4), nil},
		{"index", "idx_other", "other", int64(5), "CREATE INDEX idx_other ON other(name)"},
	})
	info, err := db.LookupTable("people")
	if err != nil {
		t.Fatal(err)
	}
	if info.TableRootPage != 2 || info.TableDefinition.RowIDColumn != 0 || len(info.IndexInfo) != 1 {
		t.Fatalf("unexpected table metadata: %+v", info)
	}
	idx := info.IndexInfo[0]
	if idx.IndexRootPage != 3 ||
		len(idx.IndexDefinitions.Columns) != 1 || idx.IndexDefinitions.Columns[0] != "name" {
		t.Fatalf("unexpected index metadata: %+v", idx)
	}
	if missing, err := db.LookupTable("missing"); err != nil || missing.TableExists {
		t.Fatalf("missing table: %+v, %v", missing, err)
	}
}

func TestLookupTableInvalidRoot(t *testing.T) {
	for _, root := range []int64{0, -1, 1 << 32} {
		db := schemaLookupFixture(t, [][]any{
			{"table", "people", "people", root, "CREATE TABLE people(id INTEGER)"},
		})
		if _, err := db.LookupTable("people"); err == nil {
			t.Fatalf("accepted root %d", root)
		}
	}
}

// Build a small sqlite_schema leaf without depending on a SQLite executable.
func schemaLookupFixture(t *testing.T, rows [][]any) *DB {
	t.Helper()
	varint := func(value uint64) []byte {
		b := []byte{byte(value & 127)}
		for value >>= 7; value != 0; value >>= 7 {
			b = append([]byte{byte(value&127) | 128}, b...)
		}
		return b
	}
	page := make([]byte, 4096)
	page[100] = TableLeafPage
	binary.BigEndian.PutUint16(page[103:], uint16(len(rows)))
	end := len(page)
	for i, values := range rows {
		var serial, body []byte
		for _, value := range values {
			switch v := value.(type) {
			case nil:
				serial = append(serial, 0)
			case string:
				serial = append(serial, varint(uint64(13+2*len(v)))...)
				body = append(body, v...)
			case int64:
				serial = append(serial, 6)
				body = binary.BigEndian.AppendUint64(body, uint64(v))
			default:
				t.Fatalf("unsupported fixture value %T", value)
			}
		}
		payload := append([]byte{byte(1 + len(serial))}, serial...)
		payload = append(payload, body...)
		cell := append(varint(uint64(len(payload))), byte(i+1))
		cell = append(cell, payload...)
		end -= len(cell)
		copy(page[end:], cell)
		binary.BigEndian.PutUint16(page[108+i*2:], uint16(end))
	}
	binary.BigEndian.PutUint16(page[105:], uint16(end))
	return &DB{file: parserTestFile(t, page), Header: DbHeader{PageSize: 4096}}
}

func TestDBPayloadValidation(t *testing.T) {
	db := cursorTestDB(t, make([]byte, 1024), 0)
	for _, tc := range []struct {
		name    string
		cell    TableLeafCell
		message string
	}{
		{"local exceeds total", TableLeafCell{PayloadSize: 1, LocalPayload: []byte{1, 2}}, "local payload"},
		{"unnecessary overflow", TableLeafCell{PayloadSize: 1, LocalPayload: []byte{1}, OverflowPageNumber: 2}, "unexpected overflow"},
		{"missing overflow", TableLeafCell{PayloadSize: 2, LocalPayload: []byte{1}}, "missing overflow"},
		{"impossible size", TableLeafCell{PayloadSize: ^uint64(0), OverflowPageNumber: 2}, "payload size"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.readPayload(tc.cell.LocalPayload, tc.cell.PayloadSize, tc.cell.OverflowPageNumber); err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("expected %q error, got %v", tc.message, err)
			}
		})
	}
}

func TestDBStatsIndependentOfLookup(t *testing.T) {
	db := schemaLookupFixture(t, [][]any{
		{"table", "apples", "apples", int64(2), "CREATE TABLE apples(id INTEGER PRIMARY KEY)"},
		{"index", "idx_apples", "apples", int64(3), "CREATE INDEX idx_apples ON apples(id)"},
		{"table", "oranges", "oranges", int64(4), "CREATE TABLE oranges(id INTEGER)"},
		{"view", "fruit", "fruit", int64(0), "CREATE VIEW fruit AS SELECT * FROM apples"},
		{"table", "sqlite_sequence", "sqlite_sequence", int64(5), nil},
		{"table", "SQLITE_internal", "SQLITE_internal", int64(6), nil},
	})
	for i := 0; i < 2; i++ {
		if _, err := db.LookupTable("apples"); err != nil {
			t.Fatal(err)
		}
		info, err := db.DBStats()
		if err != nil {
			t.Fatal(err)
		}
		if info.TotalNumberOfTables != 2 || strings.Join(info.TableNames, ",") != "apples,oranges" {
			t.Fatalf("stats=%+v", info)
		}
	}
}
