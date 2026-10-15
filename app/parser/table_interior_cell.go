package parser

import (
	"encoding/binary"
	"fmt"
	"io"
)

const pageNumberLength = 4

type TableInteriorCell struct {
	PageNumber uint32
	Rowid      uint64
}

func NewTableInteriorCell(buf []byte, off int) (TableInteriorCell, int, error) {
	if off < 0 || off > len(buf) {
		return TableInteriorCell{}, 0,
			fmt.Errorf("NewTableInteriorCell(): invalid offset %d", off)
	}
	if len(buf)-off < pageNumberLength {
		return TableInteriorCell{}, 0, io.ErrUnexpectedEOF
	}

	c := TableInteriorCell{}
	c.PageNumber = uint32(binary.BigEndian.Uint32(buf[off : off+pageNumberLength]))

	total := pageNumberLength

	rowid, n, err := DecodeVariant(buf, off+pageNumberLength)
	total += n
	if err != nil {
		return TableInteriorCell{}, total, err
	}

	c.Rowid = rowid

	return c, total, nil
}
