package parser

import (
	"bytes"
	"testing"
)

func TestNewTableLeafCell(t *testing.T) {
	payload := []byte{3, 23, 1, 'a', 'p', 'p', 'l', 'e', 42}
	buf := append([]byte{99, 9, 7}, payload...)
	buf = append(buf, 99)
	got, n, err := NewTableLeafCell(buf, 1, &DbHeader{PageSize: 512})
	if err != nil || n != 11 || got.PayloadSize != 9 || got.RowId != 7 ||
		got.OverflowPageNumber != 0 || !bytes.Equal(got.LocalPayload, payload) {
		t.Fatalf("cell=%+v bytes=%d err=%v", got, n, err)
	}
}

func TestTableLeafCellNineByteRowID(t *testing.T) {
	buf := []byte{1, 255, 255, 255, 255, 255, 255, 255, 255, 255, 1}
	cell, n, err := NewTableLeafCell(buf, 0, &DbHeader{PageSize: 512})
	if err != nil || n != len(buf) || cell.RowId != ^uint64(0) {
		t.Fatalf("cell=%+v bytes=%d err=%v", cell, n, err)
	}
}
