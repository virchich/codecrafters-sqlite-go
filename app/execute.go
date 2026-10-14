package main

import (
	"errors"
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

	databaseFile, err := openDatabase(databaseFilePath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := databaseFile.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
		}
	}()

	switch command {
	case ".dbinfo":
		return cmd.Dbinfo(databaseFile)
	case ".tables":
		return cmd.Tables(databaseFile)
	default:
		return cmd.Sql(databaseFile, command)
	}
}

// openDatabase opens a database for reading. The caller owns the returned file.
func openDatabase(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return file, nil
}

func handleError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
