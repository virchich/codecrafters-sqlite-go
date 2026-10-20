package parser

import (
	"encoding/binary"
	"fmt"
	"io"
)

type IndexInteriorCell struct {
	LocalPayload       []byte
	PayloadSize        uint64
	PageNumber         uint32
	OverflowPageNumber uint32
}

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
	if payloadSize > uint64(len(buf)-(off+total)) {
		return IndexInteriorCell{}, total, fmt.Errorf("index interior cell: payload is not available locally: %w", io.ErrUnexpectedEOF)
	}
	c.PayloadSize = payloadSize

	localLen := localPayloadLen(payloadSize, dbh.PageSize, dbh.ReservedBytes, true)
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
