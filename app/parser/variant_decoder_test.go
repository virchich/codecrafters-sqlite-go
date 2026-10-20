package parser

import (
	"errors"
	"io"
	"testing"
)

func TestDecodeVariant(t *testing.T) {
	tests := []struct {
		name string
		buf  []byte
		off  int
		want uint64
		n    int
	}{
		{"zero", []byte{0}, 0, 0, 1},
		{"single byte", []byte{5}, 0, 5, 1},
		{"single byte maximum", []byte{0x7f}, 0, 127, 1},
		{"two bytes", []byte{0x81, 0x00}, 0, 128, 2},
		{"two byte maximum", []byte{0xff, 0x7f}, 0, 16383, 2},
		{"three bytes", []byte{0x81, 0x80, 0x00}, 0, 16384, 3},
		{"offset and trailing bytes", []byte{0xff, 0x81, 0x00, 0xff}, 1, 128, 2},
		{"stops at first byte", []byte{5, 0xff}, 0, 5, 1},
		{"eight byte maximum", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}, 0, (uint64(1) << 56) - 1, 8},
		{"nine byte minimum", []byte{0x80, 0xc0, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00}, 0, uint64(1) << 56, 9},
		{"ninth byte high bit is data", []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}, 0, 128, 9},
		{"maximum and trailing byte", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, 0, ^uint64(0), 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n, err := decodeVariant(tt.buf, tt.off)
			if err != nil || got != tt.want || n != tt.n {
				t.Fatalf("got (%d, %d, %v), want (%d, %d, nil)", got, n, err, tt.want, tt.n)
			}
		})
	}
}

func TestDecodeVariantTruncated(t *testing.T) {
	// Every possible incomplete length, including an empty buffer.
	for length := 0; length < 9; length++ {
		buf := make([]byte, length)
		for i := range buf {
			buf[i] = 0x80
		}
		value, n, err := decodeVariant(buf, 0)
		if value != 0 || n != length || !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("length %d: got (%d, %d, %v)", length, value, n, err)
		}
	}
	if _, n, err := decodeVariant([]byte{1}, 1); n != 0 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("offset at end: got (%d, %v)", n, err)
	}
}

func TestDecodeVariantInvalidOffset(t *testing.T) {
	for _, off := range []int{-1, 2, int(^uint(0) >> 1)} {
		value, n, err := decodeVariant([]byte{0}, off)
		if value != 0 || n != 0 || err == nil {
			t.Fatalf("offset %d: got (%d, %d, %v)", off, value, n, err)
		}
	}
}
