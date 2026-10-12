package parser

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	leafPageHeaderSize     = 8
	interiorPageHeaderSize = 12

	indexInteriorPage = 0x02
	tableInteriorPage = 0x05
	indexLeafPage     = 0x0a
	tableLeafPage     = 0x0d

	pageTypeOffset = 0
	pageTypeLen    = 1

	firstFreeblockStartOffset = 1
	firstFreeblockStartLen    = 2

	numOfCellsOnPageOffset = 3
	numOfCellsOnPageLen    = 2

	cellContentAreaStartOffset = 5
	cellContentAreaStartLen    = 2

	fragmentedFreeBytesOffset = 7
	fragmentedFreeBytesLen    = 1

	rightMostPointerOffset = 8
	rightMostPointerLen    = 4
)

type PageHeader struct {
	PageType               uint8
	FirstFreeblockStart    uint16
	NumOfCellsOnPage       uint16
	CellContentAreaStart   uint32
	FragmentedFreeBytesNum uint8
	RightMostPointer       uint32
}

// Parse expects a buffer starting at the beginning of the whole page.
func (h *PageHeader) Parse(pageNum uint32, buf []byte) (int, error) {
	if pageNum == 0 {
		return 0, fmt.Errorf("page numbers start at 1")
	}
	headerStart := 0
	if pageNum == 1 {
		headerStart = DbHeaderSizeBytes
	}
	if len(buf) < headerStart+leafPageHeaderSize {
		return 0, io.ErrUnexpectedEOF
	}
	header := buf[headerStart:]
	pageType := header[pageTypeOffset]
	headerSize := leafPageHeaderSize

	switch pageType {
	case indexInteriorPage, tableInteriorPage:
		headerSize = interiorPageHeaderSize
	case indexLeafPage, tableLeafPage:
	default:
		return 0, fmt.Errorf("invalid b-tree page type: %#x", pageType)
	}

	if len(header) < headerSize {
		return 0, io.ErrUnexpectedEOF
	}

	parsed := PageHeader{
		PageType:               pageType,
		FirstFreeblockStart:    binary.BigEndian.Uint16(header[firstFreeblockStartOffset : firstFreeblockStartOffset+firstFreeblockStartLen]),
		NumOfCellsOnPage:       binary.BigEndian.Uint16(header[numOfCellsOnPageOffset : numOfCellsOnPageOffset+numOfCellsOnPageLen]),
		CellContentAreaStart:   uint32(binary.BigEndian.Uint16(header[cellContentAreaStartOffset : cellContentAreaStartOffset+cellContentAreaStartLen])),
		FragmentedFreeBytesNum: header[fragmentedFreeBytesOffset],
	}
	// SQLite uses zero to represent an offset that cannot fit in two bytes.
	if parsed.CellContentAreaStart == 0 {
		parsed.CellContentAreaStart = 65536
	}
	if headerSize == interiorPageHeaderSize {
		parsed.RightMostPointer = binary.BigEndian.Uint32(header[rightMostPointerOffset : rightMostPointerOffset+rightMostPointerLen])
	}
	*h = parsed
	return headerSize, nil
}
