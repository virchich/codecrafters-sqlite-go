package main

import (
	"fmt"
	"os"

	"github.com/codecrafters-io/sqlite-starter-go/app/cmd"
	"github.com/codecrafters-io/sqlite-starter-go/app/sql"
)

func main() {
	databaseFilePath := os.Args[1]
	command := os.Args[2]

	switch command {
	case ".dbinfo":
		cmd.Dbinfo(databaseFilePath)
	case ".tables":
		cmd.Tables(databaseFilePath)
	default:
		if sql.IsSql(command) {
			cmd.Sql(databaseFilePath, command)
		} else {
			fmt.Println("Unknown command", command)
			os.Exit(1)
		}
	}
}
