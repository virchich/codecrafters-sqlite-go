package parser

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type Page struct {
	pageNum     uint32
	pageSize    uint32
	Header      PageHeader
	cellPointer []uint16
	cellIdx     int
	buf         []byte
}

func NewPage(pageNum uint32, pageSize uint32) (Page, error) {
	if pageNum == 0 {
		return Page{}, fmt.Errorf("page numbers start at 1")
	}
	if pageSize < 512 || pageSize > 65536 || pageSize&(pageSize-1) != 0 {
		return Page{}, fmt.Errorf("invalid page size: %d", pageSize)
	}

	buf := make([]byte, pageSize)

	return Page{
		pageNum:     pageNum,
		pageSize:    pageSize,
		Header:      PageHeader{},
		cellPointer: []uint16{},
		cellIdx:     0,
		buf:         buf,
	}, nil
}

func (p *Page) Parse(dbf *os.File) error {
	// Invalidate iteration before replacing the buffer, including on read errors.
	p.cellPointer = nil
	p.cellIdx = 0
	p.Header = PageHeader{}
	offset := int64(p.pageNum-1) * int64(p.pageSize)

	if _, err := dbf.ReadAt(p.buf, offset); err != nil {
		return err
	}
	off, err := p.Header.Parse(p.pageNum, p.buf)
	if err != nil {
		return err
	}

	// Parse returns the B-tree header size, not its absolute page offset.
	if p.pageNum == 1 {
		off += DbHeaderSizeBytes
	}
	pointers, err := p.getCellPointerArray(p.buf, off, p.Header.NumOfCellsOnPage)
	if err != nil {
		return err
	}
	p.cellPointer = pointers
	return nil
}

func (p *Page) getCellPointerArray(buf []byte, off int, numOfCells uint16) ([]uint16, error) {
	if off < 0 || off > len(buf) {
		return nil, fmt.Errorf("cell pointer array offset is out of bounds")
	}
	if int(numOfCells) > (len(buf)-off)/2 {
		return nil, fmt.Errorf("cell pointer array: %w", io.ErrUnexpectedEOF)
	}
	cellLenBytes := 2
	cellPointer := make([]uint16, numOfCells)
	for i := 0; i < int(numOfCells); i++ {
		cellPointer[i] = binary.BigEndian.Uint16(buf[off : off+cellLenBytes])
		off += 2
	}
	return cellPointer, nil
}

func (p *Page) GetNextCell() (Cell, error) {
	if p.Header.PageType != tableLeafPage {
		return Cell{}, fmt.Errorf("cell iteration requires a table leaf page, got %#x", p.Header.PageType)
	}
	if p.cellIdx >= len(p.cellPointer) {
		return Cell{}, io.EOF
	}
	pointer := int(p.cellPointer[p.cellIdx])
	headerStart := 0
	if p.pageNum == 1 {
		headerStart = DbHeaderSizeBytes
	}
	pointerArrayEnd := headerStart + leafPageHeaderSize + 2*len(p.cellPointer)
	if pointer < pointerArrayEnd || pointer >= len(p.buf) ||
		uint32(pointer) < p.Header.CellContentAreaStart {
		return Cell{}, fmt.Errorf("invalid cell pointer: %d", pointer)
	}
	cell, _, err := NewCell(p.buf, int(p.cellPointer[p.cellIdx]))
	if err != nil {
		return Cell{}, err
	}
	p.cellIdx++
	return cell, nil
}
