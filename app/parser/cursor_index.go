package parser

import (
	"fmt"
	"io"
)

type IndexRecord struct {
	Record Record
}

type IndexCursor struct {
	rootPage  uint32
	db        *DB
	stack     []indexFrame
	leaf      *Page
	exhausted bool
	failure   error
}

type indexFrame struct {
	page      Page
	nextChild int
	action    action
	cell      *IndexInteriorCell
}

type action uint8

const (
	visitChild action = 0
	returnKey  action = 1
)

func NewIndexCursor(rootPage uint32, db *DB) (IndexCursor, error) {
	if rootPage == 0 {
		return IndexCursor{}, fmt.Errorf("index cursor: invalid root page: %d", rootPage)
	}
	if db == nil || db.file == nil {
		return IndexCursor{}, fmt.Errorf("index cursor: database file is required")
	}

	c := IndexCursor{rootPage: rootPage, db: db}
	pageNumber := rootPage

	if err := c.descendToLeaf(pageNumber); err != nil {
		return IndexCursor{}, fmt.Errorf("index cursor: initialize root page %d: %w", rootPage, err)
	}

	return c, nil
}

func (c *IndexCursor) descendToLeaf(pageNumber uint32) error {
	for {
		for _, ancestor := range c.stack {
			if ancestor.page.pageNum == pageNumber {
				return fmt.Errorf("index cursor: cycle through interior page: %d", pageNumber)
			}
		}

		p, err := NewPage(pageNumber, &c.db.Header)
		if err != nil {
			return fmt.Errorf("index cursor: descend to page %d: %w", pageNumber, err)
		}
		if err := p.Parse(c.db.file); err != nil {
			return fmt.Errorf("index cursor: descend to page %d: %w", pageNumber, err)
		}

		switch p.Header.PageType {
		case IndexLeafPage:
			c.leaf = &p
			return nil
		case IndexInteriorPage:
			cell, err := p.GetNextIndexInteriorCell()
			if err == io.EOF {
				pageNumber = p.Header.RightMostPointer
			} else if err != nil {
				return fmt.Errorf("index cursor: page %d: read first child: %w", pageNumber, err)
			} else {
				pageNumber = cell.PageNumber
			}

			f := indexFrame{page: p, nextChild: 1, action: visitChild}
			if err != io.EOF {
				f.action = returnKey
				f.cell = &cell
			}
			c.stack = append(c.stack, f)
		default:
			return fmt.Errorf("index cursor: page %d: expected index page, got %#x", p.pageNum, p.Header.PageType)
		}
	}
}

// advanceToNextEntry stops at a pending interior entry or a new leaf.
// A nil leaf means the previous leaf has been exhausted.
func (c *IndexCursor) advanceToNextEntry() error {
	for len(c.stack) > 0 {
		top := &c.stack[len(c.stack)-1]
		if top.action == returnKey {
			return nil
		}
		var child uint32
		switch {
		case top.nextChild < len(top.page.cellPointer):
			cell, err := top.page.GetNextIndexInteriorCell()
			if err != nil {
				return fmt.Errorf("index cursor: page %d: read child %d: %w", top.page.pageNum, top.nextChild, err)
			}
			child = cell.PageNumber
			top.cell = &cell
			top.action = returnKey
		case top.nextChild == len(top.page.cellPointer):
			child = top.page.Header.RightMostPointer
			top.cell = nil
		default:
			c.stack = c.stack[:len(c.stack)-1]
			continue
		}
		parent, childIndex := top.page.pageNum, top.nextChild
		top.nextChild++
		if err := c.descendToLeaf(child); err != nil {
			return fmt.Errorf("index cursor: page %d: follow child %d to page %d: %w", parent, childIndex, child, err)
		}
		return nil
	}
	return io.EOF
}

