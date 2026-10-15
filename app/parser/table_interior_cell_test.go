package parser

import (
	"errors"
	"io"
	"testing"
)

func TestTableInteriorCell(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  []byte
		want uint64
	}{
		{"one byte", []byte{57}, 57},
		{"multiple bytes", []byte{0x81, 0}, 128},
		{"nine bytes", []byte{255, 255, 255, 255, 255, 255, 255, 255, 255}, ^uint64(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A nonzero offset catches confusing a slice's end index with its length.
			buf := append([]byte{99, 99, 99, 1, 2, 3, 4}, tc.key...)
			buf = append(buf, 88)
			cell, n, err := NewTableInteriorCell(buf, 3)
			if err != nil || cell.PageNumber != 0x01020304 || cell.Rowid != tc.want || n != 4+len(tc.key) {
				t.Fatalf("cell=%+v bytes=%d err=%v", cell, n, err)
			}
		})
	}
}

func TestTableInteriorCellTruncation(t *testing.T) {
	for _, tc := range []struct {
		buf           []byte
		off, consumed int
	}{
		{nil, 0, 0}, {[]byte{0, 0, 0}, 0, 0},
		{[]byte{0, 0, 0, 4}, 0, 4}, {[]byte{0, 0, 0, 4, 0x80}, 0, 5},
		{[]byte{9, 0, 0, 0}, 1, 0},
	} {
		_, n, err := NewTableInteriorCell(tc.buf, tc.off)
		if !errors.Is(err, io.ErrUnexpectedEOF) || n != tc.consumed {
			t.Errorf("%v at %d: bytes=%d err=%v", tc.buf, tc.off, n, err)
		}
	}
	for _, off := range []int{-1, 6} {
		if _, _, err := NewTableInteriorCell(make([]byte, 5), off); err == nil {
			t.Fatalf("accepted offset %d", off)
		}
	}
}
