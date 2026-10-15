package table

import (
	"reflect"
	"testing"
)

func TestRowIDAliasAndShadowing(t *testing.T) {
	schema, err := NewRowIDSchema([]string{"name", "id", "RoWiD", "optional"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	raw := []any{"Ada", nil, "shadow", nil}
	row, err := NewRowWithRowID(schema, raw, -42)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]any{"id": int64(-42), "ID": int64(-42), "rowid": "shadow", "_rowid_": int64(-42), "oid": int64(-42), "optional": nil} {
		got, ok := row.Get(name)
		if !ok || got != want {
			t.Errorf("Get(%q) = %v, %v; want %v", name, got, ok, want)
		}
		positions, err := schema.Resolve([]string{name})
		if err != nil {
			t.Fatal(err)
		}
		got, ok = row.At(positions[0])
		if !ok || got != want {
			t.Errorf("At resolved %q = %v, %v", name, got, ok)
		}
	}
	if raw[1] != nil {
		t.Fatal("modified physical record")
	}
	if !reflect.DeepEqual(schema.Columns(), []string{"name", "id", "RoWiD", "optional"}) {
		t.Fatal("hidden columns leaked into star")
	}
	if _, err := NewRow(schema, raw); err == nil {
		t.Fatal("missing rowid accepted")
	}
}

func TestRowIDWithoutDeclaredAlias(t *testing.T) {
	schema, err := NewRowIDSchema([]string{"id"}, -1)
	if err != nil {
		t.Fatal(err)
	}
	row, err := NewRowWithRowID(schema, []any{nil}, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := row.Get("id"); got != nil {
		t.Fatal("ordinary NULL replaced")
	}
	if got, _ := row.Get("rowid"); got != int64(7) {
		t.Fatalf("rowid = %v", got)
	}
	for _, index := range []int{-2, -1, 2} {
		if _, ok := row.At(index); ok {
			t.Fatalf("accepted index %d", index)
		}
	}
}

func TestRowIDSchemaValidation(t *testing.T) {
	for _, index := range []int{-2, 1, 2} {
		if _, err := NewRowIDSchema([]string{"id"}, index); err == nil {
			t.Errorf("accepted alias index %d", index)
		}
	}
	if _, err := NewRowIDSchema([]string{"id", "ID"}, 0); err == nil {
		t.Fatal("accepted duplicate names")
	}
	if _, err := NewRowWithRowID(nil, nil, 1); err == nil {
		t.Fatal("accepted nil schema")
	}
	schema, err := NewRowIDSchema([]string{"id"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRowWithRowID(schema, nil, 1); err == nil {
		t.Fatal("accepted wrong record width")
	}
	for _, id := range []int64{0, -1, -1 << 63, 1<<63 - 1} {
		row, err := NewRowWithRowID(schema, []any{nil}, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"id", "ROWID", "_ROWID_", "OID"} {
			got, err := row.Int64(name)
			if err != nil || got != id {
				t.Errorf("%s = %d, %v; want %d", name, got, err, id)
			}
		}
	}
}

func TestAllRowIDNamesShadowed(t *testing.T) {
	schema, err := NewRowIDSchema([]string{"rowid", "_rowid_", "oid"}, -1)
	if err != nil {
		t.Fatal(err)
	}
	row, err := NewRowWithRowID(schema, []any{nil, "text", int64(9)}, 42)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]any{"rowid": nil, "_rowid_": "text", "oid": int64(9)} {
		got, ok := row.Get(name)
		if !ok || got != want {
			t.Errorf("%s = %v,%v; want %v", name, got, ok, want)
		}
	}
}
