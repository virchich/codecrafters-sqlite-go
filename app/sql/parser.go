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

func ParseCreateTableSqlCommand(command string) ([]string, error) {
	stmt, err := parseStatement(command)
	if err != nil {
		return nil, err
	}
	ddl, ok := stmt.(*sql.CreateTableStatement)
	if !ok || len(ddl.Columns) == 0 {
		return nil, fmt.Errorf("expected CREATE TABLE with column definitions")
	}
	columns := make([]string, len(ddl.Columns))
	for i, col := range ddl.Columns {
		columns[i] = col.Name.Name
	}
	return columns, nil
}
