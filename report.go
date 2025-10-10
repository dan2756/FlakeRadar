package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dan2756/flaky/internal/runner"
	"github.com/dan2756/flaky/internal/stats"
)

const (
	labelWidth = 11
	maxListed  = 20 // failed runs listed before the rest are summarized
)

func writeReport(w io.Writer, command string, results []runner.Result) {
	var (
		passes   int
		failed   []runner.Result
		timedOut int
		samples  = make([]float64, 0, len(results))
	)
	for _, r := range results {
		if r.Passed() {
			passes++
		} else {
			failed = append(failed, r)
		}
		if r.TimedOut {
			// Its duration is just the time limit; leave it out of the
			// timing so it doesn't drag the numbers toward the limit.
			timedOut++
			continue
		}
		samples = append(samples, float64(r.Duration))
	}

	field(w, "command", command)
	field(w, "runs", strconv.Itoa(len(results)))
	field(w, "passes", strconv.Itoa(passes))
	field(w, "fails", strconv.Itoa(len(failed)))
	field(w, "pass rate", formatPercent(passes, len(results)))

	fmt.Fprintln(w)
	writeTiming(w, stats.Summarize(samples), timedOut)

	if len(failed) > 0 {
		fmt.Fprintln(w)
		writeFailures(w, failed)
	}
}

// writeTiming writes the duration statistics (in nanoseconds) as a column
// of right-aligned values in a shared unit.
func writeTiming(w io.Writer, s stats.Summary, timedOut int) {
	if s.N == 0 {
		field(w, "timing", "n/a, every run timed out")
		return
	}
	labels := []string{"min", "mean", "median", "p95", "max", "stddev", "cv"}
	values := formatDurations(s.Mean, s.Min, s.Mean, s.Median, s.P95, s.Max, s.Stddev)
	values = append(values, strconv.FormatFloat(100*s.CV, 'f', 1, 64)+"%")
	values = alignRight(values)
	for i, label := range labels {
		field(w, label, values[i])
	}
	if timedOut > 0 {
		field(w, "", fmt.Sprintf("(timing excludes %s)", plural(timedOut, "timed-out run")))
	}
}

func writeFailures(w io.Writer, failed []runner.Result) {
	fmt.Fprintln(w, "failed runs")
	listed := failed[:min(len(failed), maxListed)]
	statuses := make([]string, len(listed))
	statusWidth := 0
	for i, r := range listed {
		statuses[i] = status(r)
		statusWidth = max(statusWidth, len(statuses[i]))
	}
	indexWidth := len(strconv.Itoa(listed[len(listed)-1].Index))
	for i, r := range listed {
		fmt.Fprintf(w, "  run %-*d  %-*s  %s\n", indexWidth, r.Index,
			statusWidth, statuses[i], formatDurations(float64(r.Duration), float64(r.Duration))[0])
	}
	if rest := len(failed) - len(listed); rest > 0 {
		fmt.Fprintf(w, "  ... and %d more\n", rest)
	}
}

// status describes how a failed run ended.
func status(r runner.Result) string {
	if r.TimedOut {
		return "timeout"
	}
	return "exit " + strconv.Itoa(r.ExitCode)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
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
