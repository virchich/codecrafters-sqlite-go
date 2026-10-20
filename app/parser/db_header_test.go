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

func parserTestFile(t *testing.T, data []byte) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.db")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func databaseHeaderFixture(encodedSize uint16, reserved byte) []byte {
	b := make([]byte, 100)
	copy(b, "SQLite format 3\x00")
	binary.BigEndian.PutUint16(b[16:], encodedSize)
	b[18], b[19], b[20] = 2, 1, reserved
	return b
}

func TestDbHeaderFields(t *testing.T) {
	for _, tc := range []struct {
		encoded  uint16
		reserved byte
		size     uint32
	}{
		{512, 0, 512}, {512, 32, 512}, {4096, 255, 4096}, {32768, 0, 32768}, {1, 0, 65536},
	} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			var h DbHeader
			err := h.Parse(parserTestFile(t, databaseHeaderFixture(tc.encoded, tc.reserved)))
			want := DbHeader{MagicHeaderString: "SQLite format 3\x00", PageSize: tc.size, FileFormatWriteVersion: 2, FileFormatReadVersion: 1, ReservedBytes: tc.reserved}
			if err != nil || h != want {
				t.Fatalf("header=%+v err=%v want=%+v", h, err, want)
			}
		})
	}
}

func TestDbHeaderInvalidSizes(t *testing.T) {
	for _, tc := range []struct {
		size     uint16
		reserved byte
	}{
		{0, 1}, {256, 0}, {513, 0}, {65535, 0}, {512, 33},
	} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			var h DbHeader
			if err := h.Parse(parserTestFile(t, databaseHeaderFixture(tc.size, tc.reserved))); err == nil {
				t.Fatal("accepted invalid size")
			}
		})
	}
}

func TestDbHeaderTruncated(t *testing.T) {
	for _, size := range []int{0, 1, 99} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			var h DbHeader
			err := h.Parse(parserTestFile(t, make([]byte, size)))
			want := io.ErrUnexpectedEOF
			if size == 0 {
				want = io.EOF
			}
			if !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
		})
	}
}
