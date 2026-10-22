package parser

import (
	"encoding/binary"
	"io"
	"testing"
)

func TestLookupTableByRowID(t *testing.T) {
	data := make([]byte, 4*512)
	root := data[512:1024]
	root[0] = TableInteriorPage
	binary.BigEndian.PutUint16(root[3:], 1)
	binary.BigEndian.PutUint16(root[5:], 500)
	binary.BigEndian.PutUint32(root[8:], 4)
	binary.BigEndian.PutUint16(root[12:], 500)
	binary.BigEndian.PutUint32(root[500:], 3)
	root[504] = 5
	for i, ids := range [][]byte{{2, 5}, {8, 12}} {
		p := data[(i+2)*512 : (i+3)*512]
		p[0] = TableLeafPage
		binary.BigEndian.PutUint16(p[3:], 2)
		binary.BigEndian.PutUint16(p[5:], 500)
		for j, id := range ids {
			off := 500 + j*5
			binary.BigEndian.PutUint16(p[8+j*2:], uint16(off))
			copy(p[off:], []byte{3, id, 2, 1, id})
		}
	}
	db := cursorTestDB(t, data, 0)
	// Repeated lookups exercise exact separator equality and rightmost routing.
	for _, id := range []int64{12, 2, 5, 8, 5} {
		row, err := db.LookupTableByRowID(2, id)
		if err != nil || row.RowID != id || len(row.Record.Body) != 1 || row.Record.Body[0] != id {
			t.Fatalf("id=%d row=%+v err=%v", id, row, err)
		}
	}
	for _, id := range []int64{-1, 0, 1, 3, 6, 9, 13} {
		if _, err := db.LookupTableByRowID(2, id); err != io.EOF {
			t.Fatalf("missing id=%d: %v", id, err)
		}
	}
}

func TestLookupTableSignedRowID(t *testing.T) {
	data := make([]byte, 3*512)
	root := data[512:1024]
	root[0] = TableInteriorPage
	binary.BigEndian.PutUint16(root[3:], 1)
	binary.BigEndian.PutUint16(root[5:], 490)
	binary.BigEndian.PutUint16(root[12:], 490)
	binary.BigEndian.PutUint32(root[490:], 3)
	// -1 encoded as a nine-byte SQLite varint. Rightmost pointer is invalid
	// deliberately: an unsigned comparison would select it for target zero.
	for i := 494; i < 503; i++ {
		root[i] = 255
	}
	leaf := data[1024:]
	leaf[0] = TableLeafPage
	binary.BigEndian.PutUint16(leaf[3:], 1)
	binary.BigEndian.PutUint16(leaf[5:], 490)
	binary.BigEndian.PutUint16(leaf[8:], 490)
	leaf[490] = 3
	for i := 491; i < 500; i++ {
		leaf[i] = 255
	}
	copy(leaf[500:], []byte{2, 1, 42})
	db := cursorTestDB(t, data, 0)
	r, err := db.LookupTableByRowID(2, -1)
	if err != nil || r.RowID != -1 {
		t.Fatalf("row=%+v err=%v", r, err)
	}
	if _, err := db.LookupTableByRowID(2, 0); err == nil || err == io.EOF {
		t.Fatalf("expected invalid rightmost pointer error, got %v", err)
	}
}

func TestLookupTableInvalidInputs(t *testing.T) {
	var absent *DB
	for _, db := range []*DB{absent, {}} {
		if _, err := db.LookupTableByRowID(2, 1); err == nil {
			t.Fatal("accepted missing database")
		}
	}
	for _, typ := range []byte{TableLeafPage, IndexLeafPage, TableInteriorPage} {
		data := make([]byte, 2*512)
		p := data[512:]
		p[0] = typ
		binary.BigEndian.PutUint16(p[5:], 512)
		if typ == TableInteriorPage {
			binary.BigEndian.PutUint32(p[8:], 2)
		}
		db := cursorTestDB(t, data, 0)
		_, err := db.LookupTableByRowID(2, 1)
		if typ == TableLeafPage {
			if err != io.EOF {
				t.Fatalf("empty leaf: %v", err)
			}
		} else if err == nil || err == io.EOF {
			t.Fatalf("invalid tree accepted: %v", err)
		}
		if _, err := db.LookupTableByRowID(0, 1); err == nil {
			t.Fatal("accepted zero root")
		}
	}
}
