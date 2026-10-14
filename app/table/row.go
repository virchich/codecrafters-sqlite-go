package table

import "fmt"

// Schema holds column order and a lookup shared by all rows of a table.
// Its contents are immutable after construction.
type Schema struct {
	columns []string
	index   map[string]int
}

func NewSchema(columns []string) (*Schema, error) {
	s := &Schema{columns: append([]string(nil), columns...), index: make(map[string]int, len(columns))}
	for i, name := range columns {
		if _, exists := s.index[name]; exists {
			return nil, fmt.Errorf("duplicate column %q", name)
		}
		s.index[name] = i
	}
	return s, nil
}

// Columns returns column names in storage order, without exposing the schema's slice.
func (s *Schema) Columns() []string {
	return append([]string(nil), s.columns...)
}

// Resolve validates a projection once, preserving requested order and repetitions.
func (s *Schema) Resolve(names []string) ([]int, error) {
	indices := make([]int, len(names))
	for i, name := range names {
		index, ok := s.index[name]
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
}

// NewRow borrows values, including any BLOB slices. Callers must not mutate them
// while the row is in use. The schema can be shared across rows.
func NewRow(schema *Schema, values []any) (Row, error) {
	if schema == nil {
		return Row{}, fmt.Errorf("row requires a schema")
	}
	if len(values) != len(schema.columns) {
		return Row{}, fmt.Errorf("row has %d values, schema has %d columns", len(values), len(schema.columns))
	}
	return Row{schema: schema, values: values}, nil
}

// Get distinguishes a present NULL (nil, true) from an unknown column.
func (r Row) Get(name string) (any, bool) {
	if r.schema == nil {
		return nil, false
	}
	index, ok := r.schema.index[name]
	if !ok {
		return nil, false
	}
	return r.At(index)
}

func (r Row) At(index int) (any, bool) {
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
