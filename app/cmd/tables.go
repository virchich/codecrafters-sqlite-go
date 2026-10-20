package cmd

import (
	"fmt"
	"strings"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
)

// Tables writes space-separated user table names and a newline to stdout.
// Names starting with sqlite_ are excluded.
func Tables(databaseFilePath string) error {
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

	_, err = fmt.Println(strings.Join(dbStats.TableNames, " "))
	return err
}
