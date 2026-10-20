package parser

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	leafPageHeaderSize     = 8
	interiorPageHeaderSize = 12

	IndexInteriorPage = 0x02
	TableInteriorPage = 0x05
	IndexLeafPage     = 0x0a
	TableLeafPage     = 0x0d

	pageTypeOffset = 0
	pageTypeLen    = 1

	firstFreeBlockStartOffset   = 1
	firstFreeBlockStartLenBytes = 2

	numOfCellsOnPageOffset   = 3
	numOfCellsOnPageLenBytes = 2

	cellContentAreaStartOffset   = 5
	cellContentAreaStartLenBytes = 2

	fragmentedFreeBytesOffset   = 7
	fragmentedFreeBytesLenBytes = 1

	rightMostPointerOffset   = 8
	rightMostPointerLenBytes = 4
)

// PageHeader describes a B-tree page. RightMostPointer is used only by interior pages.
type PageHeader struct {
	CellContentAreaStart   uint32
	RightMostPointer       uint32
	FirstFreeblockStart    uint16
	NumOfCellsOnPage       uint16
	PageType               uint8
	FragmentedFreeBytesNum uint8
}

// Parse decodes a header from a complete page buffer, accounting for page 1
// metadata. It returns the B-tree header size, excluding the database header.
func (h *PageHeader) Parse(pageNum uint32, buf []byte) (int, error) {
	if pageNum == 0 {
		return 0, fmt.Errorf("page_header.Parse(): page numbers start at 1")
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
	case IndexInteriorPage, TableInteriorPage:
		headerSize = interiorPageHeaderSize
	case IndexLeafPage, TableLeafPage:
	default:
		return 0, fmt.Errorf("page_header.Parse(): invalid b-tree page type: %#x", pageType)
	}

	if len(header) < headerSize {
		return 0, io.ErrUnexpectedEOF
	}

	parsed := PageHeader{
		PageType:               pageType,
		FirstFreeblockStart:    binary.BigEndian.Uint16(header[firstFreeBlockStartOffset : firstFreeBlockStartOffset+firstFreeBlockStartLenBytes]),
		NumOfCellsOnPage:       binary.BigEndian.Uint16(header[numOfCellsOnPageOffset : numOfCellsOnPageOffset+numOfCellsOnPageLenBytes]),
		CellContentAreaStart:   uint32(binary.BigEndian.Uint16(header[cellContentAreaStartOffset : cellContentAreaStartOffset+cellContentAreaStartLenBytes])),
		FragmentedFreeBytesNum: header[fragmentedFreeBytesOffset],
	}
	// SQLite uses zero to represent an offset that cannot fit in two bytes.
	if parsed.CellContentAreaStart == 0 {
		parsed.CellContentAreaStart = 65536
	}
	if headerSize == interiorPageHeaderSize {
		parsed.RightMostPointer = binary.BigEndian.Uint32(header[rightMostPointerOffset : rightMostPointerOffset+rightMostPointerLenBytes])
	}
	*h = parsed
	return headerSize, nil
}
