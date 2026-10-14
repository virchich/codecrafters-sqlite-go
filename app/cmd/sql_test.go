package cmd

import (
	s "github.com/codecrafters-io/sqlite-starter-go/app/sql"
	"testing"
)

func TestFormatValue(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{nil, ""}, {"Ada", "Ada"}, {int64(37), "37"}, {1.5, "1.5"}, {[]byte("blob"), "blob"},
	} {
		if got := formatValue(tc.value); got != tc.want {
			t.Errorf("formatValue(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestMatchesWhere(t *testing.T) {
	for _, tc := range []struct {
		left, right any
		op          s.Operator
		want        bool
	}{
		{"Ada", "Ada", s.Equal, true},
		{"Ada", "Bob", s.Equal, false},
		{"Ada", "Bob", s.NotEqual, true},
		{"Ada", "Bob", s.LessThan, true},
		{int64(3), int64(3), s.LessThanOrEqual, true},
		{int64(4), int64(3), s.GreaterThan, true},
		{int64(3), int64(3), s.GreaterThanOrEqual, true},
		{int64(3), 3.0, s.Equal, true},
		{3.5, int64(3), s.GreaterThan, true},
		{int64(9007199254740993), float64(9007199254740992), s.GreaterThan, true},
		{nil, "Ada", s.NotEqual, false},
		{"Ada", nil, s.Equal, false},
	} {
		got, err := matchesWhere(tc.left, &s.WhereClause{Operator: tc.op, Value: tc.right})
		if err != nil || got != tc.want {
			t.Errorf("%v %s %v = %v, %v; want %v", tc.left, tc.op, tc.right, got, err, tc.want)
		}
	}
	if _, err := matchesWhere("3", &s.WhereClause{Operator: s.Equal, Value: int64(3)}); err == nil {
		t.Fatal("expected incompatible-type error")
	}
}
