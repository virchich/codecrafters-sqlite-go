package sql

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/xwb1989/sqlparser"
)

func IsSql(command string) bool {
	if _, err := sqlparser.Parse(command); err != nil {
		return false
	}
	return true
}

func ParseCreateTableSqlCommand(command string) ([]string, error) {
	pkAutoIncRegex := regexp.MustCompile(`(?i)primary\s+key\s+auto_increment`)

	sanitizedSQL := strings.ToLower(command)
	sanitizedSQL = strings.ReplaceAll(sanitizedSQL, "integer", "int")
	sanitizedSQL = strings.ReplaceAll(sanitizedSQL, "autoincrement", "auto_increment")

	// Flip "primary key auto_increment" to "auto_increment primary key"
	sanitizedSQL = pkAutoIncRegex.ReplaceAllString(sanitizedSQL, "auto_increment primary key")

	stmt, err := sqlparser.Parse(sanitizedSQL)
	if err != nil {
		return nil, err
	}

	columns := []string{}

	switch ddl := stmt.(type) {
	case *sqlparser.DDL:
		if ddl.TableSpec != nil {
			for _, col := range ddl.TableSpec.Columns {
				columns = append(columns, col.Name.String())
			}
		}
	default:
		return nil, fmt.Errorf("\"%s\" is not an CREATE TABLE command", command)
	}

	return columns, nil
}
