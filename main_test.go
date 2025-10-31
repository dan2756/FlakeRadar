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
