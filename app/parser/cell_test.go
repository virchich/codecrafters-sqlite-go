package parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

type payloadCellResult struct {
	size     uint64
	payload  []byte
	overflow uint32
	child    uint32
}

var payloadCellParsers = []struct {
	name   string
	prefix []byte
	rowid  []byte
	parse  func([]byte, int, *DbHeader) (payloadCellResult, int, error)
}{
	{"table leaf", nil, []byte{7}, func(b []byte, off int, h *DbHeader) (payloadCellResult, int, error) {
		c, n, err := NewTableLeafCell(b, off, h)
		return payloadCellResult{c.PayloadSize, c.LocalPayload, c.OverflowPageNumber, 0}, n, err
	}},
	{"index leaf", nil, nil, func(b []byte, off int, h *DbHeader) (payloadCellResult, int, error) {
		c, n, err := NewIndexLeafCell(b, off, h)
		return payloadCellResult{c.PayloadSize, c.LocalPayload, c.OverflowPageNumber, 0}, n, err
	}},
	{"index interior", []byte{1, 2, 3, 4}, nil, func(b []byte, off int, h *DbHeader) (payloadCellResult, int, error) {
		c, n, err := NewIndexInteriorCell(b, off, h)
		return payloadCellResult{c.PayloadSize, c.LocalPayload, c.OverflowPageNumber, c.PageNumber}, n, err
	}},
}

func TestPayloadCellExtraction(t *testing.T) {
	for _, parser := range payloadCellParsers {
		for _, tc := range []struct {
			name                   string
			size                   []byte
			total                  uint64
			tableLocal, indexLocal int
			reserved               byte
		}{
			{"tiny", []byte{1}, 1, 1, 1, 0},
			{"opaque record bytes", []byte{3}, 3, 3, 3, 0},
			{"index threshold", []byte{102}, 102, 102, 102, 0},
			{"index overflow", []byte{103}, 103, 103, 39, 0},
			{"table threshold", []byte{0x83, 0x5d}, 477, 477, 39, 0},
			{"table overflow", []byte{0x83, 0x5e}, 478, 39, 39, 0},
			{"remainder retained", []byte{0x84, 0x58}, 600, 92, 92, 0},
			{"reserved bytes", []byte{0x84, 0x58}, 600, 124, 35, 32},
		} {
			t.Run(parser.name+"/"+tc.name, func(t *testing.T) {
				local := tc.indexLocal
				if parser.name == "table leaf" {
					local = tc.tableLocal
				}
				header := append([]byte{}, parser.prefix...)
				header = append(header, tc.size...)
				header = append(header, parser.rowid...)
				payload := bytes.Repeat([]byte{0xab}, local)
				cell := append(header, payload...)
				var overflow uint32
				if uint64(local) < tc.total {
					overflow = 0x05060708
					cell = binary.BigEndian.AppendUint32(cell, overflow)
				}
				// Nonzero cell offset; trailing bytes must not become payload or a pointer.
				buf := append([]byte{99, 99}, cell...)
				buf = append(buf, 88, 88, 88, 88)
				got, n, err := parser.parse(buf, 2, &DbHeader{PageSize: 512, ReservedBytes: tc.reserved})
				if err != nil || n != len(cell) || got.size != tc.total ||
					got.overflow != overflow || !bytes.Equal(got.payload, payload) {
					t.Fatalf("cell=%+v bytes=%d err=%v; want local=%d bytes=%d overflow=%x", got, n, err, local, len(cell), overflow)
				}
				if parser.name == "index interior" && got.child != 0x01020304 {
					t.Fatalf("child=%x", got.child)
				}
			})
		}
	}
}

func TestPayloadCellTruncation(t *testing.T) {
	for _, parser := range payloadCellParsers {
		t.Run(parser.name, func(t *testing.T) {
			h := &DbHeader{PageSize: 512}
			for _, off := range []int{-1, 2} {
				if _, n, err := parser.parse([]byte{0}, off, h); err == nil || n != 0 {
					t.Fatalf("offset %d: bytes=%d err=%v", off, n, err)
				}
			}
			if _, n, err := parser.parse([]byte{0}, 1, h); !errors.Is(err, io.ErrUnexpectedEOF) || n != 0 {
				t.Fatalf("end offset: bytes=%d err=%v", n, err)
			}
			// A 478-byte payload has 39 local bytes for all three cell types.
			header := append([]byte{}, parser.prefix...)
			header = append(header, 0x83, 0x5e)
			header = append(header, parser.rowid...)
			local := append(header, bytes.Repeat([]byte{0xab}, 39)...)
			for missing := 1; missing <= 4; missing++ {
				buf := append(append([]byte{}, local...), make([]byte, 4-missing)...)
				_, n, err := parser.parse(buf, 0, h)
				if !errors.Is(err, io.ErrUnexpectedEOF) || n != len(local) {
					t.Errorf("missing %d pointer bytes: bytes=%d err=%v; want %d", missing, n, err, len(local))
				}
			}
			_, n, err := parser.parse(local[:len(local)-1], 0, h)
			if !errors.Is(err, io.ErrUnexpectedEOF) || n != len(header) {
				t.Errorf("truncated local payload: bytes=%d err=%v", n, err)
			}
		})
	}
}

func TestLocalPayloadLen(t *testing.T) {
	for _, tc := range []struct {
		name     string
		size     uint64
		reserved byte
		table    bool
		want     uint64
	}{
		{"empty payload", 0, 0, true, 0},
		{"table local threshold", 477, 0, true, 477},
		{"table overflow threshold", 478, 0, true, 39},
		{"index local threshold", 102, 0, false, 102},
		{"index overflow threshold", 103, 0, false, 39},
		{"table remainder", 600, 0, true, 92},
		{"index remainder", 600, 0, false, 92},
		{"table reserved space", 600, 32, true, 124},
		{"index reserved space", 600, 32, false, 35},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := localPayloadLen(tc.size, 512, tc.reserved, tc.table); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// An index leaf has no child pointer and can occupy fewer than four bytes.
func TestIndexLeafCellAtBufferEnd(t *testing.T) {
	cell, n, err := NewIndexLeafCell([]byte{99, 1, 1}, 1, &DbHeader{PageSize: 512})
	if err != nil || n != 2 || cell.PayloadSize != 1 ||
		!bytes.Equal(cell.LocalPayload, []byte{1}) || cell.OverflowPageNumber != 0 {
		t.Fatalf("cell=%+v bytes=%d err=%v", cell, n, err)
	}
}

// Extra page bytes must not change the index-specific local payload length.
func TestIndexInteriorCellOverflowWithPagePadding(t *testing.T) {
	buf := make([]byte, 512)
	binary.BigEndian.PutUint32(buf, 3)
	buf[4] = 103
	copy(buf[5:44], bytes.Repeat([]byte{0xab}, 39))
	binary.BigEndian.PutUint32(buf[44:48], 9)
	cell, n, err := NewIndexInteriorCell(buf, 0, &DbHeader{PageSize: 512})
	if err != nil || n != 48 || cell.PayloadSize != 103 ||
		len(cell.LocalPayload) != 39 || cell.OverflowPageNumber != 9 {
		t.Fatalf("cell=%+v bytes=%d err=%v", cell, n, err)
	}
}
