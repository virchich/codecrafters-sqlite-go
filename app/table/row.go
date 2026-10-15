package table

import (
	"fmt"
	"strings"
)

// Schema holds column order and a lookup shared by all rows of a table.
// Its contents are immutable after construction.
type Schema struct {
	columns     []string
	index       map[string]int
	rowIDColumn int
	hasRowID    bool
}

func NewSchema(columns []string) (*Schema, error) {
	s := &Schema{columns: append([]string(nil), columns...), index: make(map[string]int, len(columns)), rowIDColumn: -1}
	for i, name := range columns {
		name = strings.ToLower(name)
		if _, exists := s.index[name]; exists {
			return nil, fmt.Errorf("duplicate column %q", name)
		}
		s.index[name] = i
	}
	return s, nil
}

// NewRowIDSchema describes a rowid table. rowIDColumn is -1 if no declared
// column aliases the rowid. Hidden names resolve only when not shadowed.
func NewRowIDSchema(columns []string, rowIDColumn int) (*Schema, error) {
	if rowIDColumn < -1 || rowIDColumn >= len(columns) {
		return nil, fmt.Errorf("invalid rowid column %d", rowIDColumn)
	}
	schema, err := NewSchema(columns)
	if err != nil {
		return nil, err
	}
	schema.hasRowID = true
	schema.rowIDColumn = rowIDColumn
	for _, name := range []string{"rowid", "_rowid_", "oid"} {
		if _, exists := schema.index[name]; !exists {
			schema.index[name] = len(columns)
		}
	}
	return schema, nil
}

// Columns returns column names in storage order, without exposing the schema's slice.
func (s *Schema) Columns() []string {
	return append([]string(nil), s.columns...)
}

// Resolve validates a projection once, preserving requested order and repetitions.
func (s *Schema) Resolve(names []string) ([]int, error) {
	indices := make([]int, len(names))
	for i, name := range names {
		index, ok := s.index[strings.ToLower(name)]
		if !ok {
			return nil, fmt.Errorf("unknown column %q", name)
		}
		indices[i] = index
	}
	return indices, nil
}

type Row struct {
	schema *Schema
	values []any
	rowID  int64
}

// NewRow borrows values, including any BLOB slices. Callers must not mutate them
// while the row is in use. The schema can be shared across rows.
func NewRow(schema *Schema, values []any) (Row, error) {
	return newRow(schema, values, 0, false)
}

// NewRowWithRowID combines the stored record with its table-leaf key.
func NewRowWithRowID(schema *Schema, values []any, rowID int64) (Row, error) {
	return newRow(schema, values, rowID, true)
}

func newRow(schema *Schema, values []any, rowID int64, supplied bool) (Row, error) {
	if schema == nil {
		return Row{}, fmt.Errorf("row requires a schema")
	}
	if schema.hasRowID && !supplied {
		return Row{}, fmt.Errorf("rowid table requires a rowid")
	}
	if len(values) != len(schema.columns) {
		return Row{}, fmt.Errorf("row has %d values, schema has %d columns", len(values), len(schema.columns))
	}
	return Row{schema: schema, values: values, rowID: rowID}, nil
}

// Get distinguishes a present NULL (nil, true) from an unknown column.
func (r Row) Get(name string) (any, bool) {
	if r.schema == nil {
		return nil, false
	}
	index, ok := r.schema.index[strings.ToLower(name)]
	if !ok {
		return nil, false
	}
	return r.At(index)
}

func (r Row) At(index int) (any, bool) {
	if r.schema != nil && r.schema.hasRowID && (index == len(r.values) || (index >= 0 && index == r.schema.rowIDColumn)) {
		return r.rowID, true
	}
	if index < 0 || index >= len(r.values) {
		return nil, false
	}
	return r.values[index], true
}

func (r Row) String(name string) (string, error) {
	value, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("unknown column %q", name)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("column %q: expected string, got %T", name, value)
	}
	return text, nil
}

func (r Row) Int64(name string) (int64, error) {
	value, ok := r.Get(name)
	if !ok {
		return 0, fmt.Errorf("unknown column %q", name)
	}
	number, ok := value.(int64)
	if !ok {
		return 0, fmt.Errorf("column %q: expected int64, got %T", name, value)
	}
	return number, nil
}
