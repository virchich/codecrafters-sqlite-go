package sql

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xwb1989/sqlparser"
)

type SqlLine struct {
	Select string
	From   string
	// Where is nil when the query has no filter.
	Where *WhereClause
}

// Operator identifies a supported WHERE comparison. Its zero value is invalid.
type Operator string

const (
	Equal              Operator = "="
	NotEqual           Operator = "!="
	LessThan           Operator = "<"
	LessThanOrEqual    Operator = "<="
	GreaterThan        Operator = ">"
	GreaterThanOrEqual Operator = ">="
)

// WhereClause describes one column-to-literal comparison, independent of the
// SQL parser. Value is a string, int64, float64, or nil (SQL NULL).
type WhereClause struct {
	Column   string
	Operator Operator
	Value    any
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
		if selectStmt.Where != nil {
			sl.Where, err = parseWhere(selectStmt.Where.Expr)
			if err != nil {
				return SqlLine{}, fmt.Errorf("WHERE clause: %w", err)
			}
		}
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

func parseWhere(expr sqlparser.Expr) (*WhereClause, error) {
	if group, ok := expr.(*sqlparser.ParenExpr); ok {
		return parseWhere(group.Expr)
	}
	comparison, ok := expr.(*sqlparser.ComparisonExpr)
	if !ok {
		return nil, fmt.Errorf("expected a single column-to-literal comparison")
	}
	operator := Operator(comparison.Operator)
	switch operator {
	case Equal, NotEqual, LessThan, LessThanOrEqual, GreaterThan, GreaterThanOrEqual:
	default:
		return nil, fmt.Errorf("unsupported operator %q", comparison.Operator)
	}
	column, ok := comparison.Left.(*sqlparser.ColName)
	if !ok || !column.Qualifier.IsEmpty() {
		return nil, fmt.Errorf("expected an unqualified column on the left")
	}
	value, err := searchValue(comparison.Right)
	if err != nil {
		return nil, err
	}
	return &WhereClause{Column: column.Name.String(), Operator: operator, Value: value}, nil
}

func searchValue(expr sqlparser.Expr) (any, error) {
	switch value := expr.(type) {
	case *sqlparser.NullVal:
		return nil, nil
	case *sqlparser.SQLVal:
		switch value.Type {
		case sqlparser.StrVal:
			return string(value.Val), nil
		case sqlparser.IntVal:
			return strconv.ParseInt(string(value.Val), 10, 64)
		case sqlparser.FloatVal:
			return strconv.ParseFloat(string(value.Val), 64)
		}
	case *sqlparser.UnaryExpr:
		if value.Operator == "+" || value.Operator == "-" {
			if literal, ok := value.Expr.(*sqlparser.SQLVal); ok {
				text := value.Operator + string(literal.Val)
				switch literal.Type {
				case sqlparser.IntVal:
					return strconv.ParseInt(text, 10, 64)
				case sqlparser.FloatVal:
					return strconv.ParseFloat(text, 64)
				}
			}
		}
	}
	return nil, fmt.Errorf("expected a string, number, or NULL search value")
}
