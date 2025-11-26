package main

import (
	"slices"
	"testing"
	"time"

	"github.com/dan2756/flaky/internal/runner"
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

func TestMeetsThreshold(t *testing.T) {
	tests := []struct {
		passes, runs int
		threshold    float64
		want         bool
	}{
		{10, 10, 100, true},
		{9, 10, 100, false},
		{19, 20, 95, true}, // exactly on the line
		{18, 20, 95, false},
		{1, 3, 33.3, true},
		{1, 3, 33.34, false},
		{2, 3, 66.66, true},
		{0, 10, 0, true},
		{0, 0, 0, false}, // nothing ran, nothing passed
		{9999, 10000, 99.99, true},
		{9998, 10000, 99.99, false},
	}
	for _, tt := range tests {
		if got := meetsThreshold(tt.passes, tt.runs, tt.threshold); got != tt.want {
			t.Errorf("meetsThreshold(%d, %d, %v) = %v, want %v", tt.passes, tt.runs, tt.threshold, got, tt.want)
		}
	}
}

func TestVerdict(t *testing.T) {
	tests := []struct {
		passes, runs int
		threshold    float64
		want         string
	}{
		{20, 20, 100, "PASS  20/20 runs passed"},
		{18, 20, 100, "FAIL  18/20 runs passed (threshold 100%)"},
		{18, 20, 80, "PASS  18/20 runs passed (threshold 80%)"},
		{15, 20, 80, "FAIL  15/20 runs passed (threshold 80%)"},
		{199, 200, 99.5, "PASS  199/200 runs passed (threshold 99.5%)"},
	}
	for _, tt := range tests {
		s := summary{runs: tt.runs, passes: tt.passes, threshold: tt.threshold}
		if got := s.verdict(); got != tt.want {
			t.Errorf("verdict for %d/%d at %v%% = %q, want %q", tt.passes, tt.runs, tt.threshold, got, tt.want)
		}
	}
}

func TestSummarizeLeavesTimeoutsOutOfTiming(t *testing.T) {
	results := []runner.Result{
		{Index: 1, Duration: 100 * time.Millisecond},
		{Index: 2, Duration: 300 * time.Millisecond, ExitCode: 1},
		{Index: 3, Duration: 30 * time.Second, ExitCode: -1, TimedOut: true},
	}
	s := summarize(results, 100)
	if s.runs != 3 || s.passes != 1 || len(s.failed) != 2 || s.timedOut != 1 {
		t.Errorf("summary = %+v", s)
	}
	if s.timing.N != 2 || s.timing.Max != float64(300*time.Millisecond) {
		t.Errorf("timing = %+v, want the two runs that finished", s.timing)
	}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		r    runner.Result
		want string
	}{
		{runner.Result{ExitCode: 1}, "exit 1"},
		{runner.Result{ExitCode: 2}, "exit 2"},
		{runner.Result{ExitCode: -1, TimedOut: true}, "timeout"},
		{runner.Result{ExitCode: 139, Signal: "SIGSEGV"}, "killed by SIGSEGV"},
	}
	for _, tt := range tests {
		if got := status(tt.r); got != tt.want {
			t.Errorf("status(%+v) = %q, want %q", tt.r, got, tt.want)
		}
	}
}
