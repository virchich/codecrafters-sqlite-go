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

func IsSql(command string) bool {
	_, err := parseStatement(command)
	return err == nil
}

// TableDefinition preserves the metadata needed to interpret stored records.
type TableDefinition struct {
	Columns      []string
	RowIDColumn  int // -1 means no declared column aliases rowId
	WithoutRowID bool
}

type IndexDefinition struct {
	TableName string
	Columns   []string // Indexed columns in declared order
}

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
	definition.Columns = make([]string, len(ddl.Columns))
	for i, col := range ddl.Columns {
		if name, ok := col.X.(*sql.Ident); ok {
			definition.Columns[i] = name.Name
		} else {
			definition.Columns[i] = col.String()
		}
	}

	return definition, nil
}
