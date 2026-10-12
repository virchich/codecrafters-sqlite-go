package parser

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"
)

func TestNewCell(t *testing.T) {
	// Payload: three-byte header, "apple", and integer 42.
	cell := []byte{9, 7, 3, 23, 1, 'a', 'p', 'p', 'l', 'e', 42}
	buf := append([]byte{99}, cell...)
	buf = append(buf, 99)
	got, n, err := NewCell(buf, 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(cell) || got.Size != 9 || got.Rowid != 7 || got.Record.Header.Size != 3 {
		t.Fatalf("unexpected cell/count: %+v, %d", got, n)
	}
	if !reflect.DeepEqual(got.Record.Header.SerialTypes, []SerialType{23, Int8}) ||
		!reflect.DeepEqual(got.Record.Body.Value, []any{"apple", int64(42)}) {
		t.Fatalf("unexpected record: %+v", got.Record)
	}
}

func TestNewCellNegativeRowID(t *testing.T) {
	buf := []byte{1, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 1}
	cell, n, err := NewCell(buf, 0)
	if err != nil || n != len(buf) || cell.Rowid != -1 {
		t.Fatalf("got (%+v, %d, %v)", cell, n, err)
	}
}

func TestRecordIntegers(t *testing.T) {
	for _, tc := range []struct {
		st    SerialType
		width int
	}{{Int8, 1}, {Int16, 2}, {Int24, 3}, {Int32, 4}, {Int48, 6}, {Int64, 8}} {
		bits := uint(tc.width * 8)
		min := int64(-1) << (bits - 1)
		max := int64((uint64(1) << (bits - 1)) - 1)
		for _, want := range []int64{0, 42, -1, -42, min, max} {
			var encoded [8]byte
			binary.BigEndian.PutUint64(encoded[:], uint64(want))
			body, n, err := readRecordBody(encoded[8-tc.width:], 0, []SerialType{tc.st})
			if err != nil || n != tc.width || body.Value[0] != want {
				t.Fatalf("type %d value %d: got (%+v, %d, %v)", tc.st, want, body, n, err)
			}
		}
	}
}

func TestRecordFloats(t *testing.T) {
	for _, bits := range []uint64{0, 0x8000000000000000, math.Float64bits(1.5), math.Float64bits(-42.25), 0x7ff0000000000000, 0x7ff8000000000001} {
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], bits)
		body, n, err := readRecordBody(buf[:], 0, []SerialType{Float64})
		if err != nil || n != 8 {
			t.Fatalf("got (%d, %v)", n, err)
		}
		if got := math.Float64bits(body.Value[0].(float64)); got != bits {
			t.Fatalf("got bits %x, want %x", got, bits)
		}
	}
}

func TestRecordSpecialValues(t *testing.T) {
	buf := []byte{0, 0xff, 'h', 'i'}
	types := []SerialType{Null, IntegerZero, IntegerOne, BlobBase, TextBase, 16, 17}
	body, n, err := readRecordBody(buf, 0, types)
	want := []any{nil, int64(0), int64(1), []byte{}, "", []byte{0, 0xff}, "hi"}
	if err != nil || n != len(buf) || !reflect.DeepEqual(body.Value, want) {
		t.Fatalf("got (%+v, %d, %v)", body, n, err)
	}
	buf[0] = 123
	if !reflect.DeepEqual(body.Value[5], []byte{0, 0xff}) {
		t.Fatal("BLOB aliases input buffer")
	}
}

func TestRecordMultibyteVarints(t *testing.T) {
	// Header size 130 encoded in two bytes, then 128 NULL serial types.
	header := append([]byte{0x81, 0x02}, make([]byte, 128)...)
	record, n, err := readRecord(header, len(header), 0)
	if err != nil || n != 130 || len(record.Header.SerialTypes) != 128 {
		t.Fatalf("multibyte header: got (%+v, %d, %v)", record, n, err)
	}
	// Serial type 129 encodes 58 bytes of text.
	payload := append([]byte{3, 0x81, 0x01}, make([]byte, 58)...)
	record, n, err = readRecord(payload, len(payload), 0)
	if err != nil || n != len(payload) || record.Body.Value[0] != string(make([]byte, 58)) {
		t.Fatalf("multibyte serial type: got (%+v, %d, %v)", record, n, err)
	}
}

func TestNewCellMalformed(t *testing.T) {
	for _, tc := range []struct {
		name string
		buf  []byte
		off  int
	}{
		{"negative offset", []byte{1}, -1},
		{"past end", []byte{1}, 2},
		{"empty", nil, 0},
		{"truncated payload varint", []byte{0x80}, 0},
		{"missing rowid", []byte{1}, 0},
		{"missing payload", []byte{1, 1}, 0},
		{"zero payload", []byte{0, 1}, 0},
		{"zero header", []byte{1, 1, 0}, 0},
		{"header smaller than size varint", []byte{2, 1, 0x80, 1}, 0},
		{"header beyond payload", []byte{1, 1, 2, 0}, 0},
		{"serial type crossing header", []byte{3, 1, 2, 0x81, 0}, 0},
		{"field crossing payload", []byte{2, 1, 2, 1, 42}, 0},
		{"unused payload bytes", []byte{2, 1, 1, 0}, 0},
		{"reserved 10", []byte{2, 1, 2, 10}, 0},
		{"reserved 11", []byte{2, 1, 2, 11}, 0},
		{"huge payload", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 1}, 0},
		{"huge header", []byte{9, 1, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := NewCell(tc.buf, tc.off); err == nil {
				t.Fatal("accepted malformed cell")
			}
		})
	}
}

func TestRecordBodyTruncation(t *testing.T) {
	for _, st := range []SerialType{Int8, Int16, Int24, Int32, Int48, Int64, Float64, 16, 17, ^SerialType(0)} {
		if _, _, err := readRecordBody(nil, 0, []SerialType{st}); err == nil {
			t.Fatalf("accepted missing field for type %d", st)
		}
	}
	for _, off := range []int{-1, 1} {
		if _, _, err := readRecordBody(nil, off, nil); err == nil {
			t.Fatalf("accepted offset %d", off)
		}
	}
}

func TestRecordBounds(t *testing.T) {
	for _, args := range [][2]int{{-1, 0}, {1, -1}, {1, 2}, {2, 0}} {
		if _, _, err := readRecord([]byte{1}, args[0], args[1]); err == nil {
			t.Fatalf("accepted size/offset %v", args)
		}
	}
}
