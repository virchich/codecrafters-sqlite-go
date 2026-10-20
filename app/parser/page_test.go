package parser

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPageCellIteration(t *testing.T) {
	for _, pageNum := range []uint32{1, 2} {
		data := make([]byte, int(pageNum)*512)
		pageBuf := data[(int(pageNum)-1)*512:]
		start := 0
		if pageNum == 1 {
			start = 100
		}
		copy(pageBuf[start:], []byte{13, 0, 0, 0, 2, 1, 224, 0})
		// Pointer order deliberately differs from physical cell order.
		binary.BigEndian.PutUint16(pageBuf[start+8:], 500)
		binary.BigEndian.PutUint16(pageBuf[start+10:], 480)
		copy(pageBuf[500:], []byte{3, 7, 2, 15, 'a'})
		copy(pageBuf[480:], []byte{3, 8, 2, 15, 'b'})
		path := filepath.Join(t.TempDir(), "test.db")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		page, err := NewPage(pageNum, &DbHeader{PageSize: 512})
		if err != nil {
			t.Fatal(err)
		}
		for repeat := 0; repeat < 2; repeat++ {
			if err := page.Parse(f); err != nil {
				t.Fatal(err)
			}
			for _, want := range []uint64{7, 8} {
				cell, err := page.GetNextTableLeafCell()
				if err != nil || cell.RowId != want {
					t.Fatalf("page %d: got rowid %d, err %v; want %d", pageNum, cell.RowId, err, want)
				}
			}
			for i := 0; i < 2; i++ {
				if _, err := page.GetNextTableLeafCell(); !errors.Is(err, io.EOF) {
					t.Fatalf("expected EOF, got %v", err)
				}
			}
		}
	}
}

func TestPageIterationBounds(t *testing.T) {
	p, _ := NewPage(2, &DbHeader{PageSize: 512})
	p.Header.PageType = TableLeafPage
	if _, err := p.GetNextTableLeafCell(); !errors.Is(err, io.EOF) {
		t.Fatalf("empty page: %v", err)
	}
	for _, pointer := range []uint16{0, 9, 512, 65535} {
		p.cellPointer = []uint16{pointer}
		if _, err := p.GetNextTableLeafCell(); err == nil {
			t.Fatalf("accepted pointer %d", pointer)
		}
	}
	p.Header.PageType = TableInteriorPage
	if _, err := p.GetNextTableLeafCell(); err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("expected unsupported page error, got %v", err)
	}
	for _, off := range []int{-1, 512, 513} {
		if _, err := p.getCellPointerArray(p.buf, off, 1); err == nil {
			t.Fatalf("accepted truncated array at %d", off)
		}
	}
}

func TestInteriorPageIteration(t *testing.T) {
	for _, pageNum := range []uint32{1, 2} {
		data := make([]byte, int(pageNum)*512)
		buf := data[(pageNum-1)*512:]
		start := 0
		if pageNum == 1 {
			start = 100
		}
		copy(buf[start:], []byte{5, 0, 0, 0, 2, 1, 224, 0, 0, 0, 0, 9})
		binary.BigEndian.PutUint16(buf[start+12:], 500)
		binary.BigEndian.PutUint16(buf[start+14:], 480)
		copy(buf[500:], []byte{0, 0, 0, 3, 57})
		copy(buf[480:], []byte{0, 0, 0, 7, 0x81, 0})
		path := filepath.Join(t.TempDir(), "interior.db")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		page, err := NewPage(pageNum, &DbHeader{PageSize: 512})
		if err != nil {
			t.Fatal(err)
		}
		for repeat := 0; repeat < 2; repeat++ {
			if err := page.Parse(f); err != nil {
				t.Fatal(err)
			}
			if page.Header.RightMostPointer != 9 {
				t.Fatal("lost rightmost child")
			}
			for _, want := range []TableInteriorCell{{PageNumber: 3, RowId: 57}, {PageNumber: 7, RowId: 128}} {
				got, err := page.GetNextTableInteriorCell()
				if err != nil || got != want {
					t.Fatalf("page %d: got %+v, %v; want %+v", pageNum, got, err, want)
				}
			}
			for i := 0; i < 2; i++ {
				if _, err := page.GetNextTableInteriorCell(); !errors.Is(err, io.EOF) {
					t.Fatalf("expected EOF: %v", err)
				}
			}
		}
	}
}

func TestInteriorPageInvalidPointers(t *testing.T) {
	for _, pointer := range []uint16{0, 12, 479, 510, 512, 65535} {
		page, _ := NewPage(2, &DbHeader{PageSize: 512})
		page.Header.PageType = TableInteriorPage
		page.Header.CellContentAreaStart = 480
		page.cellPointer = []uint16{pointer}
		if _, err := page.GetNextTableInteriorCell(); err == nil {
			t.Errorf("accepted pointer %d", pointer)
		}
		if page.cellIdx != 0 {
			t.Error("failed read advanced iterator")
		}
	}
	page, _ := NewPage(2, &DbHeader{PageSize: 512})
	page.Header.PageType = TableLeafPage
	if _, err := page.GetNextTableInteriorCell(); err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("wrong page type: %v", err)
	}
}

