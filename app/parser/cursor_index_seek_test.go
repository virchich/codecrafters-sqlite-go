package parser

import (
	"encoding/binary"
	"io"
	"reflect"
	"testing"
)

func seekTextCell(value string, rowid byte, child uint32) []byte {
	payload := append([]byte{3, byte(13 + 2*len(value)), 1}, []byte(value)...)
	payload = append(payload, rowid)
	cell := append([]byte{byte(len(payload))}, payload...)
	if child != 0 {
		prefix := make([]byte, 4)
		binary.BigEndian.PutUint32(prefix, child)
		cell = append(prefix, cell...)
	}
	return cell
}

func TestIndexCursorSeek(t *testing.T) {
	data := make([]byte, 10*512)
	indexTestPage(data, 2, IndexInteriorPage, 4, seekTextCell("eritrea", 4, 3))
	indexTestPage(data, 3, IndexInteriorPage, 7, seekTextCell("eritrea", 2, 5), seekTextCell("eritrea", 3, 6))
	indexTestPage(data, 4, IndexInteriorPage, 9, seekTextCell("myanmar", 6, 8))
	indexTestPage(data, 5, IndexLeafPage, 0, seekTextCell("egypt", 1, 0))
	indexTestPage(data, 6, IndexLeafPage, 0)
	indexTestPage(data, 7, IndexLeafPage, 0)
	indexTestPage(data, 8, IndexLeafPage, 0, seekTextCell("eritrea", 5, 0))
	indexTestPage(data, 9, IndexLeafPage, 0, seekTextCell("spain", 7, 0))
	indexTestPage(data, 10, IndexLeafPage, 0)
	all := []string{"egypt", "eritrea", "eritrea", "eritrea", "eritrea", "myanmar", "spain"}
	for _, root := range []uint32{2, 5, 10} {
		c, err := NewIndexCursor(root, cursorTestDB(t, data, 0))
		if err != nil {
			t.Fatal(err)
		}
		values := all
		if root == 5 {
			values = all[:1]
		}
		if root == 10 {
			values = nil
		}
		// Reuse the cursor, including seeking backward after EOF.
		for _, target := range []string{"eritrea", "z", "", "myanmar", "eritreaa", "egypt", "spain", "a"} {
			if err := c.Seek(target); err != nil {
				t.Fatal(err)
			}
			for i, value := range values {
				if value < target {
					continue
				}
				got, err := c.Next()
				if err != nil || !reflect.DeepEqual(got.Record.Body, []any{value, int64(i + 1)}) {
					t.Fatalf("root=%d target=%q: got=%v err=%v want=(%s,%d)", root, target, got.Record.Body, err, value, i+1)
				}
			}
			for i := 0; i < 2; i++ {
				if _, err := c.Next(); err != io.EOF {
					t.Fatalf("root=%d target=%q: expected EOF, got %v", root, target, err)
				}
			}
		}
	}
}

func TestIndexCursorSeekFailure(t *testing.T) {
	var uninitialized IndexCursor
	if err := uninitialized.Seek("x"); err == nil {
		t.Fatal("accepted uninitialized cursor")
	}
	data := make([]byte, 2*512)
	indexTestPage(data, 2, IndexLeafPage, 0, []byte{1, 0})
	c, err := NewIndexCursor(2, cursorTestDB(t, data, 0))
	if err != nil {
		t.Fatal(err)
	}
	first := c.Seek("x")
	if first == nil {
		t.Fatal("accepted malformed record")
	}
	if _, again := c.Next(); again != first {
		t.Fatalf("seek failure not retained: %v", again)
	}
}
