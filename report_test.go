package main

import "testing"

func TestFormatPercent(t *testing.T) {
	tests := []struct {
		n, total int
		want     string
	}{
		{10, 10, "100.0%"},
		{9, 10, "90.0%"},
		{0, 10, "0.0%"},
		{2, 3, "66.7%"},
		{9999, 10000, "99.99%"},
		{1, 100000, "0.001%"},
		{0, 0, "-"},
	}
	for _, tt := range tests {
		if got := formatPercent(tt.n, tt.total); got != tt.want {
			t.Errorf("formatPercent(%d, %d) = %q, want %q", tt.n, tt.total, got, tt.want)
		}
	}
}
