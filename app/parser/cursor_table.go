package parser

import (
	"fmt"
	"io"
)

// TableRecord combines a decoded record with its signed table rowId.
type TableRecord struct {
	RowID  int64
	Record Record
}

// TableCursor scans a table B-tree in rowId order and assembles overflow records.
// It maintains its own traversal state and is not safe for concurrent use.
type TableCursor struct {
	db        *DB
	stack     []frame
	leaf      *Page
	exhausted bool
	failure   error
}

// frame tracks an interior page and its next unvisited child, including the rightmost child.
type frame struct {
	page      Page
	nextChild int
	rowID     int64
}

// NewTableCursor positions a scan at the leftmost leaf of a table B-tree.
// db must remain open for the lifetime of the cursor.
func NewTableCursor(rootPage uint32, db *DB) (TableCursor, error) {
	if rootPage == 0 {
		return TableCursor{}, fmt.Errorf("table cursor: invalid root page: %d", rootPage)
	}
	if db == nil || db.file == nil {
		return TableCursor{}, fmt.Errorf("table cursor: database file is required")
	}

	c := TableCursor{db: db}
	if err := c.descendToLeaf(rootPage); err != nil {
		return TableCursor{}, fmt.Errorf("table cursor: initialize root page %d: %w", rootPage, err)
	}
	return c, nil
}

// descendToLeaf follows leftmost children, saving parents for later traversal.
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
			c.stack = append(c.stack, frame{page: p, nextChild: 1, rowID: int64(cell.RowId)})
		default:
			return fmt.Errorf("table cursor: page %d: expected table page, got %#x", p.pageNum, p.Header.PageType)
		}
	}
}

// advanceToNextLeaf visits the next unvisited subtree, or returns io.EOF.
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

// Next returns the next decoded record in rowId order, or io.EOF after the scan.
// After a failure, subsequent calls return that failure instead of skipping it.
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
