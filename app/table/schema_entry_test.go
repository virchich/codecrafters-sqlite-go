package table

import (
	"reflect"
	"strings"
	"testing"
)

func TestDecodeSchemaEntry(t *testing.T) {
	statement := "CREATE TABLE people(name TEXT)"
	for _, tc := range []struct {
		name   string
		values []any
		want   SchemaEntry
	}{
		{"table", []any{"table", "people", "people", int64(2), statement},
			SchemaEntry{Type: TableEntry, Name: "people", TableName: "people", RootPage: 2, SQL: &statement}},
		{"implicit index", []any{"index", "idx", "people", int64(3), nil},
			SchemaEntry{Type: IndexEntry, Name: "idx", TableName: "people", RootPage: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeSchemaEntry(tc.values)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("entry=%+v err=%v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestDecodeSchemaEntryInvalidFields(t *testing.T) {
	for _, tc := range []struct {
		column   string
		position int
		value    any
	}{
		{"type", 0, int64(1)},
		{"name", 1, nil},
		{"tbl_name", 2, int64(1)},
		{"rootpage", 3, "wrong"},
		{"sql", 4, int64(1)},
	} {
		t.Run(tc.column, func(t *testing.T) {
			values := []any{"table", "people", "people", int64(2), nil}
			values[tc.position] = tc.value
			if _, err := DecodeSchemaEntry(values); err == nil || !strings.Contains(err.Error(), tc.column) {
				t.Fatalf("expected error identifying %s, got %v", tc.column, err)
			}
		})
	}
}
