package parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestPageConstructorsRejectInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		number uint32
		header *DbHeader
	}{
		{"zero page", 0, &DbHeader{PageSize: 512}},
		{"missing header", 1, nil},
		{"small page", 1, &DbHeader{PageSize: 256}},
		{"non power of two", 1, &DbHeader{PageSize: 513}},
		{"large page", 1, &DbHeader{PageSize: 131072}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewPage(tc.number, tc.header); err == nil {
				t.Error("NewPage accepted invalid input")
			}
			if _, err := NewOverflowPage(tc.number, tc.header); err == nil {
				t.Error("NewOverflowPage accepted invalid input")
			}
		})
	}
}

func TestOverflowPageRead(t *testing.T) {
	for _, reserved := range []byte{0, 32} {
		data := make([]byte, 1024)
		binary.BigEndian.PutUint32(data[512:], 0x01020304)
		payload := bytes.Repeat([]byte{0xab}, 508-int(reserved))
		copy(data[516:], payload)
		f := parserTestFile(t, data)
		if _, err := f.Seek(17, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		p, err := NewOverflowPage(2, &DbHeader{PageSize: 512, ReservedBytes: reserved})
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Parse(f); err != nil {
			t.Fatal(err)
		}
		if p.NextPage != 0x01020304 || !bytes.Equal(p.Payload, payload) {
			t.Errorf("reserved=%d: next=%x payload length=%d, want %d", reserved, p.NextPage, len(p.Payload), len(payload))
		}
		pos, err := f.Seek(0, io.SeekCurrent)
		if err != nil || pos != 17 {
			t.Fatalf("file position=%d err=%v", pos, err)
		}
	}
}

func TestOverflowPageReadTruncated(t *testing.T) {
	p, err := NewOverflowPage(2, &DbHeader{PageSize: 512})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Parse(parserTestFile(t, make([]byte, 600))); !errors.Is(err, io.EOF) {
		t.Fatalf("truncated page: %v", err)
	}
}
