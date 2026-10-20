package parser

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPageHeaderParse(t *testing.T) {
	for _, pageNum := range []uint32{1, 2} {
		for _, pageType := range []byte{0x02, 0x05, 0x0a, 0x0d} {
			start := 0
			if pageNum == 1 {
				start = 100
			}
			buf := make([]byte, start+12)
			copy(buf[start:], []byte{pageType, 0x01, 0x23, 0x00, 0x03, 0x0f, 0x80, 0x02, 0x01, 0x02, 0x03, 0x04})
			h := PageHeader{RightMostPointer: 99}
			if _, err := h.Parse(pageNum, buf); err != nil {
				t.Fatal(err)
			}
			want := PageHeader{PageType: pageType, FirstFreeblockStart: 291, NumOfCellsOnPage: 3, CellContentAreaStart: 3968, FragmentedFreeBytesNum: 2}
			if pageType == 0x02 || pageType == 0x05 {
				want.RightMostPointer = 0x01020304
			}
			if h != want {
				t.Fatalf("page %d type %x: got %+v, want %+v", pageNum, pageType, h, want)
			}
		}
	}
}

func TestPageHeaderBoundsAndSpecialOffset(t *testing.T) {
	var h PageHeader
	if _, err := h.Parse(2, []byte{13, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if h.CellContentAreaStart != 65536 {
		t.Fatal(h.CellContentAreaStart)
	}
	for _, buf := range [][]byte{nil, {13}, {5, 0, 0, 0, 0, 0, 0, 0}} {
		if _, err := h.Parse(2, buf); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("got %v", err)
		}
	}
	if _, err := h.Parse(2, make([]byte, 8)); err == nil {
		t.Fatal("accepted invalid page type")
	}
}

func TestPageReadIgnoresCurrentCursor(t *testing.T) {
	data := make([]byte, 1024)
	data[16], data[17] = 2, 0 // 512-byte pages
	copy(data[100:], []byte{13, 0, 0, 0, 3, 1, 240, 0})
	copy(data[512:], []byte{13, 0, 0, 0, 7, 1, 240, 0})
	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var header DbHeader
	if err := header.Parse(f); err != nil {
		t.Fatal(err)
	}
	for i, want := range []uint16{3, 7} {
		page, err := NewPage(uint32(i+1), &header)
		if err != nil {
			t.Fatal(err)
		}
		if err := page.Parse(f); err != nil {
			t.Fatal(err)
		}
		if page.Header.NumOfCellsOnPage != want {
			t.Fatalf("got %d, want %d", page.Header.NumOfCellsOnPage, want)
		}
	}
}
