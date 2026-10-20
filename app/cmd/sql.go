package cmd

import (
	"cmp"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
	"github.com/codecrafters-io/sqlite-starter-go/app/sql"
	"github.com/codecrafters-io/sqlite-starter-go/app/table"
)

// Sql executes a supported SELECT using a table scan and writes results to stdout.
// Rows are pipe-separated; COUNT(*) prints one number.
func Sql(databaseFilePath string, sqlString string) error {
	db, err := parser.Open(databaseFilePath)
	if err != nil {
		return err
	}
	defer func(db *parser.DB) {
		err := db.Close()
		if err != nil {
			panic(err)
		}
	}(&db)

	s, err := sql.NewSqlLine(sqlString)
	if err != nil {
		return err
	}

	ti, err := db.LookupTable(s.From)
	if err != nil {
		return err
	}

	if !ti.TableExists {
		return fmt.Errorf("table %s does not exist", s.From)
	}
	if ti.TableDefinition.WithoutRowID {
		return fmt.Errorf("table %q: WITHOUT ROWID tables are not supported", s.From)
	}

	schema, err := table.NewRowIDSchema(ti.TableDefinition.Columns, ti.TableDefinition.RowIDColumn)
	if err != nil {
		return err
	}

	c, err := db.NewTableCursor(ti.TableRootPage)
	if err != nil {
		return err
	}

	return executeTableQuery(c, schema, &s)
}

// executeTableQuery applies filtering before aggregation or projection.
func executeTableQuery(c *parser.TableCursor, schema *table.Schema, s *sql.SqlLine) error {
	onlyCount := strings.EqualFold(s.Select, "COUNT(*)")
	filterIndex := -1
	if s.Where != nil {
		positions, err := schema.Resolve([]string{s.Where.Column})
		if err != nil {
			return fmt.Errorf("WHERE clause: %w", err)
		}
		filterIndex = positions[0]
	}
	var indices []int
	if !onlyCount {
		columns := strings.Split(s.Select, ",")
		if s.Select == "*" {
			columns = schema.Columns()
		}
		var err error
		indices, err = schema.Resolve(columns)
		if err != nil {
			return err
		}
	}

	count := 0
	for {
		tr, err := c.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		row, err := table.NewRowWithRowID(schema, tr.Record.Body, tr.RowID)
		if err != nil {
			return err
		}
		if s.Where != nil {
			value, _ := row.At(filterIndex)
			matches, err := matchesWhere(value, s.Where)
			if err != nil {
				return fmt.Errorf("WHERE column %q: %w", s.Where.Column, err)
			}
			if !matches {
				continue
			}
		}
		if onlyCount {
			count++
			continue
		}
		values := make([]string, len(indices))
		for i, index := range indices {
			value, _ := row.At(index)
			values[i] = formatValue(value)
		}
		if _, err := fmt.Println(strings.Join(values, "|")); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	if onlyCount {
		if _, err := fmt.Println(count); err != nil {
			return fmt.Errorf("write count: %w", err)
		}
	}
	return nil
}

// formatValue renders decoded values for the command's pipe-separated output.
func formatValue(value any) string {
	switch value := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(value)
	default:
		return fmt.Sprint(value)
	}
}

// matchesWhere compares decoded values without converting numbers to strings.
// NULL comparisons do not select rows. Text/numeric coercion is not supported.
func matchesWhere(value any, where *sql.WhereClause) (bool, error) {
	if value == nil || where.Value == nil {
		return false, nil
	}
	var order int
	if left, ok := value.(string); ok {
		right, ok := where.Value.(string)
		if !ok {
			return false, fmt.Errorf("cannot compare %T with %T", value, where.Value)
		}
		order = strings.Compare(left, right)
	} else if left, ok := value.(int64); ok {
		if right, ok := where.Value.(int64); ok {
			order = cmp.Compare(left, right)
		} else {
			leftNumber, rightNumber := numericValue(value), numericValue(where.Value)
			if rightNumber == nil {
				return false, fmt.Errorf("cannot compare %T with %T", value, where.Value)
			}
			order = leftNumber.Cmp(rightNumber)
		}
	} else {
		left, right := numericValue(value), numericValue(where.Value)
		if left == nil || right == nil {
			return false, fmt.Errorf("cannot compare %T with %T", value, where.Value)
		}
		order = left.Cmp(right)
	}
	switch where.Operator {
	case sql.Equal:
		return order == 0, nil
	case sql.NotEqual:
		return order != 0, nil
	case sql.LessThan:
		return order < 0, nil
	case sql.LessThanOrEqual:
		return order <= 0, nil
	case sql.GreaterThan:
		return order > 0, nil
	case sql.GreaterThanOrEqual:
		return order >= 0, nil
	default:
		return false, fmt.Errorf("unsupported operator %q", where.Operator)
	}
}

// numericValue represents a number exactly for mixed integer/float comparisons.
// It returns nil for unsupported values or non-finite floats.
func numericValue(value any) *big.Rat {
	switch value := value.(type) {
	case int64:
		return new(big.Rat).SetInt64(value)
	case float64:
		return new(big.Rat).SetFloat64(value)
	default:
		return nil
	}
}
