package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const labelWidth = 11

func writeReport(w io.Writer, command string, results []result) {
	var (
		passes int
		failed []result
		total  time.Duration
		lo, hi = results[0].duration, results[0].duration
	)
	for _, r := range results {
		if r.passed() {
			passes++
		} else {
			failed = append(failed, r)
		}
		total += r.duration
		lo = min(lo, r.duration)
		hi = max(hi, r.duration)
	}
	mean := total / time.Duration(len(results))

	field(w, "command", command)
	field(w, "runs", strconv.Itoa(len(results)))
	field(w, "passes", strconv.Itoa(passes))
	field(w, "fails", strconv.Itoa(len(failed)))
	field(w, "pass rate", formatPercent(passes, len(results)))

	fmt.Fprintln(w)
	ds := formatDurations(mean, lo, mean, hi)
	field(w, "min", ds[0])
	field(w, "mean", ds[1])
	field(w, "max", ds[2])

	if len(failed) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "failed runs")
		width := len(strconv.Itoa(failed[len(failed)-1].index))
		for _, r := range failed {
			fmt.Fprintf(w, "  run %-*d  exit %d\n", width, r.index, r.exitCode)
		}
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

// formatDurations formats ds in one unit, picked to suit ref, and pads them
// to a common width so they line up in a column.
func formatDurations(ref time.Duration, ds ...time.Duration) []string {
	unit, suffix, prec := time.Second, "s", 3
	switch {
	case ref < time.Millisecond:
		unit, suffix, prec = time.Microsecond, "µs", 1
	case ref < time.Second:
		unit, suffix, prec = time.Millisecond, "ms", 1
	}
	out := make([]string, len(ds))
	width := 0
	for i, d := range ds {
		out[i] = strconv.FormatFloat(float64(d)/float64(unit), 'f', prec, 64) + suffix
		width = max(width, len(out[i]))
	}
	for i, s := range out {
		out[i] = strings.Repeat(" ", width-len(s)) + s
	}
	return out
}
