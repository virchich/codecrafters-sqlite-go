package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/codecrafters-io/sqlite-starter-go/app/cmd"
)

func run(args []string) (err error) {
	if len(args) != 2 {
		return fmt.Errorf("usage: %s <database> <command>", os.Args[0])
	}
	databaseFilePath, command := args[0], args[1]
	if strings.HasPrefix(command, ".") && command != ".dbinfo" && command != ".tables" {
		return fmt.Errorf("unknown command %q", command)
	}

	switch command {
	case ".dbinfo":
		return cmd.DBInfo(databaseFilePath)
	case ".tables":
		return cmd.Tables(databaseFilePath)
	default:
		return cmd.Sql(databaseFilePath, command)
	}
}

func handleError(err error) {
	if _, e := fmt.Fprintln(os.Stderr, err); e != nil {
		panic(e)
	}
	os.Exit(1)
}
