package sql_test

import (
	"reflect"
	"testing"

	"github.com/codecrafters-io/sqlite-starter-go/app/sql"
)

func TestSQLiteCreateTable(t *testing.T) {
	for _, tc := range []struct {
		statement string
		columns   []string
	}{
		{`CREATE TABLE "superheroes" (id integer primary key autoincrement, name text not null, eye_color text, hair_color text, appearance_count integer, first_appearance text, first_appearance_year text)`, []string{"id", "name", "eye_color", "hair_color", "appearance_count", "first_appearance", "first_appearance_year"}},
		{`CREATE TABLE "MixedCase" ("IntegerName" TEXT DEFAULT 'A,B', "Eye Color" TEXT, amount DECIMAL(10,2), PRIMARY KEY ("IntegerName", "Eye Color"))`, []string{"IntegerName", "Eye Color", "amount"}},
	} {
		got, err := sql.ParseCreateTableSqlCommand(tc.statement)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Columns, tc.columns) {
			t.Fatalf("columns = %v, want %v", got, tc.columns)
		}
	}
}

func TestSQLiteSelect(t *testing.T) {
	for _, query := range []string{
		`SELECT id, name FROM superheroes WHERE eye_color = 'Pink Eyes'`,
		`SELECT "id", "name" FROM "superheroes" WHERE "eye_color" = 'Pink Eyes';`,
	} {
		got, err := sql.NewSqlLine(query)
		want := sql.SqlLine{Select: "id,name", From: "superheroes", Where: &sql.WhereClause{Column: "eye_color", Operator: sql.Equal, Value: "Pink Eyes"}}
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("query = %+v, want %+v", got, want)
		}
	}
}

func TestRejectTrailingStatements(t *testing.T) {
	if sql.IsSql(`SELECT name FROM people; SELECT age FROM people`) {
		t.Fatal("accepted multiple statements")
	}
	if _, err := sql.NewSqlLine(`SELECT name FROM people; SELECT age FROM people`); err == nil {
		t.Fatal("accepted multiple statements")
	}
}

func TestRowIDDefinitions(t *testing.T) {
	for _, tc := range []struct {
		ddl     string
		column  int
		without bool
	}{
		{`CREATE TABLE t(name TEXT, id INTEGER PRIMARY KEY AUTOINCREMENT)`, 1, false},
		{`CREATE TABLE t(id integer PRIMARY KEY)`, 0, false},
		{`CREATE TABLE t(id INTEGER PRIMARY KEY ASC)`, 0, false},
		{`CREATE TABLE t(id INTEGER PRIMARY KEY DESC)`, -1, false},
		{`CREATE TABLE t(id INTEGER, PRIMARY KEY(id DESC))`, 0, false},
		{`CREATE TABLE t("Key" INTEGER, PRIMARY KEY("KEY"))`, 0, false},
		{`CREATE TABLE t(id INT PRIMARY KEY)`, -1, false},
		{`CREATE TABLE t(id BIGINT PRIMARY KEY)`, -1, false},
		{`CREATE TABLE t(id INTEGER(8) PRIMARY KEY)`, -1, false},
		{`CREATE TABLE t(id INTEGER, name TEXT, PRIMARY KEY(id, name))`, -1, false},
		{`CREATE TABLE t(id INTEGER PRIMARY KEY) WITHOUT ROWID`, -1, true},
		{`CREATE TABLE t(id INTEGER, name TEXT)`, -1, false},
	} {
		t.Run(tc.ddl, func(t *testing.T) {
			got, err := sql.ParseCreateTableSqlCommand(tc.ddl)
			if err != nil {
				t.Fatal(err)
			}
			if got.RowIDColumn != tc.column || got.WithoutRowID != tc.without {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
