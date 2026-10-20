package table

import (
	"reflect"
	"strings"
	"testing"
)

func TestSchemaAndRow(t *testing.T) {
	names := []string{"name", "age", "optional"}
	schema, err := NewSchema(names)
	if err != nil {
		t.Fatal(err)
	}
	names[0] = "changed"
	columns := schema.Columns()
	columns[0] = "changed"
	row, err := NewRow(schema, []any{"Ada", int64(37), nil})
	if err != nil {
		t.Fatal(err)
	}
	indices, err := schema.Resolve([]string{"age", "name", "age"})
	if err != nil {
		t.Fatal(err)
	}
	var got []any
	for _, index := range indices {
		value, ok := row.At(index)
		if !ok {
			t.Fatal("invalid position")
		}
		got = append(got, value)
	}
	if !reflect.DeepEqual(got, []any{int64(37), "Ada", int64(37)}) {
		t.Fatal(got)
	}
	if value, ok := row.Get("optional"); !ok || value != nil {
		t.Fatal("NULL must be present")
	}
	if _, ok := row.Get("missing"); ok {
		t.Fatal("missing column must be absent")
	}
	if _, err := schema.Resolve([]string{"missing"}); err == nil {
		t.Fatal("expected unknown-column error")
	}
	if _, err := row.String("age"); err == nil || !strings.Contains(err.Error(), "age") {
		t.Fatal(err)
	}
	if n, err := row.Int64("age"); err != nil || n != 37 {
		t.Fatalf("%d: %v", n, err)
	}
	if _, err := NewRow(schema, []any{"short"}); err == nil {
		t.Fatal("expected width error")
	}
	if _, err := NewSchema([]string{"x", "x"}); err == nil {
		t.Fatal("expected duplicate error")
	}
}
