package parser

import (
	"bytes"
	"encoding/binary"
	"io"
	"reflect"
	"strings"
	"testing"
)

// Build pages directly so tests control which entries live in interior pages.
func indexTestPage(data []byte, number uint32, typ byte, right uint32, cells ...[]byte) {
	p := data[(number-1)*512 : number*512]
	p[0] = typ
	binary.BigEndian.PutUint16(p[3:], uint16(len(cells)))
	header := 8
	if typ == IndexInteriorPage {
		header = 12
		binary.BigEndian.PutUint32(p[8:], right)
	}
	off := 512
	for i, cell := range cells {
		off -= len(cell)
		copy(p[off:], cell)
		binary.BigEndian.PutUint16(p[header+2*i:], uint16(off))
	}
	binary.BigEndian.PutUint16(p[5:], uint16(off))
}

func indexTestCell(value, rowid byte, child uint32) []byte {
	// Record: two one-byte integers, indexed value followed by table rowid.
	cell := []byte{5, 3, 1, 1, value, rowid}
	if child != 0 {
		prefix := make([]byte, 4)
		binary.BigEndian.PutUint32(prefix, child)
		cell = append(prefix, cell...)
	}
	return cell
}

func assertIndexScan(t *testing.T, c *IndexCursor, want [][2]int64) {
	t.Helper()
	for i, pair := range want {
		r, err := c.Next()
		if err != nil || !reflect.DeepEqual(r.Record.Body, []any{pair[0], pair[1]}) {
			t.Fatalf("entry %d: body=%v err=%v want=%v", i, r.Record.Body, err, pair)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := c.Next(); err != io.EOF || !c.exhausted {
			t.Fatalf("after scan: err=%v exhausted=%v", err, c.exhausted)
		}
	}
}

func TestIndexCursorLeafRoot(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "entries", true: "empty"}[empty], func(t *testing.T) {
			data := make([]byte, 2*512)
			var cells [][]byte
			var want [][2]int64
			if !empty {
				cells = [][]byte{indexTestCell(10, 2, 0), indexTestCell(10, 7, 0), indexTestCell(20, 3, 0)}
				want = [][2]int64{{10, 2}, {10, 7}, {20, 3}}
			}
			indexTestPage(data, 2, IndexLeafPage, 0, cells...)
			c, err := NewIndexCursor(2, cursorTestDB(t, data, 0))
			if err != nil {
				t.Fatal(err)
			}
			assertIndexScan(t, &c, want)
		})
	}
}

func TestIndexCursorNestedTraversal(t *testing.T) {
	data := make([]byte, 9*512)
	// Finishing page 5 must expose page 3's key; finishing page 6 must
	// pop page 3 and expose the root's key before entering page 7.
	indexTestPage(data, 2, IndexInteriorPage, 9, indexTestCell(20, 4, 3), indexTestCell(40, 8, 7))
	indexTestPage(data, 3, IndexInteriorPage, 6, indexTestCell(10, 2, 4), indexTestCell(15, 3, 5))
	indexTestPage(data, 4, IndexLeafPage, 0, indexTestCell(10, 1, 0))
	indexTestPage(data, 5, IndexLeafPage, 0) // An empty subtree must not end the scan.
	indexTestPage(data, 6, IndexLeafPage, 0, indexTestCell(20, 1, 0))
	indexTestPage(data, 7, IndexInteriorPage, 8) // Rightmost-only interior page.
	indexTestPage(data, 8, IndexLeafPage, 0, indexTestCell(30, 6, 0))
	indexTestPage(data, 9, IndexLeafPage, 0, indexTestCell(50, 9, 0))
	c, err := NewIndexCursor(2, cursorTestDB(t, data, 0))
	if err != nil {
		t.Fatal(err)
	}
	assertIndexScan(t, &c, [][2]int64{{10, 1}, {10, 2}, {15, 3}, {20, 1}, {20, 4}, {30, 6}, {40, 8}, {50, 9}})
}

