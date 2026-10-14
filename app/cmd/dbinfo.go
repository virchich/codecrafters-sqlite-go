package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
)

func Dbinfo(databaseFilePath string) (err error) {
	databaseFile, err := os.Open(databaseFilePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if closeErr := databaseFile.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
		}
	}()

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