func TestCurrentCellOffset(t *testing.T) {
	for _, pageType := range []uint8{TableLeafPage, TableInteriorPage, IndexLeafPage, IndexInteriorPage} {
		for _, pageNum := range []uint32{1, 2} {
			t.Run(fmt.Sprintf("type_%x/page_%d", pageType, pageNum), func(t *testing.T) {
				p, err := NewPage(pageNum, &DbHeader{PageSize: 512})
				if err != nil {
					t.Fatal(err)
				}
				p.Header.PageType = pageType
				headerEnd := 8
				if pageType == TableInteriorPage || pageType == IndexInteriorPage {
					headerEnd = 12
				}
				if pageNum == 1 {
					headerEnd += 100
				}
				// Two pointers occupy four bytes after the appropriate header.
				firstValid := headerEnd + 4
				p.cellPointer = []uint16{uint16(firstValid), 500}
				p.Header.CellContentAreaStart = uint32(firstValid)
				for _, tc := range []struct {
					name         string
					pointer      uint16
					contentStart uint32
					valid        bool
				}{
					{"pointer array", uint16(firstValid - 1), uint32(firstValid - 1), false},
					{"first valid byte", uint16(firstValid), uint32(firstValid), true},
					{"before content", uint16(firstValid), uint32(firstValid + 1), false},
					{"last byte", 511, uint32(firstValid), true},
					{"past page", 512, uint32(firstValid), false},
					{"large pointer", 65535, uint32(firstValid), false},
				} {
					t.Run(tc.name, func(t *testing.T) {
						p.cellPointer[0] = tc.pointer
						p.Header.CellContentAreaStart = tc.contentStart
						got, err := p.currentCellOffset(pageType)
						if tc.valid {
							if err != nil || got != int(tc.pointer) {
								t.Fatalf("got (%d, %v)", got, err)
							}
						} else if err == nil || errors.Is(err, io.EOF) {
							t.Fatalf("expected invalid pointer error, got %v", err)
						}
						if p.cellIdx != 0 {
							t.Fatal("helper advanced cursor")
						}
					})
				}
				p.cellIdx = 1
				p.Header.CellContentAreaStart = uint32(firstValid)
				if got, err := p.currentCellOffset(pageType); err != nil || got != 500 || p.cellIdx != 1 {
					t.Fatalf("current cell: got (%d, %v), cursor %d", got, err, p.cellIdx)
				}
				p.cellIdx = len(p.cellPointer)
				if _, err := p.currentCellOffset(pageType); err != io.EOF {
					t.Fatalf("expected EOF, got %v", err)
				}
				p.Header.PageType = 0
				if _, err := p.currentCellOffset(pageType); err == nil || errors.Is(err, io.EOF) {
					t.Fatalf("page type must be checked before EOF: %v", err)
				}
			})
		}
	}
}

func TestPageCellReaderCursor(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pageType uint8
		cell     []byte
		next     func(*Page) error
	}{
		{"table leaf", TableLeafPage, []byte{1, 7, 1}, func(p *Page) error { _, err := p.GetNextTableLeafCell(); return err }},
		{"table interior", TableInteriorPage, []byte{0, 0, 0, 3, 7}, func(p *Page) error { _, err := p.GetNextTableInteriorCell(); return err }},
		{"index leaf", IndexLeafPage, []byte{3, 2, 15, 'a'}, func(p *Page) error { _, err := p.GetNextIndexLeafCell(); return err }},
		{"index interior", IndexInteriorPage, []byte{0, 0, 0, 3, 3, 2, 15, 'a'}, func(p *Page) error { _, err := p.GetNextIndexInteriorCell(); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewPage(2, &DbHeader{PageSize: 512})
			if err != nil {
				t.Fatal(err)
			}
			p.Header.PageType = tc.pageType
			p.Header.CellContentAreaStart = 480
			p.cellPointer = []uint16{511}
			p.buf[511] = 0x80
			if err := tc.next(&p); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("expected truncation, got %v", err)
			}
			if p.cellIdx != 0 {
				t.Fatal("failed parsing advanced cursor")
			}
			p.cellPointer[0] = 480
			copy(p.buf[480:], tc.cell)
			if err := tc.next(&p); err != nil {
				t.Fatal(err)
			}
			if p.cellIdx != 1 {
				t.Fatalf("successful parsing left cursor at %d", p.cellIdx)
			}
			for i := 0; i < 2; i++ {
				if err := tc.next(&p); err != io.EOF {
					t.Fatalf("expected EOF, got %v", err)
				}
				if p.cellIdx != 1 {
					t.Fatal("EOF advanced cursor")
				}
			}
		})
	}
}

func TestPageReadFailureClearsIteration(t *testing.T) {
	p, err := NewPage(2, &DbHeader{PageSize: 512})
	if err != nil {
		t.Fatal(err)
	}
	p.Header = PageHeader{PageType: TableLeafPage}
	p.cellPointer = []uint16{500}
	p.cellIdx = 1
	if err := p.Parse(parserTestFile(t, make([]byte, 600))); !errors.Is(err, io.EOF) {
		t.Fatalf("read error: %v", err)
	}
	if p.cellIdx != 0 || len(p.cellPointer) != 0 || p.Header != (PageHeader{}) {
		t.Fatalf("stale iteration state: %+v", p)
	}
}