func TestIndexCursorOverflow(t *testing.T) {
	for _, interior := range []bool{false, true} {
		for _, broken := range []bool{false, true} {
			name := map[bool]string{false: "leaf", true: "interior"}[interior] + map[bool]string{false: "/valid", true: "/missing"}[broken]
			t.Run(name, func(t *testing.T) {
				data := make([]byte, 6*512)
				// 1000 bytes: record header, 996-byte BLOB, and one-byte rowid.
				payload := append([]byte{4, 0x8f, 0x54, 1}, bytes.Repeat([]byte{0xab}, 996)...)
				payload = append(payload, 7)
				// P=1001, U=512: index payload has 39 bytes locally.
				cell := append([]byte{0x87, 0x69}, payload[:39]...)
				overflow := make([]byte, 4)
				if !broken {
					binary.BigEndian.PutUint32(overflow, 3)
				}
				cell = append(cell, overflow...)
				typ := byte(IndexLeafPage)
				if interior {
					typ = IndexInteriorPage
					prefix := make([]byte, 4)
					binary.BigEndian.PutUint32(prefix, 5)
					cell = append(prefix, cell...)
					indexTestPage(data, 5, IndexLeafPage, 0)
					indexTestPage(data, 6, IndexLeafPage, 0)
				}
				indexTestPage(data, 2, typ, 6, cell)
				binary.BigEndian.PutUint32(data[1024:], 4)
				copy(data[1028:1536], payload[39:547])
				copy(data[1540:], payload[547:])
				c, err := NewIndexCursor(2, cursorTestDB(t, data, 0))
				if err != nil {
					t.Fatal(err)
				}
				r, err := c.Next()
				if broken {
					if err == nil || !strings.Contains(err.Error(), "page 2: cell 0: assemble payload") {
						t.Fatalf("missing contextual error: %v", err)
					}
					if _, again := c.Next(); again != err {
						t.Fatalf("failure changed: %v", again)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(r.Record.Body) != 2 || !bytes.Equal(r.Record.Body[0].([]byte), payload[4:1000]) || r.Record.Body[1] != int64(7) {
					t.Fatalf("unexpected record: %v", r.Record.Body)
				}
				assertIndexScan(t, &c, nil)
			})
		}
	}
}

func TestIndexCursorInvalidInitialization(t *testing.T) {
	for _, tc := range []struct {
		root uint32
		db   *DB
	}{{0, &DB{}}, {2, nil}, {2, &DB{}}} {
		if _, err := NewIndexCursor(tc.root, tc.db); err == nil {
			t.Fatal("accepted invalid initialization")
		}
	}
	var c IndexCursor
	_, first := c.Next()
	if first == nil {
		t.Fatal("accepted uninitialized cursor")
	}
	if _, again := c.Next(); again != first {
		t.Fatalf("failure changed: %v", again)
	}
	for _, cycle := range []bool{false, true} {
		data := make([]byte, 2*512)
		if cycle {
			indexTestPage(data, 2, IndexInteriorPage, 2)
		} else {
			indexTestPage(data, 2, TableLeafPage, 0)
		}
		if _, err := NewIndexCursor(2, cursorTestDB(t, data, 0)); err == nil {
			t.Fatalf("accepted invalid tree (cycle=%v)", cycle)
		}
	}
}

func TestIndexCursorMalformedRecord(t *testing.T) {
	for _, interior := range []bool{false, true} {
		data := make([]byte, 4*512)
		cell := []byte{1, 0} // Complete payload with an invalid record header.
		typ := byte(IndexLeafPage)
		if interior {
			typ = IndexInteriorPage
			cell = append([]byte{0, 0, 0, 3}, cell...)
			indexTestPage(data, 3, IndexLeafPage, 0)
			indexTestPage(data, 4, IndexLeafPage, 0)
		}
		indexTestPage(data, 2, typ, 4, cell)
		c, err := NewIndexCursor(2, cursorTestDB(t, data, 0))
		if err != nil {
			t.Fatal(err)
		}
		_, first := c.Next()
		if first == nil || !strings.Contains(first.Error(), "page 2: cell 0: decode record") {
			t.Fatalf("unexpected error: %v", first)
		}
		if _, again := c.Next(); again != first {
			t.Fatalf("failure changed: %v", again)
		}
	}
}
