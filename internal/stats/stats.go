// Package stats computes summary statistics for a sample of measurements.
package stats

import (
	"math"
	"slices"
)

// Summary describes a sample.
type Summary struct {
	N      int
	Min    float64
	Max    float64
	Mean   float64
	Median float64
	P95    float64
	// Stddev is the sample standard deviation (n-1 in the denominator),
	// since the runs being summarized are a sample of all possible runs.
	Stddev float64
	// CV is the coefficient of variation, Stddev / Mean. It is 0 when the
	// mean is 0.
	CV float64
}

// Summarize computes a Summary of xs without modifying it. An empty sample
// gives the zero Summary.
func Summarize(xs []float64) Summary {
	n := len(xs)
	if n == 0 {
		return Summary{}
	}
	sorted := slices.Clone(xs)
	slices.Sort(sorted)

	var sum float64
	for _, x := range sorted {
		sum += x
	}
	mean := sum / float64(n)

	s := Summary{
		N:      n,
		Min:    sorted[0],
		Max:    sorted[n-1],
		Mean:   mean,
		Median: Percentile(sorted, 50),
		P95:    Percentile(sorted, 95),
		Stddev: stddev(sorted, mean),
	}
	if mean != 0 {
		s.CV = s.Stddev / mean
	}
	return s
}

// Percentile returns the p-th percentile (0 <= p <= 100) of sorted, which
// must be in ascending order. Values between two samples are linearly
// interpolated (the same method as numpy's default and Excel's
// PERCENTILE.INC), so Percentile(sorted, 50) is the median. It returns 0
// for an empty sample.
func Percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	switch {
	case n == 0:
		return 0
	case p <= 0:
		return sorted[0]
	case p >= 100:
		return sorted[n-1]
	}
	rank := p / 100 * float64(n-1)
	lo := int(rank)
	if lo+1 >= n {
		return sorted[n-1]
	}
	frac := rank - float64(lo)
	return sorted[lo] + frac*(sorted[lo+1]-sorted[lo])
}

func stddev(xs []float64, mean float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var ss float64
	for _, x := range xs {
		d := x - mean
		ss += d * d
	}
	return math.Sqrt(ss / float64(len(xs)-1))
}
