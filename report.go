package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dan2756/flaky/internal/stats"
)

const labelWidth = 11

func writeReport(w io.Writer, command string, results []result) {
	var (
		passes  int
		failed  []result
		samples = make([]float64, 0, len(results))
	)
	for _, r := range results {
		if r.passed() {
			passes++
		} else {
			failed = append(failed, r)
		}
		samples = append(samples, float64(r.duration))
	}

	field(w, "command", command)
	field(w, "runs", strconv.Itoa(len(results)))
	field(w, "passes", strconv.Itoa(passes))
	field(w, "fails", strconv.Itoa(len(failed)))
	field(w, "pass rate", formatPercent(passes, len(results)))

	fmt.Fprintln(w)
	writeTiming(w, stats.Summarize(samples))

	if len(failed) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "failed runs")
		width := len(strconv.Itoa(failed[len(failed)-1].index))
		for _, r := range failed {
			fmt.Fprintf(w, "  run %-*d  exit %d\n", width, r.index, r.exitCode)
		}
	}
}

// writeTiming writes the duration statistics (in nanoseconds) as a column
// of right-aligned values in a shared unit.
func writeTiming(w io.Writer, s stats.Summary) {
	labels := []string{"min", "mean", "median", "p95", "max", "stddev", "cv"}
	values := formatDurations(s.Mean, s.Min, s.Mean, s.Median, s.P95, s.Max, s.Stddev)
	values = append(values, strconv.FormatFloat(100*s.CV, 'f', 1, 64)+"%")
	values = alignRight(values)
	for i, label := range labels {
		field(w, label, values[i])
	}
}

func field(w io.Writer, label, value string) {
	fmt.Fprintf(w, "%-*s%s\n", labelWidth, label, value)
}

// formatPercent formats n/total as a percentage. Rather than round 99.97%
// up to 100.0% (or 0.03% down to 0.0%), which would hide a failure (or a
// pass), it adds decimals until the difference shows.
func formatPercent(n, total int) string {
	if total == 0 {
		return "-"
	}
	rate := 100 * float64(n) / float64(total)
	var s string
	for prec := 1; prec <= 6; prec++ {
		s = strconv.FormatFloat(rate, 'f', prec, 64)
		hidesFail := n != total && strings.HasPrefix(s, "100")
		hidesPass := n != 0 && strings.Trim(s, "0.") == ""
		if !hidesFail && !hidesPass {
			break
		}
	}
	return s + "%"
}

// formatDurations formats nanosecond values in one unit, picked to suit ref.
func formatDurations(ref float64, ns ...float64) []string {
	unit, suffix, prec := time.Second, "s", 3
	switch {
	case ref < float64(time.Millisecond):
		unit, suffix, prec = time.Microsecond, "µs", 1
	case ref < float64(time.Second):
		unit, suffix, prec = time.Millisecond, "ms", 1
	}
	out := make([]string, len(ns))
	for i, v := range ns {
		out[i] = strconv.FormatFloat(v/float64(unit), 'f', prec, 64) + suffix
	}
	return out
}

// alignRight pads values on the left to a common width.
func alignRight(values []string) []string {
	width := 0
	for _, v := range values {
		width = max(width, utf8.RuneCountInString(v))
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.Repeat(" ", width-utf8.RuneCountInString(v)) + v
	}
	return out
}
