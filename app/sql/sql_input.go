package sql

import (
	"fmt"
	"strings"

	"github.com/xwb1989/sqlparser"
)

type SqlLine struct {
	Select string
	From   string
}

func NewSqlLine(sql string) (SqlLine, error) {
	stmt, err := sqlparser.Parse(sql)
	if err != nil {
		return SqlLine{}, err
	}

	sl := SqlLine{}
	sel := []string{}

	switch selectStmt := stmt.(type) {
	case *sqlparser.Select:
		for _, expr := range selectStmt.SelectExprs {
			switch selectExpr := expr.(type) {
			case *sqlparser.AliasedExpr:
				sel = append(sel, sqlparser.String(selectExpr.Expr))
				if !selectExpr.As.IsEmpty() {
					sel = append(sel, selectExpr.As.String())
				}
			case *sqlparser.StarExpr:
				sel = append(sel, "*")
			}
		}

		sl.Select = strings.Join(sel, ",")

		for _, tableExpr := range selectStmt.From {
			switch table := tableExpr.(type) {
			case *sqlparser.AliasedTableExpr:
				sl.From = sqlparser.String(table.Expr)
				if !table.As.IsEmpty() {
					sl.From = table.As.String()
				}
			}
		}
	default:
		return SqlLine{}, fmt.Errorf("Not a SELECT statement.")
	}

	return sl, nil
}
