package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strings"
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

func TestRunKilledBySignal(t *testing.T) {
	results, err := Run(context.Background(), Config{Argv: sh("kill -TERM $$"), Runs: 1})
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if r.Passed() || r.ExitCode != 128+15 || r.Signal != "SIGTERM" || r.TimedOut {
		t.Errorf("result = %+v, want exit 143 from SIGTERM", r)
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
	if got := string(results[0].Output); got != "out\nerr\n" {
		t.Errorf("run 1 output = %q, want stdout and stderr together", got)
	}
	if results[1].Output != nil {
		t.Errorf("run 2 output = %q, want only the first failure's output kept", results[1].Output)
	}
}

func TestRunKeepsFirstFailureOutput(t *testing.T) {
	// Runs print their number, and the even ones fail.
	f := t.TempDir() + "/count"
	script := fmt.Sprintf(`c=$(cat '%[1]s' 2>/dev/null || echo 0); c=$((c+1)); echo $c > '%[1]s'; `+
		`echo run $c; [ $((c %% 2)) -ne 0 ]`, f)
	results, err := Run(context.Background(), Config{Argv: sh(script), Runs: 6})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		want := ""
		if r.Index == 2 {
			want = "run 2\n"
		}
		if string(r.Output) != want {
			t.Errorf("run %d output = %q, want %q", r.Index, r.Output, want)
		}
	}
}

func TestRunCapsOutput(t *testing.T) {
	// About 1.9 MiB of output, most of which has to go.
	results, err := Run(context.Background(), Config{
		Argv: sh("yes 'some log line' | head -n 125000; echo the end; exit 1"),
		Runs: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if len(r.Output) > maxOutput {
		t.Errorf("kept %d bytes of output, want at most %d", len(r.Output), maxOutput)
	}
	if !strings.HasPrefix(string(r.Output), "some log line\n") || !strings.HasSuffix(string(r.Output), "\nthe end\n") {
		t.Errorf("kept output doesn't run from a line start to the end: %q ... %q",
			r.Output[:min(len(r.Output), 20)], r.Output[max(0, len(r.Output)-20):])
	}
	if total := int64(125000*len("some log line\n") + len("the end\n")); r.OutputDropped+int64(len(r.Output)) != total {
		t.Errorf("dropped %d + kept %d bytes, want %d in all", r.OutputDropped, len(r.Output), total)
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

// failOn returns a script that fails on run n (counting runs in a file
// under dir) and passes otherwise.
func failOn(dir string, n int) []string {
	return sh(fmt.Sprintf(`c=$(cat '%[1]s/count' 2>/dev/null || echo 0); c=$((c+1)); echo $c > '%[1]s/count'; [ $c -ne %[2]d ]`, dir, n))
}

func TestRunStopOnFailure(t *testing.T) {
	results, err := Run(context.Background(), Config{
		Argv:          failOn(t.TempDir(), 3),
		Runs:          10,
		StopOnFailure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3 (stopping at the failure)", len(results))
	}
	if !results[0].Passed() || !results[1].Passed() || results[2].Passed() {
		t.Errorf("results = %+v, want pass, pass, fail", results)
	}
}

func TestRunStopOnFailureKillsInFlight(t *testing.T) {
	// Whichever run creates the directory first fails at once; the other
	// would sleep for 5s if it weren't killed.
	script := fmt.Sprintf(`mkdir '%s/first' 2>/dev/null && exit 1; sleep 5`, t.TempDir())
	start := time.Now()
	results, err := Run(context.Background(), Config{
		Argv:          sh(script),
		Runs:          2,
		Parallel:      2,
		StopOnFailure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Run took %v; the run in flight was not killed", elapsed)
	}
	if len(results) != 1 || results[0].Passed() {
		t.Errorf("results = %+v, want just the failure", results)
	}
}

func TestRunUnlimited(t *testing.T) {
	results, err := Run(context.Background(), Config{
		Argv:          failOn(t.TempDir(), 25),
		Runs:          0,
		StopOnFailure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 25 || results[24].Passed() {
		t.Errorf("got %d results, want 25 ending in a failure", len(results))
	}
}

func TestRunWarmup(t *testing.T) {
	// Warmup runs go one at a time, before the parallel ones: the first
	// two runs must each see the count they left behind.
	dir := t.TempDir()
	script := fmt.Sprintf(`echo $$ >> '%[1]s/log'; c=$(wc -l < '%[1]s/log'); echo $c`, dir)
	var (
		mu  sync.Mutex
		out = map[int]*closeBuffer{}
	)
	results, err := Run(context.Background(), Config{
		Argv:     sh(script),
		Runs:     5,
		Parallel: 3,
		Warmup:   2,
		Tee: func(index int) io.WriteCloser {
			mu.Lock()
			defer mu.Unlock()
			out[index] = &closeBuffer{}
			return out[index]
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 5 {
		t.Fatalf("got %d results, want 5", len(results))
	}
	for i, r := range results {
		if want := i < 2; r.Warmup != want {
			t.Errorf("run %d: Warmup = %v, want %v", r.Index, r.Warmup, want)
		}
	}
	for i := 1; i <= 2; i++ {
		if got, want := strings.TrimSpace(out[i].String()), fmt.Sprint(i); got != want {
			t.Errorf("warmup run %d saw count %s, want %s", i, got, want)
		}
	}
}

func TestRunBackgroundChildHoldsOutput(t *testing.T) {
	// The backgrounded sleep keeps the output pipe open after sh exits 1.
	// The run lasts as long as sh does, and is a failure, not a timeout.
	results, err := Run(context.Background(), Config{
		Argv:    sh("sleep 3 & echo started; exit 1"),
		Runs:    1,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if r.TimedOut || r.ExitCode != 1 {
		t.Errorf("result = %+v, want exit 1", r)
	}
	if r.Duration >= time.Second {
		t.Errorf("duration %v includes the wait for the background sleep", r.Duration)
	}
	if string(r.Output) != "started\n" {
		t.Errorf("output = %q, want %q", r.Output, "started\n")
	}
}

func TestRunHugeRunLimit(t *testing.T) {
	// Nothing may be sized by the number of runs asked for.
	results, err := Run(context.Background(), Config{
		Argv:          sh("exit 1"),
		Runs:          math.MaxInt,
		StopOnFailure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Passed() {
		t.Errorf("results = %+v, want one failure", results)
	}
}
