package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/codecrafters-io/sqlite-starter-go/app/cmd"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: %s <database> <command>", os.Args[0])
	}
	databaseFilePath, command := args[0], args[1]
	switch command {
	case ".dbinfo":
		return cmd.Dbinfo(databaseFilePath)
	case ".tables":
		return cmd.Tables(databaseFilePath)
	default:
		if strings.HasPrefix(command, ".") {
			return fmt.Errorf("unknown command %q", command)
		}
		return cmd.Sql(databaseFilePath, command)
	}
}
