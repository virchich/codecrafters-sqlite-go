package parser

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type Page struct {
	dbHeader    *DbHeader
	pageNum     uint32
	Header      PageHeader
	cellPointer []uint16
	cellIdx     int
	buf         []byte
}

func NewPage(pageNum uint32, dbh *DbHeader) (Page, error) {
	if pageNum == 0 {
		return Page{}, fmt.Errorf("page: invalid page number 0 (page numbers start at 1)")
	}
	if dbh == nil {
		return Page{}, fmt.Errorf("page %d: database header is required", pageNum)
	}
	pageSize := dbh.PageSize
	if pageSize < 512 || pageSize > 65536 || pageSize&(pageSize-1) != 0 {
		return Page{}, fmt.Errorf("page %d: invalid page size: %d", pageNum, pageSize)
	}

	return Page{
		dbHeader:    dbh,
		pageNum:     pageNum,
		Header:      PageHeader{},
		cellPointer: []uint16{},
		cellIdx:     0,
		buf:         make([]byte, pageSize),
	}, nil
}

func (p *Page) Parse(dbf *os.File) error {
	// Invalidate iteration before replacing the buffer, including on read errors.
	p.cellPointer = nil
	p.cellIdx = 0
	p.Header = PageHeader{}
	offset := int64(p.pageNum-1) * int64(p.dbHeader.PageSize)

	if _, err := dbf.ReadAt(p.buf, offset); err != nil {
		return fmt.Errorf("page %d: read at file offset %d: %w", p.pageNum, offset, err)
	}
	off, err := p.Header.Parse(p.pageNum, p.buf)
	if err != nil {
		return fmt.Errorf("page %d: decode header: %w", p.pageNum, err)
	}

	// Parse returns the B-tree header size, not its absolute page offset.
	if p.pageNum == 1 {
		off += DbHeaderSizeBytes
	}
	pointers, err := p.getCellPointerArray(p.buf, off, p.Header.NumOfCellsOnPage)
	if err != nil {
		return fmt.Errorf("page %d: decode cell pointer array: %w", p.pageNum, err)
	}
	p.cellPointer = pointers
	return nil
}

func (p *Page) getCellPointerArray(buf []byte, off int, numOfCells uint16) ([]uint16, error) {
	if off < 0 || off > len(buf) {
		return nil, fmt.Errorf("invalid cell pointer array offset %d (buffer length %d)", off, len(buf))
	}
	if int(numOfCells) > (len(buf)-off)/2 {
		return nil, fmt.Errorf("read cell pointers: %w", io.ErrUnexpectedEOF)
	}
	cellLenBytes := 2
	cellPointer := make([]uint16, numOfCells)
	for i := 0; i < int(numOfCells); i++ {
		cellPointer[i] = binary.BigEndian.Uint16(buf[off : off+cellLenBytes])
		off += 2
	}
	return cellPointer, nil
}

// currentCellOffset validates the page type and current cell pointer without
// advancing the cursor. It returns io.EOF when iteration is complete.
func (p *Page) currentCellOffset(expectedPageType uint8) (int, error) {
	headerSize := leafPageHeaderSize
	var cellType string
	switch expectedPageType {
	case TableLeafPage:
		cellType = "table leaf"
	case TableInteriorPage:
		cellType = "table interior"
		headerSize = interiorPageHeaderSize
	case IndexLeafPage:
		cellType = "index leaf"
	case IndexInteriorPage:
		cellType = "index interior"
		headerSize = interiorPageHeaderSize
	default:
		return 0, fmt.Errorf("page %d: unsupported cell page type %#x", p.pageNum, expectedPageType)
	}
	if p.Header.PageType != expectedPageType {
		return 0, fmt.Errorf("page %d: %s cell iteration requires page type %#x, got %#x", p.pageNum, cellType, expectedPageType, p.Header.PageType)
	}
	if p.cellIdx >= len(p.cellPointer) {
		return 0, io.EOF
	}
	headerStart := 0
	if p.pageNum == 1 {
		headerStart = DbHeaderSizeBytes
	}
	pointer := int(p.cellPointer[p.cellIdx])
	pointerArrayEnd := headerStart + headerSize + 2*len(p.cellPointer)
	if pointer < pointerArrayEnd || pointer >= len(p.buf) ||
		uint32(pointer) < p.Header.CellContentAreaStart {
		return 0, fmt.Errorf("page %d: %s cell %d: invalid cell pointer: %d", p.pageNum, cellType, p.cellIdx, pointer)
	}
	return pointer, nil
}

func (p *Page) GetNextTableLeafCell() (TableLeafCell, error) {
	pointer, err := p.currentCellOffset(TableLeafPage)
	if err != nil {
		return TableLeafCell{}, err
	}
	cell, _, err := NewTableLeafCell(p.buf, pointer, p.dbHeader)
	if err != nil {
		return TableLeafCell{}, fmt.Errorf("page %d: table leaf cell %d at offset %d: %w", p.pageNum, p.cellIdx, pointer, err)
	}
	p.cellIdx++
	return cell, nil
}

func (p *Page) GetNextTableInteriorCell() (TableInteriorCell, error) {
	pointer, err := p.currentCellOffset(TableInteriorPage)
	if err != nil {
		return TableInteriorCell{}, err
	}
	cell, _, err := NewTableInteriorCell(p.buf, pointer)
	if err != nil {
		return TableInteriorCell{}, fmt.Errorf("page %d: table interior cell %d at offset %d: %w", p.pageNum, p.cellIdx, pointer, err)
	}
	p.cellIdx++
	return cell, nil
}

func (p *Page) GetNextIndexInteriorCell() (IndexInteriorCell, error) {
	pointer, err := p.currentCellOffset(IndexInteriorPage)
	if err != nil {
		return IndexInteriorCell{}, err
	}
	cell, _, err := NewIndexInteriorCell(p.buf, pointer, p.dbHeader)
	if err != nil {
		return IndexInteriorCell{}, fmt.Errorf("page %d: index interior cell %d at offset %d: %w", p.pageNum, p.cellIdx, pointer, err)
	}
	p.cellIdx++
	return cell, nil
}

func (p *Page) GetNextIndexLeafCell() (IndexLeafCell, error) {
	pointer, err := p.currentCellOffset(IndexLeafPage)
	if err != nil {
		return IndexLeafCell{}, err
	}
	cell, _, err := NewIndexLeafCell(p.buf, pointer, p.dbHeader)
	if err != nil {
		return IndexLeafCell{}, fmt.Errorf("page %d: index leaf cell %d at offset %d: %w", p.pageNum, p.cellIdx, pointer, err)
	}
	p.cellIdx++
	return cell, nil
}
