package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
	"github.com/codecrafters-io/sqlite-starter-go/app/table"
)

func Tables(databaseFilePath string) {
	databaseFile, err := os.Open(databaseFilePath)
	if err != nil {
		log.Fatal(err)
	}
	defer databaseFile.Close()

	header := parser.DbHeader{}
	if err := header.Parse(databaseFile); err != nil {
		log.Fatal(err)
	}

	page, err := parser.NewPage(1, header.PageSize)
	if err != nil {
		log.Fatal(err)
	}
	if err := page.Parse(databaseFile); err != nil {
		log.Fatal(err)
	}

	tableNames := []string{}

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
		if entry.Type != "table" {
			continue
		}
		tableName := entry.TableName
		if strings.HasPrefix(tableName, "sqlite_") {
			continue
		}
		tableNames = append(tableNames, tableName)
	}

	fmt.Println(strings.Join(tableNames, " "))
}
