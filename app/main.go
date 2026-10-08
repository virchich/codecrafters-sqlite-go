package main

import (
	"fmt"
	"os"

	"github.com/codecrafters-io/sqlite-starter-go/app/cmd"
)

func main() {
	databaseFilePath := os.Args[1]
	command := os.Args[2]

	switch command {
	case ".dbinfo":
		cmd.Dbinfo(databaseFilePath)
	default:
		fmt.Println("Unknown command", command)
		os.Exit(1)
	}
}
