package parser

import (
	"encoding/binary"
	"errors"
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
		page, err := NewPage(pageNum, 512)
		if err != nil {
			t.Fatal(err)
		}
		for repeat := 0; repeat < 2; repeat++ {
			if err := page.Parse(f); err != nil {
				t.Fatal(err)
			}
			for _, want := range []int64{7, 8} {
				cell, err := page.GetNextTableLeafCell()
				if err != nil || cell.Rowid != want {
					t.Fatalf("page %d: got rowid %d, err %v; want %d", pageNum, cell.Rowid, err, want)
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
	p, _ := NewPage(2, 512)
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
		page, err := NewPage(pageNum, 512)
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
			for _, want := range []TableInteriorCell{{PageNumber: 3, Rowid: 57}, {PageNumber: 7, Rowid: 128}} {
				got, err := page.GetNextTableInteriorCall()
				if err != nil || got != want {
					t.Fatalf("page %d: got %+v, %v; want %+v", pageNum, got, err, want)
				}
			}
			for i := 0; i < 2; i++ {
				if _, err := page.GetNextTableInteriorCall(); !errors.Is(err, io.EOF) {
					t.Fatalf("expected EOF: %v", err)
				}
			}
		}
	}
}

func TestInteriorPageInvalidPointers(t *testing.T) {
	for _, pointer := range []uint16{0, 12, 479, 510, 512, 65535} {
		page, _ := NewPage(2, 512)
		page.Header.PageType = TableInteriorPage
		page.Header.CellContentAreaStart = 480
		page.cellPointer = []uint16{pointer}
		if _, err := page.GetNextTableInteriorCall(); err == nil {
			t.Errorf("accepted pointer %d", pointer)
		}
		if page.cellIdx != 0 {
			t.Error("failed read advanced iterator")
		}
	}
	page, _ := NewPage(2, 512)
	page.Header.PageType = TableLeafPage
	if _, err := page.GetNextTableInteriorCall(); err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("wrong page type: %v", err)
	}
}
