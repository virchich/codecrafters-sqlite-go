package parser

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/codecrafters-io/sqlite-starter-go/app/sql"
	"github.com/codecrafters-io/sqlite-starter-go/app/table"
)

type DB struct {
	file   *os.File
	Header DbHeader
}

type DBInfo struct {
	TotalNumberOfTables int
	TableNames          []string
}

type TableInfo struct {
	TableExists     bool
	TableRootPage   uint32
	TableDefinition sql.TableDefinition
	IndexInfo       []IndexInfo
}

type IndexInfo struct {
	IndexRootPage    uint32
	IndexDefinitions sql.IndexDefinition
}

func Open(path string) (DB, error) {
	file, err := os.Open(path)
	if err != nil {
		return DB{}, fmt.Errorf("open database: %w", err)
	}

	dbHeader := DbHeader{}
	if err := dbHeader.Parse(file); err != nil {
		if e := file.Close(); e != nil {
			panic(e)
		}
		return DB{}, fmt.Errorf("parse db header: %w", err)
	}

	return DB{
		file:   file,
		Header: dbHeader,
	}, nil
}

func (db *DB) Close() error {
	return db.file.Close()
}

// DBStats reports user tables, excluding SQLite internal tables.
func (db *DB) DBStats() (DBInfo, error) {
	cursor, err := db.NewTableCursor(1)
	if err != nil {
		return DBInfo{}, fmt.Errorf("database statistics: open schema: %w", err)
	}
	info := DBInfo{}
	for {
		record, err := cursor.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return DBInfo{}, fmt.Errorf("database statistics: read schema: %w", err)
		}
		entry, err := table.DecodeSchemaEntry(record.Record.Body)
		if err != nil {
			return DBInfo{}, fmt.Errorf("database statistics: decode schema row %d: %w", record.RowID, err)
		}
		if entry.Type == table.TableEntry && !strings.HasPrefix(strings.ToLower(entry.Name), "sqlite_") {
			info.TotalNumberOfTables++
			info.TableNames = append(info.TableNames, entry.Name)
		}
	}
	return info, nil
}

// LookupTable returns TableExists=false when the table is absent. IndexInfo
// includes explicit indexes only; implicit indexes require constraint decoding.
func (db *DB) LookupTable(name string) (TableInfo, error) {
	c, err := db.NewTableCursor(1) // sqlite_schema page
	if err != nil {
		return TableInfo{}, fmt.Errorf("lookup table %q: read schema root: %w", name, err)
	}

	ti := TableInfo{TableExists: false}

	for {
		tr, err := c.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return TableInfo{}, fmt.Errorf("lookup table %q: read schema: %w", name, err)
		}

		entry, err := table.DecodeSchemaEntry(tr.Record.Body)
		if err != nil {
			return TableInfo{}, fmt.Errorf("lookup table %q: decode schema row %d: %w", name, tr.RowID, err)
		}

		if !strings.EqualFold(entry.TableName, name) ||
			(entry.Type != table.TableEntry && entry.Type != table.IndexEntry) {
			continue
		}
		if entry.RootPage < 1 || entry.RootPage > 1<<32-1 {
			return TableInfo{}, fmt.Errorf("schema object %q: invalid root page %d", entry.Name, entry.RootPage)
		}

		if entry.Type == table.TableEntry {
			ti.TableExists = true
			ti.TableRootPage = uint32(entry.RootPage)

			if entry.SQL == nil {
				return TableInfo{}, fmt.Errorf("table %q has no CREATE TABLE SQL", name)
			}
			tableDefinition, err := sql.ParseCreateTableSqlCommand(*entry.SQL)
			if err != nil {
				return TableInfo{}, fmt.Errorf("table %q: parse definition: %w", name, err)
			}
			ti.TableDefinition = tableDefinition
		}

		if entry.Type == table.IndexEntry {
			indInf := IndexInfo{}

			indInf.IndexRootPage = uint32(entry.RootPage)

			if entry.SQL == nil {
				// SQLite creates implicit indexes without CREATE INDEX SQL.
				continue
			}
			indexDefinition, err := sql.ParseCreateIndexSqlCommand(*entry.SQL)
			if err != nil {
				return TableInfo{}, fmt.Errorf("index %q: parse definition: %w", entry.Name, err)
			}
			indInf.IndexDefinitions = indexDefinition

			ti.IndexInfo = append(ti.IndexInfo, indInf)
		}
	}

	return ti, nil
}

func (db *DB) NewTableCursor(rootPage uint32) (*TableCursor, error) {
	c, err := NewTableCursor(rootPage, db)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

//func (db *DB) NewIndexCursor(rootPage uint32) (*IndexCursor, error) {}
