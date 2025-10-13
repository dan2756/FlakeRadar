package main

import (
	"bytes"
	"context"
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
