package parser

import (
	"fmt"
	"io"
)

type TableRecord struct {
	RowID  int64
	Record Record
}
type TableCursor struct {
	db        *DB
	stack     []frame
	leaf      *Page
	exhausted bool
	failure   error
}

type frame struct {
	page      Page
	nextChild int
}

func NewTableCursor(rootPage uint32, db *DB) (TableCursor, error) {
	if rootPage == 0 {
		return TableCursor{}, fmt.Errorf("table cursor: invalid root page: %d", rootPage)
	}
	if db == nil || db.file == nil {
		return TableCursor{}, fmt.Errorf("table cursor: database file is required")
	}

	c := TableCursor{db: db}
	pageNumber := rootPage

	if err := c.descendToLeaf(pageNumber); err != nil {
		return TableCursor{}, fmt.Errorf("table cursor: initialize root page %d: %w", rootPage, err)
	}

	return c, nil
}

func (c *TableCursor) descendToLeaf(pageNumber uint32) error {
	for {
		for _, ancestor := range c.stack {
			if ancestor.page.pageNum == pageNumber {
				return fmt.Errorf("table cursor: cycle through interior page %d", pageNumber)
			}
		}
		p, err := NewPage(pageNumber, &c.db.Header)
		if err != nil {
			return fmt.Errorf("table cursor: descend to page %d: %w", pageNumber, err)
		}
		if err := p.Parse(c.db.file); err != nil {
			return fmt.Errorf("table cursor: descend to page %d: %w", pageNumber, err)
		}
		switch p.Header.PageType {
		case TableLeafPage:
			c.leaf = &p
			return nil
		case TableInteriorPage:
			cell, err := p.GetNextTableInteriorCell()
			if err == io.EOF {
				// A zero-cell interior page still has its rightmost child.
				pageNumber = p.Header.RightMostPointer
			} else if err != nil {
				return fmt.Errorf("table cursor: page %d: read first child: %w", p.pageNum, err)
			} else {
				// Must be in else branch so it does not overwrite RightMostPointer after EOF.
				pageNumber = cell.PageNumber
			}
			// Save the updated page iterator; child zero has been entered.
			c.stack = append(c.stack, frame{page: p, nextChild: 1})
		default:
			return fmt.Errorf("table cursor: page %d: expected table page, got %#x", p.pageNum, p.Header.PageType)
		}
	}
}

func (c *TableCursor) advanceToNextLeaf() error {
	for len(c.stack) > 0 {
		top := &c.stack[len(c.stack)-1]
		var child uint32
		switch {
		case top.nextChild < len(top.page.cellPointer):
			cell, err := top.page.GetNextTableInteriorCell()
			if err != nil {
				return fmt.Errorf("table cursor: page %d: read child %d: %w", top.page.pageNum, top.nextChild, err)
			}
			child = cell.PageNumber
		case top.nextChild == len(top.page.cellPointer):
			child = top.page.Header.RightMostPointer
		default:
			c.stack = c.stack[:len(c.stack)-1]
			continue
		}
		parent, childIndex := top.page.pageNum, top.nextChild
		top.nextChild++
		if err := c.descendToLeaf(child); err != nil {
			return fmt.Errorf("table cursor: page %d: follow child %d to page %d: %w", parent, childIndex, child, err)
		}
		return nil
	}
	return io.EOF
}

// Next returns the next table record in rowId order. After a failure, subsequent
// calls return that failure rather than silently skipping the damaged record.
func (c *TableCursor) Next() (TableRecord, error) {
	if c.failure != nil {
		return TableRecord{}, c.failure
	}
	if c.exhausted {
		return TableRecord{}, io.EOF
	}
	fail := func(err error) (TableRecord, error) {
		c.failure = err
		return TableRecord{}, err
	}
	if c.leaf == nil || c.db == nil || c.db.file == nil {
		return fail(fmt.Errorf("table cursor: not initialized"))
	}
	for {
		cellIndex := c.leaf.cellIdx
		cell, err := c.leaf.GetNextTableLeafCell()
		if err == io.EOF {
			err = c.advanceToNextLeaf()
			if err == io.EOF {
				c.exhausted = true
				return TableRecord{}, io.EOF
			}
			if err != nil {
				return fail(err)
			}
			continue
		}
		if err != nil {
			return fail(fmt.Errorf("table cursor: read next cell: %w", err))
		}
		payload, err := c.db.readPayload(cell.LocalPayload, cell.PayloadSize, cell.OverflowPageNumber)
		if err != nil {
			return fail(fmt.Errorf("table cursor: page %d: cell %d: rowId %d: assemble payload: %w", c.leaf.pageNum, cellIndex, int64(cell.RowId), err))
		}
		record, err := ParseRecord(payload)
		if err != nil {
			return fail(fmt.Errorf("table cursor: page %d: cell %d: rowId %d: decode record: %w", c.leaf.pageNum, cellIndex, int64(cell.RowId), err))
		}
		return TableRecord{RowID: int64(cell.RowId), Record: record}, nil
	}
}
