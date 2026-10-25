package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// flaky runs the CLI in-process and returns its exit code and output.
func flaky(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(context.Background(), args, console{stdout: &out, stderr: &errOut})
	return code, out.String(), errOut.String()
}

func TestAllPass(t *testing.T) {
	code, out, _ := flaky(t, "-n", "3", "--", "sh", "-c", "exit 0")
	if code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	for _, want := range []string{"runs       3\n", "passes     3\n", "fails      0\n", "pass rate  100.0%\n", "median"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}

func TestFailureShowsOutput(t *testing.T) {
	code, out, _ := flaky(t, "-n", "2", "--show-output", "2", "printf 'a\\nb\\nc\\n'; exit 3")
	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if !strings.HasPrefix(out, "--- output of run 1 (exit 3), last 2 of 3 lines ---\nb\nc\n\n") {
		t.Errorf("output of the first failure not shown first:\n%s", out)
	}
	if strings.Count(out, "--- output of run") != 1 {
		t.Errorf("want only the first failure's output:\n%s", out)
	}
	if !strings.Contains(out, "  run 1  exit 3") || !strings.Contains(out, "  run 2  exit 3") {
		t.Errorf("failed runs not listed:\n%s", out)
	}
}

// alternating returns a shell command that fails on odd runs and passes on
// even ones, counting runs in a file under a fresh temp dir.
func alternating(t *testing.T) string {
	f := filepath.Join(t.TempDir(), "count")
	return fmt.Sprintf(`n=$(cat '%[1]s' 2>/dev/null || echo 0); n=$((n+1)); echo $n > '%[1]s'; [ $((n %% 2)) -eq 0 ]`, f)
}

func TestThreshold(t *testing.T) {
	tests := []struct {
		threshold string
		wantCode  int
		wantLast  string
	}{
		{"100", 1, "FAIL  2/4 runs passed (threshold 100%)\n"},
		{"50", 0, "PASS  2/4 runs passed (threshold 50%)\n"},
		{"50.1", 1, "FAIL  2/4 runs passed (threshold 50.1%)\n"},
		{"0", 0, "PASS  2/4 runs passed (threshold 0%)\n"},
	}
	for _, tt := range tests {
		code, out, _ := flaky(t, "-q", "-n", "4", "--threshold", tt.threshold, alternating(t))
		if code != tt.wantCode {
			t.Errorf("--threshold %s: exit code %d, want %d", tt.threshold, code, tt.wantCode)
		}
		if !strings.HasSuffix(out, tt.wantLast) {
			t.Errorf("--threshold %s: report ends\n%s\nwant %q", tt.threshold, out, tt.wantLast)
		}
	}
}

func TestJSON(t *testing.T) {
	code, out, _ := flaky(t, "--json", "-n", "4", "--threshold", "50", alternating(t))
	if code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	var rep jsonReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(rep.Runs) != 4 {
		t.Fatalf("got %d runs, want 4", len(rep.Runs))
	}
	for i, r := range rep.Runs {
		wantPass := (i+1)%2 == 0
		if r.Index != i+1 || r.Passed != wantPass || (r.ExitCode == 0) != wantPass || r.TimedOut {
			t.Errorf("runs[%d] = %+v", i, r)
		}
		if r.DurationMS <= 0 {
			t.Errorf("runs[%d].duration_ms = %v, want > 0", i, r.DurationMS)
		}
	}
	sum := rep.Summary
	if sum.Runs != 4 || sum.Passes != 2 || sum.Fails != 2 || sum.PassRate != 50 || !sum.OK || sum.TimedRuns != 4 {
		t.Errorf("summary = %+v", sum)
	}
	if !slices.Equal(sum.FailedRuns, []int{1, 3}) {
		t.Errorf("failed_runs = %v, want [1 3]", sum.FailedRuns)
	}
	d := sum.DurationMS
	if !(d.Min > 0 && d.Min <= d.Median && d.Median <= d.P95 && d.P95 <= d.Max) {
		t.Errorf("duration_ms out of order: %+v", d)
	}
	if rep.Argv[0] != "sh" || rep.Argv[1] != "-c" {
		t.Errorf("argv = %q, want the sh -c form", rep.Argv)
	}
}

func TestJSONPassRateDoesNotHideExtremes(t *testing.T) {
	// Four decimal places would turn these into 100 and 0, hiding the one
	// run that went the other way. The text report already refuses to.
	cases := []struct {
		passes, runs int
		threshold    float64
		wantOK       bool
		check        func(t *testing.T, rate float64, raw string)
	}{
		{2, 4, 50, true, func(t *testing.T, rate float64, _ string) {
			if rate != 50 {
				t.Errorf("pass_rate = %v, want 50", rate)
			}
		}},
		{10, 10, 100, true, func(t *testing.T, rate float64, _ string) {
			if rate != 100 {
				t.Errorf("pass_rate = %v, want 100", rate)
			}
		}},
		{0, 10, 0, true, func(t *testing.T, rate float64, _ string) {
			if rate != 0 {
				t.Errorf("pass_rate = %v, want 0", rate)
			}
		}},
		{2, 3, 100, false, func(t *testing.T, rate float64, _ string) {
			if rate != 66.6667 {
				t.Errorf("pass_rate = %v, want 66.6667", rate)
			}
		}},
		{9999999, 10000000, 100, false, func(t *testing.T, rate float64, raw string) {
			if rate >= 100 || strings.Contains(raw, `"pass_rate": 100`) {
				t.Errorf("pass_rate = %v hides the failure:\n%s", rate, raw)
			}
		}},
		{1, 10000000, 0, true, func(t *testing.T, rate float64, raw string) {
			if rate == 0 {
				t.Errorf("pass_rate = 0 hides the pass:\n%s", raw)
			}
		}},
	}
	for _, tt := range cases {
		s := summary{runs: tt.runs, passes: tt.passes, threshold: tt.threshold}
		var buf bytes.Buffer
		if err := writeJSON(&buf, &options{threshold: tt.threshold}, nil, s); err != nil {
			t.Fatalf("writeJSON(%d/%d): %v", tt.passes, tt.runs, err)
		}
		var rep jsonReport
		if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
			t.Fatalf("unmarshal %d/%d: %v\n%s", tt.passes, tt.runs, err, buf.String())
		}
		if rep.Summary.OK != tt.wantOK {
			t.Errorf("%d/%d at %v%%: ok = %v, want %v", tt.passes, tt.runs, tt.threshold, rep.Summary.OK, tt.wantOK)
		}
		tt.check(t, rep.Summary.PassRate, buf.String())
	}
}

