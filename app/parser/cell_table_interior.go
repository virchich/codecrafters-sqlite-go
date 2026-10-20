package parser

import (
	"encoding/binary"
	"fmt"
	"io"
)

// TableInteriorCell holds the fields of a table interior cell.
type TableInteriorCell struct {
	PageNumber uint32
	RowId      uint64
}

// NewTableInteriorCell decodes a table interior cell at off and returns bytes consumed,
// including bytes read before an error. RowId retains the raw unsigned varint bits.
func NewTableInteriorCell(buf []byte, off int) (TableInteriorCell, int, error) {
	if off < 0 || off > len(buf) {
		return TableInteriorCell{}, 0, fmt.Errorf("table interior cell: invalid offset %d (buffer length %d)", off, len(buf))
	}
	if len(buf)-off < cellPageNumberLenBytes {
		return TableInteriorCell{}, 0, fmt.Errorf("table interior cell: read left child page number: %w", io.ErrUnexpectedEOF)
	}

	c := TableInteriorCell{}

	c.PageNumber = binary.BigEndian.Uint32(buf[off : off+cellPageNumberLenBytes])
	total := cellPageNumberLenBytes

	rowID, n, err := decodeVariant(buf, off+cellPageNumberLenBytes)
	total += n
	if err != nil {
		return TableInteriorCell{}, total, fmt.Errorf("table interior cell: decode rowId: %w", err)
	}
	c.RowId = rowID

	return c, total, nil
}
