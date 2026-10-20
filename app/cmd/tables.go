package cmd

import (
	"fmt"
	"strings"

	"github.com/codecrafters-io/sqlite-starter-go/app/parser"
)

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

	tblNames := make([]string, 0, len(dbStats.TableNames)-1)
	for _, name := range dbStats.TableNames {
		if strings.HasPrefix(name, "sqlite_") {
			continue
		}
		tblNames = append(tblNames, name)
	}

	_, err = fmt.Println(strings.Join(tblNames, " "))
	return err
}