func TestJSONTimeout(t *testing.T) {
	code, out, _ := flaky(t, "--json", "-n", "1", "--timeout", "50ms", "--", "sleep", "5")
	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	var rep jsonReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if len(rep.Runs) != 1 || !rep.Runs[0].TimedOut || rep.Runs[0].Passed {
		t.Errorf("runs = %+v, want one timeout", rep.Runs)
	}
	if rep.Summary.TimedRuns != 0 || rep.Summary.DurationMS != (jsonDurations{}) {
		t.Errorf("summary = %+v, want no timed runs", rep.Summary)
	}
}

// failOn returns a shell command that fails on run n, counting runs in a
// file under a fresh temp dir.
func failOn(t *testing.T, n int) string {
	f := filepath.Join(t.TempDir(), "count")
	return fmt.Sprintf(`c=$(cat '%[1]s' 2>/dev/null || echo 0); c=$((c+1)); echo $c > '%[1]s'; [ $c -ne %[2]d ]`, f, n)
}

func TestUntilFail(t *testing.T) {
	code, out, _ := flaky(t, "--until-fail", failOn(t, 30))
	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	for _, want := range []string{
		"runs       30 (stopped at the first failure)\n",
		"passes     29\n",
		"  run 30  exit 1",
		"FAIL  29/30 runs passed (threshold 100%)\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}

func TestUntilFailWithinLimit(t *testing.T) {
	code, out, _ := flaky(t, "--until-fail", "-n", "5", failOn(t, 30))
	if code != 0 {
		t.Errorf("exit code %d, want 0 when nothing failed within -n", code)
	}
	if !strings.Contains(out, "runs       5\n") {
		t.Errorf("want 5 runs:\n%s", out)
	}
}

func TestFailFast(t *testing.T) {
	code, out, _ := flaky(t, "--fail-fast", "-n", "10", failOn(t, 2))
	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if !strings.Contains(out, "runs       2 (stopped at the first failure)\n") {
		t.Errorf("want a stop after run 2:\n%s", out)
	}
}

func TestWarmup(t *testing.T) {
	code, out, _ := flaky(t, "--json", "-n", "5", "--warmup", "2", "--", "true")
	if code != 0 {
		t.Errorf("exit code %d, want 0", code)
	}
	var rep jsonReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if rep.Summary.Runs != 5 || rep.Summary.Warmup != 2 || rep.Summary.TimedRuns != 3 {
		t.Errorf("summary = %+v, want 5 runs, 2 of them warmup, 3 timed", rep.Summary)
	}
	if !rep.Runs[0].Warmup || !rep.Runs[1].Warmup || rep.Runs[2].Warmup {
		t.Errorf("runs = %+v, want the first two marked as warmup", rep.Runs)
	}

	_, out, _ = flaky(t, "-n", "5", "--warmup", "2", "--", "true")
	if !strings.Contains(out, "(timing excludes 2 warmup runs)") {
		t.Errorf("text report doesn't mention the warmup runs:\n%s", out)
	}
}

func TestInterrupted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	var out, errOut bytes.Buffer
	code := run(ctx, []string{"-n", "1000", "--", "sleep", "0.05"}, console{stdout: &out, stderr: &errOut})
	if code != 130 {
		t.Errorf("exit code %d, want 130", code)
	}
	if !strings.Contains(out.String(), " (interrupted)\n") || !strings.Contains(out.String(), "PASS  ") {
		t.Errorf("want a report of the runs that finished:\n%s", out.String())
	}
}

func TestKilledBySignal(t *testing.T) {
	code, out, _ := flaky(t, "--json", "-n", "1", "kill -KILL $$")
	if code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	var rep jsonReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if r := rep.Runs[0]; r.ExitCode != 137 || r.Signal != "SIGKILL" || r.Passed {
		t.Errorf("run = %+v, want exit 137 from SIGKILL", r)
	}

	_, out, _ = flaky(t, "-n", "1", "kill -KILL $$")
	if !strings.Contains(out, "  run 1  killed by SIGKILL  ") {
		t.Errorf("failed run not shown as killed:\n%s", out)
	}
}

func TestQuiet(t *testing.T) {
	_, out, _ := flaky(t, "-q", "-n", "1", "echo noisy; exit 1")
	if strings.Contains(out, "--- output") || strings.Contains(out, "\nnoisy\n") {
		t.Errorf("--quiet still showed command output:\n%s", out)
	}
}

func TestVerbose(t *testing.T) {
	_, out, _ := flaky(t, "-v", "-n", "2", "echo hi")
	if strings.Count(out, "] hi\n") != 2 || !strings.Contains(out, "[1] hi\n") || !strings.Contains(out, "[2] hi\n") {
		t.Errorf("--verbose output not prefixed by run:\n%s", out)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"-n", "0", "true"},
		{"-j", "0", "true"},
		{"--bogus", "true"},
		{"--threshold", "150", "true"},
	} {
		code, _, errOut := flaky(t, args...)
		if code != 2 {
			t.Errorf("flaky %q: exit code %d, want 2", args, code)
		}
		if !strings.Contains(errOut, "flaky: ") {
			t.Errorf("flaky %q: no error message, stderr %q", args, errOut)
		}
	}
}

func TestMissingCommand(t *testing.T) {
	code, _, errOut := flaky(t, "--", "flaky-no-such-command")
	if code != 2 || !strings.Contains(errOut, "flaky-no-such-command") {
		t.Errorf("exit code %d, stderr %q; want 2 and the command named", code, errOut)
	}
}

func TestHelpAndVersion(t *testing.T) {
	if code, out, _ := flaky(t, "--help"); code != 0 || !strings.HasPrefix(out, "Usage: flaky") {
		t.Errorf("--help: exit code %d, output %q", code, out)
	}
	if code, out, _ := flaky(t, "--version"); code != 0 || !strings.HasPrefix(out, "flaky ") {
		t.Errorf("--version: exit code %d, output %q", code, out)
	}
}
