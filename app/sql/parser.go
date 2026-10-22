package sql

import (
	"fmt"
	"strings"

	"github.com/rqlite/sql"
)

// parseStatement requires exactly one statement so trailing SQL is never ignored.
func parseStatement(command string) (sql.Statement, error) {
	statements, err := sql.NewParser(strings.NewReader(command)).ParseStatements()
	if err != nil {
		return nil, err
	}
	if len(statements) != 1 {
		return nil, fmt.Errorf("expected exactly one SQL statement")
	}
	return statements[0], nil
}

// IsSql reports whether command parses as exactly one SQL statement.
// It does not check whether the query executor supports that statement.
func IsSql(command string) bool {
	_, err := parseStatement(command)
	return err == nil
}

// TableDefinition preserves the metadata needed to interpret stored records.
type TableDefinition struct {
	Columns          []string
	RowIDColumn      int // -1 means no declared column aliases rowId
	WithoutRowID     bool
	ColumnCollations map[string]string // Declared collations, keyed by column name.
}

// IndexDefinition retains the metadata needed to choose a supported index.
type IndexDefinition struct {
	TableName       string
	Columns         []string // Indexed columns in declared order
	Partial         bool
	FirstDescending bool
	FirstExpression bool
	FirstCollation  string // Empty means inherit the table column collation.
}

// ParseCreateTableSqlCommand extracts column order, the rowid alias, and the
// WITHOUT ROWID flag from a CREATE TABLE statement with column definitions.
func ParseCreateTableSqlCommand(command string) (TableDefinition, error) {
	definition := TableDefinition{RowIDColumn: -1}
	stmt, err := parseStatement(command)
	if err != nil {
		return definition, err
	}
	ddl, ok := stmt.(*sql.CreateTableStatement)
	if !ok || len(ddl.Columns) == 0 {
		return definition, fmt.Errorf("expected CREATE TABLE with column definitions")
	}
	definition.WithoutRowID = ddl.Without.IsValid()
	definition.Columns = make([]string, len(ddl.Columns))
	for i, col := range ddl.Columns {
		definition.Columns[i] = col.Name.Name
		for _, constraint := range col.Constraints {
			if collate, ok := constraint.(*sql.CollateConstraint); ok {
				if definition.ColumnCollations == nil {
					definition.ColumnCollations = make(map[string]string)
				}
				definition.ColumnCollations[col.Name.Name] = collate.Collation.Name
			}
		}
	}
	if definition.WithoutRowID {
		return definition, nil
	}
	for i, col := range ddl.Columns {
		// INTEGER affinity alone is insufficient: the declared type must be exact.
		if col.Type == nil || !strings.EqualFold(col.Type.Name.Name, "INTEGER") || col.Type.Lparen.IsValid() {
			continue
		}
		for _, constraint := range col.Constraints {
			if pk, ok := constraint.(*sql.PrimaryKeyConstraint); ok && !pk.Desc.IsValid() {
				definition.RowIDColumn = i
			}
		}
		for _, constraint := range ddl.Constraints {
			pk, ok := constraint.(*sql.PrimaryKeyConstraint)
			if !ok || len(pk.Columns) != 1 {
				continue
			}
			name, ok := pk.Columns[0].X.(*sql.Ident)
			// Table-level PRIMARY KEY(id DESC) still aliases rowid.
			if ok && strings.EqualFold(name.Name, col.Name.Name) {
				definition.RowIDColumn = i
			}
		}
	}
	return definition, nil
}

// ParseCreateIndexSqlCommand extracts the table and ordered terms of CREATE INDEX.
// Plain columns use identifier names; other expressions retain their SQL text.
func ParseCreateIndexSqlCommand(command string) (IndexDefinition, error) {
	definition := IndexDefinition{}
	stmt, err := parseStatement(command)
	if err != nil {
		return definition, err
	}
	ddl, ok := stmt.(*sql.CreateIndexStatement)
	if !ok || len(ddl.Columns) == 0 {
		return definition, fmt.Errorf("expected CREATE INDEX with column definitions")
	}
	definition.TableName = ddl.Table.Name
	definition.Partial = ddl.WhereExpr != nil
	definition.FirstDescending = ddl.Columns[0].Desc.IsValid()
	first := ddl.Columns[0].X
	if collate, ok := first.(*sql.CollateExpr); ok {
		definition.FirstCollation = collate.Collation.Name.Name
		first = collate.X
	}
	_, plainColumn := first.(*sql.Ident)
	definition.FirstExpression = !plainColumn
	definition.Columns = make([]string, len(ddl.Columns))
	for i, col := range ddl.Columns {
		expr := col.X
		if collate, ok := expr.(*sql.CollateExpr); ok {
			expr = collate.X
		}
		if name, ok := expr.(*sql.Ident); ok {
			definition.Columns[i] = name.Name
		} else {
			definition.Columns[i] = col.String()
		}
	}

	return definition, nil
}

// SupportsTextSeek checks the ordering and completeness required by Seek.
func (d IndexDefinition) SupportsTextSeek(table TableDefinition) bool {
	if d.Partial || d.FirstDescending || d.FirstExpression || len(d.Columns) == 0 {
		return false
	}
	collation := d.FirstCollation
	if collation == "" {
		for name, declared := range table.ColumnCollations {
			if strings.EqualFold(name, d.Columns[0]) {
				collation = declared
				break
			}
		}
	}
	return collation == "" || strings.EqualFold(collation, "BINARY")
}
