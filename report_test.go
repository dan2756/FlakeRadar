package main

import (
	"slices"
	"testing"
)

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

func TestFormatDurations(t *testing.T) {
	tests := []struct {
		ref  float64
		ns   []float64
		want []string
	}{
		{1.4e9, []float64{1.2144e9, 30e9}, []string{"1.214s", "30.000s"}},
		{250e6, []float64{123.44e6, 1.5e9}, []string{"123.4ms", "1500.0ms"}},
		{800e3, []float64{812.3e3}, []string{"812.3µs"}},
	}
	for _, tt := range tests {
		got := formatDurations(tt.ref, tt.ns...)
		if !slices.Equal(got, tt.want) {
			t.Errorf("formatDurations(%v, %v) = %q, want %q", tt.ref, tt.ns, got, tt.want)
		}
	}
}

func TestAlignRight(t *testing.T) {
	got := alignRight([]string{"1.2ms", "812.3µs", "12.6%"})
	want := []string{"  1.2ms", "812.3µs", "  12.6%"}
	if !slices.Equal(got, want) {
		t.Errorf("alignRight = %q, want %q", got, want)
	}
}
