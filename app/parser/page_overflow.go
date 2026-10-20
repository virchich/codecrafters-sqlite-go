package parser

import (
	"encoding/binary"
	"fmt"
	"os"
)

type OverflowPage struct {
	dbHeader *DbHeader
	buf      []byte
	Payload  []byte
	pageNum  uint32
	NextPage uint32
}

func NewOverflowPage(pageNum uint32, dbh *DbHeader) (OverflowPage, error) {
	if pageNum == 0 {
		return OverflowPage{}, fmt.Errorf("overflow page: page number is zero")
	}
	if dbh == nil {
		return OverflowPage{}, fmt.Errorf("overflow page %d: database header is required", pageNum)
	}
	pageSize := dbh.PageSize
	if pageSize < 512 || pageSize > 65536 || pageSize&(pageSize-1) != 0 {
		return OverflowPage{}, fmt.Errorf("overflow page %d: invalid page size: %d", pageNum, pageSize)
	}

	return OverflowPage{
		dbHeader: dbh,
		pageNum:  pageNum,
		buf:      make([]byte, pageSize),
	}, nil
}

func (p *OverflowPage) Parse(dbf *os.File) error {
	offset := int64(p.pageNum-1) * int64(p.dbHeader.PageSize)

	if _, err := dbf.ReadAt(p.buf, offset); err != nil {
		return fmt.Errorf("overflow page %d: read at file offset %d: %w", p.pageNum, offset, err)
	}

	p.NextPage = binary.BigEndian.Uint32(p.buf[:cellOverflowPageNumberLenBytes])

	usableSize := int(p.dbHeader.PageSize) - int(p.dbHeader.ReservedBytes)
	p.Payload = p.buf[cellOverflowPageNumberLenBytes:usableSize]

	return nil
}