// Next returns leaf and interior entries in index order.
// Failures are retained so subsequent calls do not skip a failed entry.
func (c *IndexCursor) Next() (IndexRecord, error) {
	if c.failure != nil {
		return IndexRecord{}, c.failure
	}
	if c.exhausted {
		return IndexRecord{}, io.EOF
	}
	fail := func(err error) (IndexRecord, error) {
		c.failure = err
		return IndexRecord{}, err
	}
	if c.db == nil || c.db.file == nil || (c.leaf == nil && len(c.stack) == 0) {
		return fail(fmt.Errorf("index cursor: not initialized"))
	}

	for {
		var local []byte
		var size uint64
		var overflow, pageNumber uint32
		var cellIndex int

		if c.leaf != nil {
			cellIndex = c.leaf.cellIdx
			cell, err := c.leaf.GetNextIndexLeafCell()
			if err == io.EOF {
				c.leaf = nil
				continue
			}
			if err != nil {
				return fail(fmt.Errorf("index cursor: read next leaf cell: %w", err))
			}
			local, size, overflow = cell.LocalPayload, cell.PayloadSize, cell.OverflowPageNumber
			pageNumber = c.leaf.pageNum
		} else {
			if err := c.advanceToNextEntry(); err != nil {
				if err == io.EOF {
					c.exhausted = true
					return IndexRecord{}, io.EOF
				}
				return fail(err)
			}
			if c.leaf != nil {
				continue
			}
			top := &c.stack[len(c.stack)-1]
			if top.action != returnKey || top.cell == nil {
				return fail(fmt.Errorf("index cursor: page %d: missing pending interior entry", top.page.pageNum))
			}
			cell := top.cell
			local, size, overflow = cell.LocalPayload, cell.PayloadSize, cell.OverflowPageNumber
			pageNumber, cellIndex = top.page.pageNum, top.page.cellIdx-1
			top.action = visitChild
			top.cell = nil
		}

		payload, err := c.db.readPayload(local, size, overflow)
		if err != nil {
			return fail(fmt.Errorf("index cursor: page %d: cell %d: assemble payload: %w", pageNumber, cellIndex, err))
		}
		record, err := ParseRecord(payload)
		if err != nil {
			return fail(fmt.Errorf("index cursor: page %d: cell %d: decode record: %w", pageNumber, cellIndex, err))
		}
		return IndexRecord{Record: record}, nil
	}
}

// Seek positions Next at the first entry whose first column is >= target.
// It supports ascending UTF-8 text with BINARY collation. If no entry exists,
// Seek succeeds and Next returns io.EOF. Each seek resets prior scan state.
func (c *IndexCursor) Seek(target string) (err error) {
	c.stack = nil
	c.leaf = nil
	c.exhausted = false
	c.failure = nil
	// A failed seek must not leave Next using partially constructed state.
	defer func() { c.failure = err }()
	if c.db == nil || c.db.file == nil || c.rootPage == 0 {
		return fmt.Errorf("index cursor seek: not initialized")
	}

	// Decode the first indexed value, including any overflow payload.
	qualifies := func(local []byte, size uint64, overflow uint32) (bool, error) {
		payload, err := c.db.readPayload(local, size, overflow)
		if err != nil {
			return false, err
		}
		record, err := ParseRecord(payload)
		if err != nil {
			return false, err
		}
		if len(record.Body) == 0 {
			return false, fmt.Errorf("empty index record")
		}
		switch value := record.Body[0].(type) {
		case nil, int64, float64:
			// NULL and numeric values sort before text.
			return false, nil
		case string:
			return value >= target, nil
		case []byte:
			// BLOB values sort after text.
			return true, nil
		default:
			return false, fmt.Errorf("unsupported index value type %T", value)
		}
	}

	pageNumber := c.rootPage
	for {
		for _, ancestor := range c.stack {
			if ancestor.page.pageNum == pageNumber {
				return fmt.Errorf("index cursor seek: cycle through interior page %d", pageNumber)
			}
		}
		page, err := NewPage(pageNumber, &c.db.Header)
		if err != nil {
			return fmt.Errorf("index cursor seek: open page %d: %w", pageNumber, err)
		}
		if err := page.Parse(c.db.file); err != nil {
			return fmt.Errorf("index cursor seek: read page %d: %w", pageNumber, err)
		}
		switch page.Header.PageType {
		case IndexInteriorPage:
			for {
				cell, err := page.GetNextIndexInteriorCell()
				if err == io.EOF {
					c.stack = append(c.stack, indexFrame{page: page, nextChild: len(page.cellPointer) + 1, action: visitChild})
					pageNumber = page.Header.RightMostPointer
					break
				}
				if err != nil {
					return fmt.Errorf("index cursor seek: page %d: read interior cell: %w", pageNumber, err)
				}
				match, err := qualifies(cell.LocalPayload, cell.PayloadSize, cell.OverflowPageNumber)
				if err != nil {
					return fmt.Errorf("index cursor seek: page %d: cell %d: compare record: %w", pageNumber, page.cellIdx-1, err)
				}
				if match {
					c.stack = append(c.stack, indexFrame{page: page, nextChild: page.cellIdx, action: returnKey, cell: &cell})
					pageNumber = cell.PageNumber
					break
				}
			}
		case IndexLeafPage:
			c.leaf = &page
			for {
				cell, err := page.GetNextIndexLeafCell()
				if err == io.EOF {
					return nil
				} // Next resumes at a pending ancestor.
				if err != nil {
					return fmt.Errorf("index cursor seek: page %d: read leaf cell: %w", pageNumber, err)
				}
				match, err := qualifies(cell.LocalPayload, cell.PayloadSize, cell.OverflowPageNumber)
				if err != nil {
					return fmt.Errorf("index cursor seek: page %d: cell %d: compare record: %w", pageNumber, page.cellIdx-1, err)
				}
				if match {
					page.cellIdx-- // Next must return this entry, not skip it.
					return nil
				}
			}
		default:
			return fmt.Errorf("index cursor seek: page %d: expected index page, got %#x", pageNumber, page.Header.PageType)
		}
	}
}
