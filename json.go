package main

import (
	"encoding/json"
	"io"
	"math"
	"time"

	"github.com/dan2756/flaky/internal/runner"
)

type jsonReport struct {
	Command string      `json:"command"`
	Argv    []string    `json:"argv"`
	Runs    []jsonRun   `json:"runs"`
	Summary jsonSummary `json:"summary"`
}

type jsonRun struct {
	Index      int     `json:"index"`
	ExitCode   int     `json:"exit_code"`
	DurationMS float64 `json:"duration_ms"`
	TimedOut   bool    `json:"timed_out"`
	Passed     bool    `json:"passed"`
}

type jsonSummary struct {
	Runs       int           `json:"runs"`
	Passes     int           `json:"passes"`
	Fails      int           `json:"fails"`
	PassRate   float64       `json:"pass_rate"`
	Threshold  float64       `json:"threshold"`
	OK         bool          `json:"ok"`
	TimedRuns  int           `json:"timed_runs"`
	DurationMS jsonDurations `json:"duration_ms"`
	CV         float64       `json:"cv"`
	FailedRuns []int         `json:"failed_runs"`
}

type jsonDurations struct {
	Min    float64 `json:"min"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	P95    float64 `json:"p95"`
	Max    float64 `json:"max"`
	Stddev float64 `json:"stddev"`
}

func writeJSON(w io.Writer, opts *options, results []runner.Result, s summary) error {
	rep := jsonReport{
		Command: opts.display,
		Argv:    opts.argv,
		Runs:    make([]jsonRun, len(results)),
		Summary: jsonSummary{
			Runs:      s.runs,
			Passes:    s.passes,
			Fails:     len(s.failed),
			PassRate:  round(s.passRate(), 4),
			Threshold: s.threshold,
			OK:        s.ok(),
			TimedRuns: s.timing.N,
			DurationMS: jsonDurations{
				Min:    ms(s.timing.Min),
				Mean:   ms(s.timing.Mean),
				Median: ms(s.timing.Median),
				P95:    ms(s.timing.P95),
				Max:    ms(s.timing.Max),
				Stddev: ms(s.timing.Stddev),
			},
			CV:         round(s.timing.CV, 4),
			FailedRuns: make([]int, len(s.failed)),
		},
	}
	for i, r := range results {
		rep.Runs[i] = jsonRun{
			Index:      r.Index,
			ExitCode:   r.ExitCode,
			DurationMS: ms(float64(r.Duration)),
			TimedOut:   r.TimedOut,
			Passed:     r.Passed(),
		}
	}
	for i, r := range s.failed {
		rep.Summary.FailedRuns[i] = r.Index
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// ms converts nanoseconds to milliseconds, kept to microsecond precision.
func ms(ns float64) float64 {
	return round(ns/float64(time.Millisecond), 3)
}

func round(x float64, places int) float64 {
	p := math.Pow10(places)
	return math.Round(x*p) / p
}
