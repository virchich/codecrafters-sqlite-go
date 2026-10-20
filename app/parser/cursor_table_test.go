package parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTableCursorInitialization(t *testing.T) {
	for _, tc := range []struct {
		name          string
		types         []byte
		emptyInterior bool
		wantError     bool
	}{
		{"leaf root", []byte{TableLeafPage}, false, false},
		{"nested interior", []byte{TableInteriorPage, TableInteriorPage, TableLeafPage}, false, false},
		{"rightmost only", []byte{TableInteriorPage, TableLeafPage}, true, false},
		{"index root", []byte{IndexLeafPage}, false, true},
		{"index child", []byte{TableInteriorPage, IndexLeafPage}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, 512*(len(tc.types)+1))
			for i, typ := range tc.types {
				page := data[(i+1)*512 : (i+2)*512]
				page[0] = typ
				binary.BigEndian.PutUint16(page[5:], 500)
				if typ == TableLeafPage {
					binary.BigEndian.PutUint16(page[3:], 1)
					binary.BigEndian.PutUint16(page[8:], 500)
					copy(page[500:], []byte{3, 7, 2, 1, 42})
				}
				if typ == TableInteriorPage {
					binary.BigEndian.PutUint32(page[8:], uint32(i+3))
					if !tc.emptyInterior {
						binary.BigEndian.PutUint16(page[3:], 1)
						binary.BigEndian.PutUint16(page[12:], 500)
						binary.BigEndian.PutUint32(page[500:], uint32(i+3))
						page[504] = 7
					}
				}
			}
			path := filepath.Join(t.TempDir(), "cursor.db")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { f.Close() })
			db := DB{file: f, Header: DbHeader{PageSize: 512}}
			cursor, err := db.NewTableCursor(2)
			if tc.wantError {
				if err == nil {
					t.Fatal("accepted index page")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			record, err := cursor.Next()
			if err != nil || record.RowID != 7 || len(record.Record.Body) != 1 || record.Record.Body[0] != int64(42) {
				t.Fatalf("first record=%+v err=%v", record, err)
			}
		})
	}
}

func cursorTestDB(t *testing.T, data []byte, reserved byte) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scan.db")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return &DB{file: f, Header: DbHeader{PageSize: 512, ReservedBytes: reserved}}
}

