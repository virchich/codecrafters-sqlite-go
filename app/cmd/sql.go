package cmd

import (
	"fmt"
	"io"
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
		cell, err := page.GetNextCell()
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

	if strings.ToUpper(command.Select) == "COUNT(*)" {
		count := 0

		for {
			_, err := requestedTablePage.GetNextCell()
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
			count++
		}

		_, err = fmt.Println(count)
		return err
	}

	schema, err := table.NewSchema(tableColumns)
	if err != nil {
		return err
	}
	columns := strings.Split(command.Select, ",")
	if command.Select == "*" {
		columns = schema.Columns()
	}
	indices, err := schema.Resolve(columns)
	if err != nil {
		return err
	}

	for {
		cell, err := requestedTablePage.GetNextCell()
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
		values := make([]string, len(indices))
		for i, index := range indices {
			value, _ := row.At(index) // Resolve and NewRow validated these positions.
			values[i] = formatValue(value)
		}
		if _, err := fmt.Println(strings.Join(values, "|")); err != nil {
			return fmt.Errorf("write row: %w", err)
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
