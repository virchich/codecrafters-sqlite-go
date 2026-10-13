package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
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

	command := strings.Split(sql, " ")
	tableName := command[len(command)-1]

	var tablePage int64 = 0

	for {
		cell, err := page.GetNextCell()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		tblName := cell.Record.Body.Values[2].(string)

		if tblName == tableName {
			tablePage = cell.Record.Body.Values[3].(int64)
			break
		}
	}

	if tablePage == 0 {
		fmt.Printf("table %s does not exist", tableName)
		return
	}

	requestedTablePage, err := parser.NewPage(uint32(tablePage), dbHeader.PageSize)
	if err := requestedTablePage.Parse(databaseFile); err != nil {
		log.Fatal(err)
	}

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
}
