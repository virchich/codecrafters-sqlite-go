package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
	"github.com/codecrafters-io/sqlite-starter-go/app/table"
)

func Tables(databaseFile *os.File) error {
	header := parser.DbHeader{}
	if err := header.Parse(databaseFile); err != nil {
		return fmt.Errorf("read database header: %w", err)
	}

	page, err := parser.NewPage(1, header.PageSize)
	if err != nil {
		return err
	}
	if err := page.Parse(databaseFile); err != nil {
		return fmt.Errorf("read schema page: %w", err)
	}

	tableNames := []string{}

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
		if entry.Type != "table" {
			continue
		}
		tableName := entry.TableName
		if strings.HasPrefix(tableName, "sqlite_") {
			continue
		}
		tableNames = append(tableNames, tableName)
	}

	_, err = fmt.Println(strings.Join(tableNames, " "))
	return err
}
