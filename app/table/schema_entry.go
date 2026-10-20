package table

type EntryType string

const (
	TableEntry EntryType = "table"
	IndexEntry EntryType = "index"
)

// SchemaEntry describes an object in sqlite_schema. SQL is nil for NULL.
type SchemaEntry struct {
	Type      EntryType
	Name      string
	TableName string
	RootPage  int64
	SQL       *string
}

var catalogSchema = func() *Schema {
	schema, err := NewSchema([]string{"type", "name", "tbl_name", "rootpage", "sql"})
	if err != nil {
		panic(err)
	}
	return schema
}()

func DecodeSchemaEntry(values []any) (SchemaEntry, error) {
	row, err := NewRow(catalogSchema, values)
	if err != nil {
		return SchemaEntry{}, err
	}
	var entry SchemaEntry
	entryType, err := row.String("type")
	if err != nil {
		return SchemaEntry{}, err
	}
	entry.Type = EntryType(entryType)
	if entry.Name, err = row.String("name"); err != nil {
		return SchemaEntry{}, err
	}
	if entry.TableName, err = row.String("tbl_name"); err != nil {
		return SchemaEntry{}, err
	}
	if entry.RootPage, err = row.Int64("rootpage"); err != nil {
		return SchemaEntry{}, err
	}
	if value, _ := row.Get("sql"); value != nil {
		sql, err := row.String("sql")
		if err != nil {
			return SchemaEntry{}, err
		}
		entry.SQL = &sql
	}
	return entry, nil
}
