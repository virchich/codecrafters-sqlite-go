package parser

import (
	"encoding/binary"
	"fmt"
	"io"
)

// TableLeafCell holds the fields of a table leaf cell.
type TableLeafCell struct {
	PayloadSize        uint64
	RowId              uint64
	LocalPayload       []byte
	OverflowPageNumber uint32
}

// NewTableLeafCell decodes a table leaf cell at off and returns bytes consumed,
// including bytes read before an error. LocalPayload borrows buf; overflow pages
// are not read. dbh must contain validated page parameters.
func NewTableLeafCell(buf []byte, off int, dbh *DbHeader) (TableLeafCell, int, error) {
	if off < 0 || off > len(buf) {
		return TableLeafCell{}, 0, fmt.Errorf("table leaf cell: invalid offset %d (buffer length %d)", off, len(buf))
	}

	c := TableLeafCell{}

	payloadSize, n, err := decodeVariant(buf, off)
	total := n
	if err != nil {
		return TableLeafCell{}, total, fmt.Errorf("table leaf cell: decode payload size: %w", err)
	}
	c.PayloadSize = payloadSize

	rowID, n, err := decodeVariant(buf, off+total)
	total += n
	if err != nil {
		return TableLeafCell{}, total, fmt.Errorf("table leaf cell: decode rowId: %w", err)
	}
	c.RowId = rowID

	localLen := localPayloadLen(payloadSize, dbh.PageSize, dbh.ReservedBytes, true)
	if localLen > uint64(len(buf)-(off+total)) {
		return TableLeafCell{}, total,
			fmt.Errorf("table leaf cell: read local payload: %w", io.ErrUnexpectedEOF)
	}

	start := off + total
	c.LocalPayload = buf[start : start+int(localLen)]
	total += int(localLen)

	if localLen < payloadSize {
		if len(buf)-(off+total) < cellOverflowPageNumberLenBytes {
			return TableLeafCell{}, total,
				fmt.Errorf("table leaf cell: read overflow page number: %w", io.ErrUnexpectedEOF)
		}
		c.OverflowPageNumber = binary.BigEndian.Uint32(
			buf[off+total : off+total+cellOverflowPageNumberLenBytes],
		)
		total += cellOverflowPageNumberLenBytes
	}

	return c, total, nil
}
