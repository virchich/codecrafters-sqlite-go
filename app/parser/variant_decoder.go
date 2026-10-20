package parser

import (
	"fmt"
	"io"
)

// decodeVariant decodes a SQLite variant at offset and returns its raw 64-bit
// value and the number of bytes consumed. On error, the value is zero.
func decodeVariant(buf []byte, off int) (uint64, int, error) {
	if off < 0 || off > len(buf) {
		return 0, 0, fmt.Errorf("sql_decoder.DecodeVariant(): offset is out of bounds")
	}

	var value uint64
	data := buf[off:]
	for i := 0; i < 9; i++ {
		if i >= len(data) {
			return 0, i, io.ErrUnexpectedEOF
		}
		b := data[i]
		// The ninth byte contributes all eight bits, with no continuation bit.
		if i == 8 {
			return (value << 8) | uint64(b), 9, nil
		}
		value = (value << 7) | uint64(b&0x7f)
		if b&0x80 == 0 {
			return value, i + 1, nil
		}
	}
	panic("unreachable")
}
