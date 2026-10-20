package parser

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Record contains decoded values in storage order, without column names.
type Record struct {
	Header RecordHeader
	Body   []any
}

// RecordHeader describes the header size and each stored value's serial type.
type RecordHeader struct {
	Size        uint64
	SerialTypes []SerialType
}

// SerialType identifies a stored value's representation and byte length.
type SerialType uint64

const (
	Null        SerialType = 0
	Int8        SerialType = 1
	Int16       SerialType = 2
	Int24       SerialType = 3
	Int32       SerialType = 4
	Int48       SerialType = 5
	Int64       SerialType = 6
	Float64     SerialType = 7
	IntegerZero SerialType = 8 // No body bytes; schema format 4 or higher.
	IntegerOne  SerialType = 9 // No body bytes; schema format 4 or higher.
	Reserved10  SerialType = 10
	Reserved11  SerialType = 11

	// BlobBase BLOB codes are even N >= 12, with (N-12)/2 body bytes.
	BlobBase SerialType = 12
	// TextBase Text codes are odd N >= 13, with (N-13)/2 body bytes.
	TextBase SerialType = 13
)

// ParseRecord decodes exactly one complete record payload starting at offset zero.
// The caller must assemble any overflow bytes before calling ParseRecord.
// Truncated records and trailing bytes are rejected. Returned values do not
// retain references to payload. Text is interpreted as UTF-8.
func ParseRecord(payload []byte) (Record, error) {
	header, n, err := readRecordHeader(payload, 0)
	if err != nil {
		return Record{}, fmt.Errorf("record: decode header: %w", err)
	}
	body, bodyBytes, err := readRecordBody(payload, n, header.SerialTypes)
	if err != nil {
		return Record{}, fmt.Errorf("record: decode body: %w", err)
	}
	total := n + bodyBytes
	if total != len(payload) {
		return Record{}, fmt.Errorf("record: consumed %d bytes, payload contains %d", total, len(payload))
	}
	return Record{Header: header, Body: body}, nil
}

// readRecordHeader decodes the header and returns bytes consumed from off.
func readRecordHeader(buf []byte, off int) (RecordHeader, int, error) {
	headerSize, total, err := decodeVariant(buf, off)
	if err != nil {
		return RecordHeader{}, total, err
	}
	if headerSize < uint64(total) {
		return RecordHeader{}, total, fmt.Errorf("header size is smaller than its size varint")
	}
	if headerSize > uint64(len(buf)-off) {
		return RecordHeader{}, total, io.ErrUnexpectedEOF
	}
	headerEnd := off + int(headerSize)
	serialTypes := []SerialType{}
	for total < int(headerSize) {
		st, n, err := decodeVariant(buf[:headerEnd], off+total)
		total += n
		if err != nil {
			return RecordHeader{}, total, fmt.Errorf("serial type crosses header boundary: %w", err)
		}
		serialTypes = append(serialTypes, SerialType(st))
	}
	return RecordHeader{Size: headerSize, SerialTypes: serialTypes}, total, nil
}

// readRecordBody decodes values using serialTypes and returns bytes consumed from off.
func readRecordBody(buf []byte, off int, serialTypes []SerialType) ([]any, int, error) {
	if off < 0 || off > len(buf) {
		return nil, 0, fmt.Errorf("offset is out of bounds")
	}
	total := 0
	body := make([]any, len(serialTypes))
	for i, st := range serialTypes {
		var length uint64
		switch st {
		case Null:
			continue
		case IntegerZero:
			body[i] = int64(0)
			continue
		case IntegerOne:
			body[i] = int64(1)
			continue
		case Int8, Int16, Int24, Int32:
			length = uint64(st)
		case Int48:
			length = 6
		case Int64, Float64:
			length = 8
		case Reserved10, Reserved11:
			return nil, total, fmt.Errorf("reserved serial type: %d", st)
		default:
			if st%2 == 0 {
				length = uint64((st - BlobBase) / 2)
			} else {
				length = uint64((st - TextBase) / 2)
			}
		}
		if length > uint64(len(buf)-off-total) {
			return nil, total, io.ErrUnexpectedEOF
		}
		field := buf[off+total : off+total+int(length)]
		switch {
		case st >= Int8 && st <= Int64:
			// Starting with the signed first byte sign-extends short integers.
			value := int64(int8(field[0]))
			for _, b := range field[1:] {
				value = (value << 8) | int64(b)
			}
			body[i] = value
		case st == Float64:
			body[i] = math.Float64frombits(binary.BigEndian.Uint64(field))
		case st%2 == 0:
			// Own the BLOB bytes so reusing the page buffer cannot change them.
			blob := make([]byte, len(field))
			copy(blob, field)
			body[i] = blob
		default:
			body[i] = string(field)
		}
		total += int(length)
	}
	return body, total, nil
}
