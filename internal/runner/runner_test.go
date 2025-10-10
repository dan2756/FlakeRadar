package runner

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func sh(script string) []string {
	return []string{"sh", "-c", script}
}

func TestRunPassing(t *testing.T) {
	results, err := Run(context.Background(), Config{Argv: sh("exit 0"), Runs: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for i, r := range results {
		if r.Index != i+1 {
			t.Errorf("results[%d].Index = %d, want %d", i, r.Index, i+1)
		}
		if !r.Passed() || r.ExitCode != 0 || r.TimedOut {
			t.Errorf("run %d: %+v, want a pass", r.Index, r)
		}
		if r.Duration <= 0 {
			t.Errorf("run %d: duration %v, want > 0", r.Index, r.Duration)
		}
	}
}

func TestRunExitCode(t *testing.T) {
	results, err := Run(context.Background(), Config{Argv: sh("exit 3"), Runs: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Passed() || r.ExitCode != 3 {
			t.Errorf("run %d: %+v, want exit 3", r.Index, r)
		}
	}
}

func TestRunMissingCommand(t *testing.T) {
	_, err := Run(context.Background(), Config{Argv: []string{"flaky-no-such-command"}, Runs: 3})
	if err == nil {
		t.Fatal("Run succeeded, want an error for a command that cannot start")
	}
}

func TestRunTimeout(t *testing.T) {
	start := time.Now()
	results, err := Run(context.Background(), Config{
		Argv:    sh("sleep 5"),
		Runs:    2,
		Timeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Run took %v; timed-out runs were not killed promptly", elapsed)
	}
	for _, r := range results {
		if !r.TimedOut || r.Passed() || r.ExitCode != -1 {
			t.Errorf("run %d: %+v, want a timeout", r.Index, r)
		}
	}
}

func TestRunTimeoutNotHit(t *testing.T) {
	results, err := Run(context.Background(), Config{
		Argv:    sh("exit 0"),
		Runs:    2,
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if !r.Passed() {
			t.Errorf("run %d: %+v, want a pass", r.Index, r)
		}
	}
}

func TestRunParallel(t *testing.T) {
	// Each run waits for all four to have started, so this only passes
	// if they really run at the same time.
	dir := t.TempDir()
	script := fmt.Sprintf(`touch '%s'/$$; while [ "$(ls '%s' | wc -l)" -lt 4 ]; do sleep 0.01; done`, dir, dir)
	results, err := Run(context.Background(), Config{
		Argv:     sh(script),
		Runs:     4,
		Parallel: 4,
		Timeout:  10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 {
		t.Fatalf("got %d results, want 4", len(results))
	}
	for i, r := range results {
		if r.Index != i+1 || !r.Passed() {
			t.Errorf("results[%d] = %+v, want run %d passing", i, r, i+1)
		}
	}
}

func TestRunCanceled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	results, err := Run(ctx, Config{Argv: sh("sleep 5"), Runs: 3, Parallel: 3})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Run took %v after cancel", elapsed)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want runs killed by cancel to be dropped", len(results))
	}
}
