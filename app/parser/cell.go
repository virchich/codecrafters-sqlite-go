package parser

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

type Cell struct {
	Size   uint64
	Rowid  int64
	Record Record
}

type Record struct {
	Header RecordHeader
	Body   RecordBody
}

type RecordHeader struct {
	Size        uint64
	SerialTypes []SerialType
}

type RecordBody struct {
	Value []any
}

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

	// BLOB codes are even N >= 12, with (N-12)/2 body bytes.
	BlobBase SerialType = 12
	// Text codes are odd N >= 13, with (N-13)/2 body bytes.
	TextBase SerialType = 13
)

// NewCell reads a table leaf cell whose entire payload is stored locally.
// Text is interpreted as UTF-8. The caller must handle page types and overflow
// pages before using this parser; this function has no page metadata.
// Returned byte counts include bytes consumed before an error.
func NewCell(buf []byte, off int) (Cell, int, error) {
	payloadSize, n, err := DecodeVariant(buf, off)
	if err != nil {
		return Cell{}, n, err
	}
	total := n
	rowID, n, err := DecodeVariant(buf, off+total)
	total += n
	if err != nil {
		return Cell{}, total, err
	}
	// Check before converting the untrusted uint64 length to int.
	if payloadSize > uint64(len(buf)-(off+total)) {
		return Cell{}, total, fmt.Errorf("cell payload is not available locally: %w", io.ErrUnexpectedEOF)
	}
	record, n, err := readRecord(buf, int(payloadSize), off+total)
	total += n
	if err != nil {
		return Cell{}, total, err
	}
	return Cell{Size: payloadSize, Rowid: int64(rowID), Record: record}, total, nil
}

func readRecord(buf []byte, size int, off int) (Record, int, error) {
	if off < 0 || off > len(buf) || size < 0 {
		return Record{}, 0, fmt.Errorf("invalid record offset or size")
	}
	if size > len(buf)-off {
		return Record{}, 0, io.ErrUnexpectedEOF
	}
	// Neither the header nor the body may read neighboring cell bytes.
	payload := buf[off : off+size]
	header, n, err := readRecordHeader(payload, 0)
	if err != nil {
		return Record{}, n, err
	}
	body, bodyBytes, err := readRecordBody(payload, n, header.SerialTypes)
	total := n + bodyBytes
	if err != nil {
		return Record{}, total, err
	}
	if total != size {
		return Record{}, total, fmt.Errorf("record consumed %d bytes, payload contains %d", total, size)
	}
	return Record{Header: header, Body: body}, total, nil
}

func readRecordHeader(buf []byte, off int) (RecordHeader, int, error) {
	headerSize, total, err := DecodeVariant(buf, off)
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
		st, n, err := DecodeVariant(buf[:headerEnd], off+total)
		total += n
		if err != nil {
			return RecordHeader{}, total, fmt.Errorf("serial type crosses header boundary: %w", err)
		}
		serialTypes = append(serialTypes, SerialType(st))
	}
	return RecordHeader{Size: headerSize, SerialTypes: serialTypes}, total, nil
}

func readRecordBody(buf []byte, off int, serialTypes []SerialType) (RecordBody, int, error) {
	if off < 0 || off > len(buf) {
		return RecordBody{}, 0, fmt.Errorf("offset is out of bounds")
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
			return RecordBody{}, total, fmt.Errorf("reserved serial type: %d", st)
		default:
			if st%2 == 0 {
				length = uint64((st - BlobBase) / 2)
			} else {
				length = uint64((st - TextBase) / 2)
			}
		}
		if length > uint64(len(buf)-off-total) {
			return RecordBody{}, total, io.ErrUnexpectedEOF
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
	return RecordBody{Value: body}, total, nil
}
