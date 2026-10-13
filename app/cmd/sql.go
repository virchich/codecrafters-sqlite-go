package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
	s "github.com/codecrafters-io/sqlite-starter-go/app/sql"
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

	var tablePage int64 = 0
	var tableColumns []string

	for {
		cell, err := page.GetNextCell()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		tblName := cell.Record.Body.Values[2].(string)

		if tblName == command.From {
			tablePage = cell.Record.Body.Values[3].(int64)
			tableColumns, err = s.ParseCreateTableSqlCommand(cell.Record.Body.Values[4].(string))
			if err != nil {
				log.Fatalln(err)
			}
			break
		}
	}

	if tablePage == 0 {
		fmt.Printf("table %s does not exist\n", command.From)
		return
	}

	requestedTablePage, err := parser.NewPage(uint32(tablePage), dbHeader.PageSize)
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
	} else if command.Select == "*" {

	} else {
		columns := strings.Split(command.Select, ",")

		for {
			cell, err := requestedTablePage.GetNextCell()
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Fatal(err)
			}

			values := []string{}

			for i := 0; i < len(columns); i++ {
				for j := 0; j < len(tableColumns); j++ {
					if columns[i] == tableColumns[j] {
						values = append(values, cell.Record.Body.Values[j].(string))
					}
				}
			}

			fmt.Println(strings.Join(values, "|"))
		}
	}
}
