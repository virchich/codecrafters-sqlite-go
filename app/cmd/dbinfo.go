package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
)

func Dbinfo(databaseFilePath string) {
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

	fmt.Printf("database page size: %d\n", header.PageSize)
	fmt.Printf("number of tables: %d\n", page.Header.NumOfCellsOnPage)
}
