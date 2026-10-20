package cmd

import (
	"fmt"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
)

// DBInfo writes the page size and user-table count to stdout.
func DBInfo(databaseFilePath string) error {
	db, err := parser.Open(databaseFilePath)
	if err != nil {
		return err
	}
	defer func(db *parser.DB) {
		err := db.Close()
		if err != nil {
			panic(err)
		}
	}(&db)

	dbStats, err := db.DBStats()
	if err != nil {
		return err
	}

	_, err = fmt.Printf("database page size: %d\nnumber of tables: %d\n", db.Header.PageSize, dbStats.TotalNumberOfTables)
	return err
}
