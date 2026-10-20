package parser

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"
)

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
			if err != nil || n != tc.width || body[0] != want {
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
		if got := math.Float64bits(body[0].(float64)); got != bits {
			t.Fatalf("got bits %x, want %x", got, bits)
		}
	}
}

func TestRecordSpecialValues(t *testing.T) {
	buf := []byte{0, 0xff, 'h', 'i'}
	types := []SerialType{Null, IntegerZero, IntegerOne, BlobBase, TextBase, 16, 17}
	body, n, err := readRecordBody(buf, 0, types)
	want := []any{nil, int64(0), int64(1), []byte{}, "", []byte{0, 0xff}, "hi"}
	if err != nil || n != len(buf) || !reflect.DeepEqual(body, want) {
		t.Fatalf("got (%+v, %d, %v)", body, n, err)
	}
	buf[0] = 123
	if !reflect.DeepEqual(body[5], []byte{0, 0xff}) {
		t.Fatal("BLOB aliases input buffer")
	}
}

func TestRecordMultibyteVarints(t *testing.T) {
	// Header size 130 encoded in two bytes, then 128 NULL serial types.
	header := append([]byte{0x81, 0x02}, make([]byte, 128)...)
	record, err := ParseRecord(header)
	if err != nil || len(record.Header.SerialTypes) != 128 {
		t.Fatalf("multibyte header: got (%+v, %v)", record, err)
	}
	// Serial type 129 encodes 58 bytes of text.
	payload := append([]byte{3, 0x81, 0x01}, make([]byte, 58)...)
	record, err = ParseRecord(payload)
	if err != nil || record.Body[0] != string(make([]byte, 58)) {
		t.Fatalf("multibyte serial type: got (%+v, %v)", record, err)
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

func TestParseRecordMalformed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload []byte
	}{
		{"empty", nil},
		{"truncated header size", []byte{0x80}},
		{"zero header size", []byte{0}},
		{"header beyond payload", []byte{3, 0}},
		{"serial type crossing header", []byte{2, 0x81, 0}},
		{"truncated body", []byte{2, 1}},
		{"trailing bytes", []byte{1, 0}},
		{"reserved serial type", []byte{2, 10}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseRecord(tc.payload); err == nil {
				t.Fatal("accepted malformed record")
			}
		})
	}
}

func TestParseRecord(t *testing.T) {
	payload := []byte{3, 23, 1, 'a', 'p', 'p', 'l', 'e', 42}
	record, err := ParseRecord(payload)
	if err != nil {
		t.Fatal(err)
	}
	if record.Header.Size != 3 ||
		!reflect.DeepEqual(record.Header.SerialTypes, []SerialType{23, Int8}) ||
		!reflect.DeepEqual(record.Body, []any{"apple", int64(42)}) {
		t.Fatalf("unexpected record: %+v", record)
	}
}

func TestParseRecordOwnsValues(t *testing.T) {
	payload := []byte{3, 16, 17, 1, 2, 'h', 'i'}
	record, err := ParseRecord(payload)
	if err != nil {
		t.Fatal(err)
	}
	for i := range payload {
		payload[i] = 0
	}
	if !reflect.DeepEqual(record.Body, []any{[]byte{1, 2}, "hi"}) {
		t.Fatalf("record aliases source payload: %+v", record)
	}
}

func TestParseEmptyRecord(t *testing.T) {
	record, err := ParseRecord([]byte{1})
	if err != nil || record.Header.Size != 1 || len(record.Body) != 0 {
		t.Fatalf("empty record=%+v err=%v", record, err)
	}
}
