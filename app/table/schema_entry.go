package table

// SchemaEntry describes an object in sqlite_schema. SQL is nil for NULL.
type SchemaEntry struct {
	Type      string
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
	if entry.Type, err = row.String("type"); err != nil {
		return SchemaEntry{}, err
	}
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
