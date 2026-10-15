package sql

import (
	"fmt"
	"strconv"
	"strings"

	sqlparser "github.com/rqlite/sql"
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

func NewSqlLine(command string) (SqlLine, error) {
	stmt, err := parseStatement(command)
	if err != nil {
		return SqlLine{}, err
	}
	query, ok := stmt.(*sqlparser.SelectStatement)
	if !ok {
		return SqlLine{}, fmt.Errorf("not a SELECT statement")
	}
	source, ok := query.Source.(*sqlparser.QualifiedTableName)
	if !ok || source.Schema != nil {
		return SqlLine{}, fmt.Errorf("expected a single unqualified table")
	}
	if query.WithClause != nil || query.Distinct.IsValid() || query.Compound != nil || len(query.GroupByExprs) != 0 || query.HavingExpr != nil || len(query.Windows) != 0 || len(query.OrderingTerms) != 0 || query.LimitExpr != nil || query.OffsetExpr != nil {
		return SqlLine{}, fmt.Errorf("unsupported SELECT clause")
	}
	sl := SqlLine{From: source.Name.Name}
	if query.WhereExpr != nil {
		sl.Where, err = parseWhere(query.WhereExpr)
		if err != nil {
			return SqlLine{}, fmt.Errorf("WHERE clause: %w", err)
		}
	}
	columns := make([]string, 0, len(query.Columns))
	for _, column := range query.Columns {
		if column.Star.IsValid() {
			if len(query.Columns) != 1 {
				return SqlLine{}, fmt.Errorf("star must be the only selected expression")
			}
			columns = append(columns, "*")
			continue
		}
		switch expr := column.Expr.(type) {
		case *sqlparser.Ident:
			columns = append(columns, expr.Name)
		case *sqlparser.Call:
			if !strings.EqualFold(expr.Name.Name, "count") || !expr.Star.IsValid() || expr.Filter != nil || expr.Over != nil || len(query.Columns) != 1 {
				return SqlLine{}, fmt.Errorf("unsupported SELECT expression")
			}
			columns = append(columns, "COUNT(*)")
		default:
			return SqlLine{}, fmt.Errorf("unsupported SELECT expression")
		}
	}
	sl.Select = strings.Join(columns, ",")
	return sl, nil
}

func parseWhere(expr sqlparser.Expr) (*WhereClause, error) {
	if group, ok := expr.(*sqlparser.ParenExpr); ok {
		return parseWhere(group.X)
	}
	comparison, ok := expr.(*sqlparser.BinaryExpr)
	if !ok {
		return nil, fmt.Errorf("expected a single column-to-literal comparison")
	}
	operator := Operator(comparison.Op.String())
	switch operator {
	case Equal, NotEqual, LessThan, LessThanOrEqual, GreaterThan, GreaterThanOrEqual:
	default:
		return nil, fmt.Errorf("unsupported operator %q", operator)
	}
	column, ok := comparison.X.(*sqlparser.Ident)
	if !ok {
		return nil, fmt.Errorf("expected an unqualified column on the left")
	}
	value, err := searchValue(comparison.Y)
	if err != nil {
		return nil, err
	}
	return &WhereClause{Column: column.Name, Operator: operator, Value: value}, nil
}

func searchValue(expr sqlparser.Expr) (any, error) {
	switch value := expr.(type) {
	case *sqlparser.NullLit:
		return nil, nil
	case *sqlparser.StringLit:
		return value.Value, nil
	case *sqlparser.NumberLit:
		return parseNumber(value.Value)
	case *sqlparser.UnaryExpr:
		if value.Op == sqlparser.PLUS || value.Op == sqlparser.MINUS {
			if literal, ok := value.X.(*sqlparser.NumberLit); ok {
				return parseNumber(value.Op.String() + literal.Value)
			}
		}
	}
	return nil, fmt.Errorf("expected a string, number, or NULL search value")
}

func parseNumber(value string) (any, error) {
	if strings.ContainsAny(value, ".eE") {
		return strconv.ParseFloat(value, 64)
	}
	return strconv.ParseInt(value, 10, 64)
}
