package cmd

import (
	"fmt"
	"os"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
)

func Dbinfo(databaseFile *os.File) error {
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

	_, err = fmt.Printf("database page size: %d\nnumber of tables: %d\n", header.PageSize, page.Header.NumOfCellsOnPage)
	return err
}
