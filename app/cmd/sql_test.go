package cmd

import "testing"

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
