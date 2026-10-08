package cmd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
)

func Dbinfo(databaseFilePath string) {
	databaseFile, err := os.Open(databaseFilePath)
	if err != nil {
		log.Fatal(err)
	}

	page1 := make([]byte, 4095)

	_, err = databaseFile.Read(page1)
	if err != nil {
		log.Fatal(err)
	}

	var pageSize uint16
	if err := binary.Read(bytes.NewReader(page1[16:18]), binary.BigEndian, &pageSize); err != nil {
		fmt.Println("Failed to read integer:", err)
		return
	}
	fmt.Printf("database page size: %d\n", pageSize)

	var numOfTables uint16
	if err := binary.Read(bytes.NewReader(page1[103:105]), binary.BigEndian, &numOfTables); err != nil {
		fmt.Println("Failed to read integer:", err)
		return
	}
	fmt.Printf("number of tables: %d\n", numOfTables)
}
