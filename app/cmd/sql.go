package cmd

import (
	"cmp"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
	s "github.com/codecrafters-io/sqlite-starter-go/app/sql"
	"github.com/codecrafters-io/sqlite-starter-go/app/table"
)

func Sql(databaseFile *os.File, sql string) error {
	dbHeader := parser.DbHeader{}
	if err := dbHeader.Parse(databaseFile); err != nil {
		return fmt.Errorf("read database header: %w", err)
	}

	page, err := parser.NewPage(1, dbHeader.PageSize)
	if err != nil {
		return err
	}
	if err := page.Parse(databaseFile); err != nil {
		return fmt.Errorf("read schema page: %w", err)
	}

	command, err := s.NewSqlLine(sql)
	if err != nil {
		return err
	}

	var tblPage int64
	found := false
	var tableColumns []string

	for {
		cell, err := page.GetNextTableLeafCell()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		entry, err := table.DecodeSchemaEntry(cell.Record.Body.Values)
		if err != nil {
			return err
		}
		if entry.Type == "table" && entry.TableName == command.From {
			found = true
			tblPage = entry.RootPage
			if entry.SQL == nil {
				return fmt.Errorf("table %q has no CREATE TABLE SQL", command.From)
			}
			tableColumns, err = s.ParseCreateTableSqlCommand(*entry.SQL)
			if err != nil {
				return err
			}
			break
		}
	}

	if !found {
		return fmt.Errorf("table %q does not exist", command.From)
	}

	if tblPage < 1 || tblPage > 1<<32-1 {
		return fmt.Errorf("invalid root page: %d", tblPage)
	}
	requestedTablePage, err := parser.NewPage(uint32(tblPage), dbHeader.PageSize)
	if err != nil {
		return err
	}
	if err := requestedTablePage.Parse(databaseFile); err != nil {
		return err
	}

	schema, err := table.NewSchema(tableColumns)
	if err != nil {
		return err
	}
	filterIndex := -1
	if command.Where != nil {
		positions, err := schema.Resolve([]string{command.Where.Column})
		if err != nil {
			return fmt.Errorf("WHERE clause: %w", err)
		}
		filterIndex = positions[0]
	}
	countOnly := strings.EqualFold(command.Select, "COUNT(*)")
	var indices []int
	if !countOnly {
		columns := strings.Split(command.Select, ",")
		if command.Select == "*" {
			columns = schema.Columns()
		}
		indices, err = schema.Resolve(columns)
		if err != nil {
			return err
		}
	}
	count := 0

	for {
		cell, err := requestedTablePage.GetNextTableLeafCell()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		row, err := table.NewRow(schema, cell.Record.Body.Values)
		if err != nil {
			return err
		}
		if command.Where != nil {
			value, _ := row.At(filterIndex)
			matches, err := matchesWhere(value, command.Where)
			if err != nil {
				return fmt.Errorf("WHERE column %q: %w", command.Where.Column, err)
			}
			if !matches {
				continue
			}
		}
		if countOnly {
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
	if countOnly {
		_, err = fmt.Println(count)
		return err
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
func matchesWhere(value any, where *s.WhereClause) (bool, error) {
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
	case s.Equal:
		return order == 0, nil
	case s.NotEqual:
		return order != 0, nil
	case s.LessThan:
		return order < 0, nil
	case s.LessThanOrEqual:
		return order <= 0, nil
	case s.GreaterThan:
		return order > 0, nil
	case s.GreaterThanOrEqual:
		return order >= 0, nil
	default:
		return false, fmt.Errorf("unsupported operator %q", where.Operator)
	}
}

// Exact numeric conversion avoids rounding int64 values when comparing to floats.
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
