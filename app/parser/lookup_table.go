package parser

import (
	"fmt"
	"io"
)

// LookupTableByRowID returns the exact row, or io.EOF if it does not exist.
func (db *DB) LookupTableByRowID(rootPage uint32, rowID int64) (TableRecord, error) {
	if rootPage == 0 {
		return TableRecord{}, fmt.Errorf("lookup table: invalid root page: %d", rootPage)
	}
	if db == nil || db.file == nil {
		return TableRecord{}, fmt.Errorf("lookup table: database file is required")
	}

	pageNumber := rootPage
	visited := make(map[uint32]bool)
	for {
		if visited[pageNumber] {
			return TableRecord{}, fmt.Errorf("lookup table: cycle through page %d", pageNumber)
		}
		visited[pageNumber] = true
		page, err := NewPage(pageNumber, &db.Header)
		if err != nil {
			return TableRecord{}, fmt.Errorf("lookup table: open page %d: %w", pageNumber, err)
		}
		if err := page.Parse(db.file); err != nil {
			return TableRecord{}, fmt.Errorf("lookup table: read page %d: %w", pageNumber, err)
		}

		switch page.Header.PageType {
		case TableInteriorPage:
			// If no separator qualifies, use the rightmost child.
			child := page.Header.RightMostPointer
			for {
				cell, err := page.GetNextTableInteriorCell()
				if err == io.EOF {
					break
				}
				if err != nil {
					return TableRecord{}, fmt.Errorf("lookup table: page %d: read interior cell: %w", pageNumber, err)
				}
				if rowID <= int64(cell.RowId) {
					child = cell.PageNumber
					break
				}
			}
			pageNumber = child

		case TableLeafPage:
			for {
				cell, err := page.GetNextTableLeafCell()
				if err == io.EOF {
					return TableRecord{}, io.EOF
				}
				if err != nil {
					return TableRecord{}, fmt.Errorf("lookup table: page %d: read leaf cell: %w", pageNumber, err)
				}
				if int64(cell.RowId) < rowID {
					continue
				}
				if int64(cell.RowId) > rowID {
					return TableRecord{}, io.EOF
				}
				payload, err := db.readPayload(cell.LocalPayload, cell.PayloadSize, cell.OverflowPageNumber)
				if err != nil {
					return TableRecord{}, fmt.Errorf("lookup table: page %d: rowID %d: read payload: %w", pageNumber, rowID, err)
				}
				record, err := ParseRecord(payload)
				if err != nil {
					return TableRecord{}, fmt.Errorf("lookup table: page %d: rowID %d: decode record: %w", pageNumber, rowID, err)
				}
				return TableRecord{RowID: rowID, Record: record}, nil
			}

		default:
			return TableRecord{}, fmt.Errorf("lookup table: page %d: expected table page, got %#x", pageNumber, page.Header.PageType)
		}
	}
}
