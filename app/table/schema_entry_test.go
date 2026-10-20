package table

import "testing"

func TestDecodeSchemaEntryType(t *testing.T) {
	for _, kind := range []EntryType{Table, Index} {
		t.Run(string(kind), func(t *testing.T) {
			entry, err := DecodeSchemaEntry([]any{string(kind), "object", "people", int64(3), nil})
			if err != nil {
				t.Fatal(err)
			}
			if entry.Type != kind || entry.RootPage != 3 || entry.SQL != nil {
				t.Fatalf("unexpected entry: %+v", entry)
			}
		})
	}
}

func TestDecodeSchemaEntryInvalidType(t *testing.T) {
	if _, err := DecodeSchemaEntry([]any{int64(1), "object", "people", int64(3), nil}); err == nil {
		t.Fatal("accepted a non-text schema type")
	}
}
