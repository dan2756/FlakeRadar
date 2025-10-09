package stats

import (
	"math"
	"slices"
	"testing"
)

func approx(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func seq(n int) []float64 {
	xs := make([]float64, n)
	for i := range xs {
		xs[i] = float64(i + 1)
	}
	return xs
}

func TestPercentile(t *testing.T) {
	tests := []struct {
		name string
		xs   []float64
		p    float64
		want float64
	}{
		{"empty", nil, 50, 0},
		{"single", []float64{7}, 95, 7},
		{"median odd", []float64{1, 2, 3, 4, 5}, 50, 3},
		{"median even", []float64{1, 2, 3, 4}, 50, 2.5},
		{"p95 of 1..20", seq(20), 95, 19.05},
		{"p95 of 1..100", seq(100), 95, 95.05},
		{"p95 of two", []float64{10, 20}, 95, 19.5},
		{"p0 is min", []float64{3, 4, 5}, 0, 3},
		{"p100 is max", []float64{3, 4, 5}, 100, 5},
		{"interpolates", []float64{0, 10}, 25, 2.5},
	}
	for _, tt := range tests {
		if got := Percentile(tt.xs, tt.p); !approx(got, tt.want) {
			t.Errorf("%s: Percentile(%v, %v) = %v, want %v", tt.name, tt.xs, tt.p, got, tt.want)
		}
	}
}

func TestSummarize(t *testing.T) {
	// A textbook sample: population stddev 2, sample stddev sqrt(32/7).
	xs := []float64{9, 2, 5, 4, 4, 7, 4, 5}
	orig := slices.Clone(xs)
	s := Summarize(xs)

	if !slices.Equal(xs, orig) {
		t.Errorf("Summarize modified its input: %v", xs)
	}
	checks := []struct {
		name      string
		got, want float64
	}{
		{"N", float64(s.N), 8},
		{"Min", s.Min, 2},
		{"Max", s.Max, 9},
		{"Mean", s.Mean, 5},
		{"Median", s.Median, 4.5},
		{"P95", s.P95, 8.3},
		{"Stddev", s.Stddev, math.Sqrt(32.0 / 7)},
		{"CV", s.CV, math.Sqrt(32.0/7) / 5},
	}
	for _, c := range checks {
		if !approx(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestSummarizeEdgeCases(t *testing.T) {
	if s := Summarize(nil); s != (Summary{}) {
		t.Errorf("Summarize(nil) = %+v, want zero Summary", s)
	}

	s := Summarize([]float64{42})
	if s.N != 1 || s.Min != 42 || s.Max != 42 || s.Median != 42 || s.P95 != 42 {
		t.Errorf("Summarize([42]) = %+v", s)
	}
	if s.Stddev != 0 || s.CV != 0 {
		t.Errorf("single sample: stddev %v, cv %v, want 0", s.Stddev, s.CV)
	}

	s = Summarize([]float64{3, 3, 3, 3})
	if s.Stddev != 0 || s.CV != 0 {
		t.Errorf("constant sample: stddev %v, cv %v, want 0", s.Stddev, s.CV)
	}

	s = Summarize([]float64{0, 0})
	if s.CV != 0 || math.IsNaN(s.CV) {
		t.Errorf("zero mean: cv = %v, want 0", s.CV)
	}
}
