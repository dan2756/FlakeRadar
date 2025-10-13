package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
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

func TestRunTimeoutKillsProcessGroup(t *testing.T) {
	// The backgrounded sleep inherits the output pipe. If only sh were
	// killed, the run would last until waitDelay gave up on the pipe.
	start := time.Now()
	results, err := Run(context.Background(), Config{
		Argv:    sh("sleep 30 & sleep 30"),
		Runs:    1,
		Timeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed >= waitDelay {
		t.Errorf("Run took %v; the process group was not killed", elapsed)
	}
	if len(results) != 1 || !results[0].TimedOut {
		t.Errorf("results = %+v, want one timeout", results)
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

func TestRunCapturesOutput(t *testing.T) {
	results, err := Run(context.Background(), Config{
		Argv: sh("echo out; echo err >&2; exit 1"),
		Runs: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if got := string(r.Output); got != "out\nerr\n" {
			t.Errorf("run %d output = %q, want stdout and stderr together", r.Index, got)
		}
	}
}

type closeBuffer struct {
	bytes.Buffer
	closed bool
}

func (b *closeBuffer) Close() error {
	b.closed = true
	return nil
}

func TestRunTeeAndOnResult(t *testing.T) {
	var (
		mu   sync.Mutex
		tees = map[int]*closeBuffer{}
		seen []int
	)
	_, err := Run(context.Background(), Config{
		Argv:     sh("echo hello"),
		Runs:     3,
		Parallel: 2,
		Tee: func(index int) io.WriteCloser {
			mu.Lock()
			defer mu.Unlock()
			b := &closeBuffer{}
			tees[index] = b
			return b
		},
		OnResult: func(r Result) {
			seen = append(seen, r.Index)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 {
		t.Errorf("OnResult saw runs %v, want 3 calls", seen)
	}
	for i := 1; i <= 3; i++ {
		b := tees[i]
		if b == nil {
			t.Errorf("no tee for run %d", i)
			continue
		}
		if b.String() != "hello\n" || !b.closed {
			t.Errorf("tee for run %d got %q (closed %v)", i, b.String(), b.closed)
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
