package parser

import (
	"encoding/binary"
	"fmt"
	"io"
)

// IndexInteriorCell holds the fields of a index interior cell.
type IndexInteriorCell struct {
	LocalPayload       []byte
	PayloadSize        uint64
	PageNumber         uint32
	OverflowPageNumber uint32
}

// NewIndexInteriorCell decodes a index interior cell at off and returns bytes consumed,
// including bytes read before an error. LocalPayload borrows buf; overflow pages
// are not read. dbh must contain validated page parameters.
func NewIndexInteriorCell(buf []byte, off int, dbh *DbHeader) (IndexInteriorCell, int, error) {
	if off < 0 || off > len(buf) {
		return IndexInteriorCell{}, 0, fmt.Errorf("index interior cell: invalid offset %d (buffer length %d)", off, len(buf))
	}
	if len(buf)-off < cellPageNumberLenBytes {
		return IndexInteriorCell{}, 0, fmt.Errorf("index interior cell: read left child page number: %w", io.ErrUnexpectedEOF)
	}

	c := IndexInteriorCell{}

	c.PageNumber = binary.BigEndian.Uint32(buf[off : off+cellPageNumberLenBytes])
	total := cellPageNumberLenBytes

	payloadSize, n, err := decodeVariant(buf, off+cellPageNumberLenBytes)
	total += n
	if err != nil {
		return IndexInteriorCell{}, total, fmt.Errorf("index interior cell: decode payload size: %w", err)
	}
	c.PayloadSize = payloadSize

	localLen := localPayloadLen(payloadSize, dbh.PageSize, dbh.ReservedBytes, false)
	if localLen > uint64(len(buf)-(off+total)) {
		return IndexInteriorCell{}, total,
			fmt.Errorf("index interior cell: read local payload: %w", io.ErrUnexpectedEOF)
	}

	start := off + total
	c.LocalPayload = buf[start : start+int(localLen)]
	total += int(localLen)

	if localLen < payloadSize {
		if len(buf)-(off+total) < cellOverflowPageNumberLenBytes {
			return IndexInteriorCell{}, total,
				fmt.Errorf("index interior cell: read overflow page number: %w", io.ErrUnexpectedEOF)
		}
		c.OverflowPageNumber = binary.BigEndian.Uint32(
			buf[off+total : off+total+cellOverflowPageNumberLenBytes],
		)
		total += cellOverflowPageNumberLenBytes
	}

	return c, total, nil
}
