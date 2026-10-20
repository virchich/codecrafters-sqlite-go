package parser

import (
	"encoding/binary"
	"fmt"
	"io"
)

type IndexLeafCell struct {
	PayloadSize        uint64
	LocalPayload       []byte
	OverflowPageNumber uint32
}

func NewIndexLeafCell(buf []byte, off int, dbh *DbHeader) (IndexLeafCell, int, error) {
	if off < 0 || off > len(buf) {
		return IndexLeafCell{}, 0, fmt.Errorf("index leaf cell: invalid offset %d (buffer length %d)", off, len(buf))
	}

	c := IndexLeafCell{}

	payloadSize, n, err := decodeVariant(buf, off)
	total := n
	if err != nil {
		return IndexLeafCell{}, total, fmt.Errorf("index leaf cell: decode payload size: %w", err)
	}
	c.PayloadSize = payloadSize

	localLen := localPayloadLen(payloadSize, dbh.PageSize, dbh.ReservedBytes, false)
	if localLen > uint64(len(buf)-(off+total)) {
		return IndexLeafCell{}, total,
			fmt.Errorf("index leaf cell: read local payload: %w", io.ErrUnexpectedEOF)
	}

	start := off + total
	c.LocalPayload = buf[start : start+int(localLen)]
	total += int(localLen)

	if localLen < payloadSize {
		if len(buf)-(off+total) < cellOverflowPageNumberLenBytes {
			return IndexLeafCell{}, total,
				fmt.Errorf("index leaf cell: read overflow page number: %w", io.ErrUnexpectedEOF)
		}
		c.OverflowPageNumber = binary.BigEndian.Uint32(
			buf[off+total : off+total+cellOverflowPageNumberLenBytes],
		)
		total += cellOverflowPageNumberLenBytes
	}

	return c, total, nil
}
