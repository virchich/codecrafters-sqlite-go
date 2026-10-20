package parser

import (
	"fmt"
	"io"
)

const (
	cellPageNumberLenBytes         = 4
	cellOverflowPageNumberLenBytes = 4
)

// localPayloadLen returns the number of payload bytes stored in the cell's
// B-tree page, excluding cell fields and the overflow page pointer. payloadSize
// is the total payload size, including bytes stored on overflow pages.
//
// isTableLeaf selects table leaf cells when true and index leaf or interior
// cells when false. Table interior cells have no payload and must not use this
// function.
//
// The caller must validate that pageSize is a power of two from 512 through
// 65536 and that pageSize minus reservedBytes is at least 480. pageSize is the
// full page size; do not subtract database or B-tree headers from it.
func localPayloadLen(payloadSize uint64, pageSize uint32, reservedBytes byte, isTableLeaf bool) uint64 {
	U := uint64(pageSize) - uint64(reservedBytes)

	P := payloadSize
	M := ((U - 12) * 32 / 255) - 23

	var X uint64
	if isTableLeaf {
		X = U - 35
	} else {
		X = ((U - 12) * 64 / 255) - 23
	}

	if P <= X {
		return P
	}

	K := M + ((P - M) % (U - 4))
	if K <= X {
		return K
	}

	return M
}

// readPayload assembles a complete record payload for table or index cells.
// The database header must be validated. When no overflow is needed, the
// returned slice borrows localPayload; otherwise it owns the assembled bytes.
func (db *DB) readPayload(localPayload []byte, payloadSize uint64, overflowPageNumber uint32) ([]byte, error) {
	localSize := uint64(len(localPayload))
	if localSize > payloadSize {
		return nil, fmt.Errorf("local payload has %d bytes, total size is %d", localSize, payloadSize)
	}
	if localSize == payloadSize {
		if overflowPageNumber != 0 {
			return nil, fmt.Errorf("unexpected overflow page %d after complete payload", overflowPageNumber)
		}
		return localPayload, nil
	}
	if overflowPageNumber == 0 {
		return nil, fmt.Errorf("missing overflow page with %d payload bytes remaining: %w", payloadSize-localSize, io.ErrUnexpectedEOF)
	}
	info, err := db.file.Stat()
	if err != nil {
		return nil, fmt.Errorf("check database size: %w", err)
	}
	if payloadSize > uint64(^uint(0)>>1) || payloadSize > uint64(info.Size()) {
		return nil, fmt.Errorf("payload size %d exceeds addressable memory or database size %d", payloadSize, info.Size())
	}

	// Grow only as bytes are read; do not allocate the declared total upfront.
	payload := append([]byte(nil), localPayload...)
	seen := make(map[uint32]bool)
	next := overflowPageNumber
	for uint64(len(payload)) < payloadSize {
		if next == 0 {
			return nil, fmt.Errorf("overflow chain ended with %d payload bytes remaining: %w", payloadSize-uint64(len(payload)), io.ErrUnexpectedEOF)
		}
		if seen[next] {
			return nil, fmt.Errorf("overflow chain cycles at page %d", next)
		}
		seen[next] = true
		if uint64(next) > uint64(info.Size())/uint64(db.Header.PageSize) {
			return nil, fmt.Errorf("overflow page %d is outside database: %w", next, io.ErrUnexpectedEOF)
		}
		page, err := NewOverflowPage(next, &db.Header)
		if err != nil {
			return nil, fmt.Errorf("open overflow page %d: %w", next, err)
		}
		if err := page.Parse(db.file); err != nil {
			return nil, fmt.Errorf("read overflow chain: %w", err)
		}
		n := min(payloadSize-uint64(len(payload)), uint64(len(page.Payload)))
		payload = append(payload, page.Payload[:int(n)]...)
		next = page.NextPage
	}
	if next != 0 {
		return nil, fmt.Errorf("overflow chain continues at page %d after complete payload", next)
	}
	return payload, nil
}