func TestTableCursorNext(t *testing.T) {
	data := make([]byte, 6*512)
	interior := func(number, left, right uint32) {
		p := data[(number-1)*512 : number*512]
		p[0] = TableInteriorPage
		binary.BigEndian.PutUint16(p[3:], 1)
		binary.BigEndian.PutUint16(p[5:], 500)
		binary.BigEndian.PutUint32(p[8:], right)
		binary.BigEndian.PutUint16(p[12:], 500)
		binary.BigEndian.PutUint32(p[500:], left)
		p[504] = 7
	}
	interior(2, 3, 6)
	interior(3, 4, 5)
	for _, number := range []uint32{4, 5, 6} {
		p := data[(number-1)*512 : number*512]
		p[0] = TableLeafPage
		binary.BigEndian.PutUint16(p[5:], 500)
		if number == 5 {
			continue
		} // Empty middle leaf must not end the scan.
		binary.BigEndian.PutUint16(p[3:], 1)
		binary.BigEndian.PutUint16(p[8:], 500)
		copy(p[500:], []byte{3, byte(number), 2, 1, byte(number)})
	}
	db := cursorTestDB(t, data, 0)
	c, err := db.NewTableCursor(2)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int64{4, 6} {
		r, err := c.Next()
		if err != nil || r.RowID != want || len(r.Record.Body) != 1 || r.Record.Body[0] != want {
			t.Fatalf("record=%+v err=%v want=%d", r, err, want)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := c.Next(); err != io.EOF || !c.exhausted {
			t.Fatalf("EOF: %v", err)
		}
	}
}

func TestTableCursorOverflow(t *testing.T) {
	for _, mode := range []string{"valid", "short", "cycle", "extra", "outside", "missing"} {
		t.Run(mode, func(t *testing.T) {
			data := make([]byte, 5*512)
			// A 1000-byte record: 3-byte header plus a 997-byte BLOB.
			payload := append([]byte{3, 0x8f, 0x56}, bytes.Repeat([]byte{0xab}, 997)...)
			leaf := data[512:1024]
			leaf[0] = TableLeafPage
			binary.BigEndian.PutUint16(leaf[3:], 1)
			binary.BigEndian.PutUint16(leaf[5:], 400)
			binary.BigEndian.PutUint16(leaf[8:], 400)
			// With U=480, P=1000 leaves 48 bytes locally.
			copy(leaf[400:], []byte{0x87, 0x68, 7})
			copy(leaf[403:], payload[:48])
			binary.BigEndian.PutUint32(leaf[451:], 3)
			binary.BigEndian.PutUint32(data[1024:], 4)
			copy(data[1028:1504], payload[48:524])
			copy(data[1540:2016], payload[524:])
			switch mode {
			case "short":
				binary.BigEndian.PutUint32(data[1024:], 0)
			case "cycle":
				// Larger declared payload leaves bytes outstanding after revisiting page 3.
				copy(leaf[400:], []byte{0x8b, 0x44}) // 1476, same local length.
				binary.BigEndian.PutUint32(data[1536:], 3)
			case "extra":
				binary.BigEndian.PutUint32(data[1536:], 5)
			case "outside":
				binary.BigEndian.PutUint32(data[1024:], 99)
			case "missing":
				binary.BigEndian.PutUint32(leaf[451:], 0)
			}
			c, err := cursorTestDB(t, data, 32).NewTableCursor(2)
			if err != nil {
				t.Fatal(err)
			}
			r, err := c.Next()
			if mode == "valid" {
				if err != nil || r.RowID != 7 || !bytes.Equal(r.Record.Body[0].([]byte), payload[3:]) {
					t.Fatalf("record=%+v err=%v", r, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "page 2: cell 0: rowId 7") {
				t.Fatalf("missing context: %v", err)
			}
			if mode == "short" || mode == "outside" || mode == "missing" {
				if !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("lost underlying EOF: %v", err)
				}
			}
			if _, again := c.Next(); again != err {
				t.Fatalf("failure not preserved: %v", again)
			}
		})
	}
}

func TestTableCursorInvalidInitialization(t *testing.T) {
	for _, tc := range []struct {
		name string
		root uint32
		db   *DB
	}{
		{"zero root", 0, &DB{}},
		{"nil database", 1, nil},
		{"missing file", 1, &DB{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewTableCursor(tc.root, tc.db); err == nil {
				t.Fatal("accepted invalid cursor")
			}
		})
	}
	var c TableCursor
	_, first := c.Next()
	_, second := c.Next()
	if first == nil || second != first {
		t.Fatalf("uninitialized cursor errors: %v, %v", first, second)
	}
}

func TestTableCursorInteriorCycle(t *testing.T) {
	data := make([]byte, 1024)
	p := data[512:]
	p[0] = TableInteriorPage
	binary.BigEndian.PutUint16(p[5:], 512)
	binary.BigEndian.PutUint32(p[8:], 2)
	_, err := cursorTestDB(t, data, 0).NewTableCursor(2)
	if err == nil || !strings.Contains(err.Error(), "cycle") || !strings.Contains(err.Error(), "page 2") {
		t.Fatalf("expected contextual cycle error, got %v", err)
	}
}

func TestTableCursorEmptyRootAndMalformedRecord(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		data := make([]byte, 1024)
		p := data[512:]
		p[0] = TableLeafPage
		binary.BigEndian.PutUint16(p[5:], 500)
		if malformed {
			binary.BigEndian.PutUint16(p[3:], 1)
			binary.BigEndian.PutUint16(p[8:], 500)
			copy(p[500:], []byte{1, 7, 0}) // Complete cell, invalid zero-sized record header.
		}
		c, err := cursorTestDB(t, data, 0).NewTableCursor(2)
		if err != nil {
			t.Fatal(err)
		}
		_, first := c.Next()
		if malformed {
			if first == nil || !strings.Contains(first.Error(), "page 2: cell 0: rowId 7: decode record") {
				t.Fatalf("record error: %v", first)
			}
		} else if first != io.EOF || !c.exhausted {
			t.Fatalf("empty root: %v", first)
		}
		if _, second := c.Next(); second != first {
			t.Fatalf("changed terminal error: %v", second)
		}
	}
}
