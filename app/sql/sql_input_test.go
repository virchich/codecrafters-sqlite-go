package sql_test

import (
	"reflect"
	"testing"

	"github.com/codecrafters-io/sqlite-starter-go/app/sql"
)

func TestNewSqlLineWhere(t *testing.T) {
	for _, tc := range []struct {
		predicate string
		want      sql.WhereClause
	}{
		{"name = 'Ada Lovelace'", sql.WhereClause{"name", sql.Equal, "Ada Lovelace"}},
		{"name = 'O''Brien'", sql.WhereClause{"name", sql.Equal, "O'Brien"}},
		{"name = '37'", sql.WhereClause{"name", sql.Equal, "37"}},
		{"name = ''", sql.WhereClause{"name", sql.Equal, ""}},
		{"age >= 37", sql.WhereClause{"age", sql.GreaterThanOrEqual, int64(37)}},
		{"age = 0", sql.WhereClause{"age", sql.Equal, int64(0)}},
		{"age > -37", sql.WhereClause{"age", sql.GreaterThan, int64(-37)}},
		{"age = -9223372036854775808", sql.WhereClause{"age", sql.Equal, int64(-9223372036854775808)}},
		{"score < 3.5", sql.WhereClause{"score", sql.LessThan, 3.5}},
		{"score <= -3.5", sql.WhereClause{"score", sql.LessThanOrEqual, -3.5}},
		{"(name != 'Ada')", sql.WhereClause{"name", sql.NotEqual, "Ada"}},
		{"name = NULL", sql.WhereClause{"name", sql.Equal, nil}},
	} {
		t.Run(tc.predicate, func(t *testing.T) {
			line, err := sql.NewSqlLine("SELECT name FROM people WHERE " + tc.predicate)
			if err != nil {
				t.Fatal(err)
			}
			if line.Select != "name" || line.From != "people" {
				t.Fatalf("unexpected query: %+v", line)
			}
			if !reflect.DeepEqual(line.Where, &tc.want) {
				t.Fatalf("Where = %+v, want %+v", line.Where, tc.want)
			}
		})
	}
	line, err := sql.NewSqlLine("SELECT name FROM people")
	if err != nil || line.Where != nil {
		t.Fatalf("unexpected filter: %+v, error: %v", line.Where, err)
	}
}

func TestNewSqlLineInvalidWhere(t *testing.T) {
	for _, predicate := range []string{"", "age =", "age = 1 AND name = 'Ada'", "age = 1 OR age = 2", "age = other_age", "1 = age", "age IN (1, 2)", "name LIKE 'A%'", "age IS NULL", "age = 9223372036854775808", "age = ?", "people.age = 1"} {
		if _, err := sql.NewSqlLine("SELECT name FROM people WHERE " + predicate); err == nil {
			t.Fatalf("expected error for %q", predicate)
		}
	}
}
