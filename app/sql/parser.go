package sql

import (
	"github.com/xwb1989/sqlparser"
)

func IsSql(command string) bool {
	_, err := sqlparser.Parse(command)
	if err != nil {
		return false
	}
	return true
}
