package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
	s "github.com/codecrafters-io/sqlite-starter-go/app/sql"
	"github.com/codecrafters-io/sqlite-starter-go/app/table"
)

func Sql(databaseFilePath string, sql string) {
	databaseFile, err := os.Open(databaseFilePath)
	if err != nil {
		log.Fatal(err)
	}
	defer databaseFile.Close()

	dbHeader := parser.DbHeader{}
	if err := dbHeader.Parse(databaseFile); err != nil {
		log.Fatal(err)
	}

	page, err := parser.NewPage(1, dbHeader.PageSize)
	if err != nil {
		log.Fatal(err)
	}
	if err := page.Parse(databaseFile); err != nil {
		log.Fatal(err)
	}

	command, err := s.NewSqlLine(sql)
	if err != nil {
		log.Fatalln(err)
	}

	var tblPage int64 = 0
	var tableColumns []string

	for {
		cell, err := page.GetNextCell()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		entry, err := table.DecodeSchemaEntry(cell.Record.Body.Values)
		if err != nil {
			log.Fatal(err)
		}
		if entry.Type == "table" && entry.TableName == command.From {
			tblPage = entry.RootPage
			if entry.SQL == nil {
				log.Fatalf("table %q has no CREATE TABLE SQL", command.From)
			}
			tableColumns, err = s.ParseCreateTableSqlCommand(*entry.SQL)
			if err != nil {
				log.Fatal(err)
			}
			break
		}
	}

	if tblPage == 0 {
		fmt.Printf("table %s does not exist\n", command.From)
		return
	}

	if tblPage < 1 || tblPage > 1<<32-1 {
		log.Fatalf("invalid root page: %d", tblPage)
	}
	requestedTablePage, err := parser.NewPage(uint32(tblPage), dbHeader.PageSize)
	if err != nil {
		log.Fatal(err)
	}
	if err := requestedTablePage.Parse(databaseFile); err != nil {
		log.Fatal(err)
	}

	if strings.ToUpper(command.Select) == "COUNT(*)" {
		count := 0

		for {
			_, err := requestedTablePage.GetNextCell()
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Fatal(err)
			}
			count++
		}

		fmt.Println(count)
		return
	}

	schema, err := table.NewSchema(tableColumns)
	if err != nil {
		log.Fatal(err)
	}
	columns := strings.Split(command.Select, ",")
	if command.Select == "*" {
		columns = schema.Columns()
	}
	indices, err := schema.Resolve(columns)
	if err != nil {
		log.Fatal(err)
	}

	for {
		cell, err := requestedTablePage.GetNextCell()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		row, err := table.NewRow(schema, cell.Record.Body.Values)
		if err != nil {
			log.Fatal(err)
		}
		values := make([]string, len(indices))
		for i, index := range indices {
			value, _ := row.At(index) // Resolve and NewRow validated these positions.
			values[i] = formatValue(value)
		}
		fmt.Println(strings.Join(values, "|"))
	}
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
