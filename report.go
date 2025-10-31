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

// summary is what the report says about a set of results.
type summary struct {
	runs      int
	passes    int
	failed    []runner.Result
	timedOut  int           // timed-out runs, left out of timing
	timing    stats.Summary // run durations in nanoseconds
	threshold float64       // required pass rate in percent
}

func summarize(results []runner.Result, threshold float64) summary {
	s := summary{runs: len(results), threshold: threshold}
	samples := make([]float64, 0, len(results))
	for _, r := range results {
		if r.Passed() {
			s.passes++
		} else {
			s.failed = append(s.failed, r)
		}
		if r.TimedOut {
			// Its duration is just the time limit; leave it out of the
			// timing so it doesn't drag the numbers toward the limit.
			s.timedOut++
			continue
		}
		samples = append(samples, float64(r.Duration))
	}
	s.timing = stats.Summarize(samples)
	return s
}

// passRate is the percentage of runs that passed, 0 if there were none.
func (s summary) passRate() float64 {
	if s.runs == 0 {
		return 0
	}
	return 100 * float64(s.passes) / float64(s.runs)
}

// ok reports whether the pass rate meets the threshold.
func (s summary) ok() bool {
	return meetsThreshold(s.passes, s.runs, s.threshold)
}

// meetsThreshold reports whether passes out of runs is at least threshold
// percent. It compares passes*100 with threshold*runs rather than the
// rounded pass rate, so 19 of 20 meets a threshold of 95.
func meetsThreshold(passes, runs int, threshold float64) bool {
	if runs == 0 {
		return false
	}
	return float64(passes)*100 >= threshold*float64(runs)-1e-9
}

// verdict is the report's last line.
func (s summary) verdict() string {
	word := "PASS"
	if !s.ok() {
		word = "FAIL"
	}
	v := fmt.Sprintf("%s  %d/%d runs passed", word, s.passes, s.runs)
	if s.threshold < 100 || !s.ok() {
		v += " (threshold " + strconv.FormatFloat(s.threshold, 'f', -1, 64) + "%)"
	}
	return v
}

func writeReport(w io.Writer, command string, s summary) {
	field(w, "command", command)
	field(w, "runs", strconv.Itoa(s.runs))
	field(w, "passes", strconv.Itoa(s.passes))
	field(w, "fails", strconv.Itoa(len(s.failed)))
	field(w, "pass rate", formatPercent(s.passes, s.runs))

	fmt.Fprintln(w)
	writeTiming(w, s.timing, s.timedOut)

	if len(s.failed) > 0 {
		fmt.Fprintln(w)
		writeFailures(w, s.failed)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, s.verdict())
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
