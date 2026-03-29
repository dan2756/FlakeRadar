// Package runner executes a command repeatedly and records how each run
// went.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"sync"
	"time"
)

// waitDelay is how long to keep reading a run's output after it exits (or
// is killed) when something it started still holds the output open.
const waitDelay = 2 * time.Second

// Config describes what to run and how.
type Config struct {
	Argv     []string      // command and arguments
	Runs     int           // number of runs; 0 means no limit
	Parallel int           // maximum number of concurrent runs; below 1 means 1
	Timeout  time.Duration // per-run time limit; 0 means none

	// Warmup runs come first, one at a time, before any others start.
	// Their results are marked as such so they can be left out of timing.
	Warmup int

	// StopOnFailure stops at the first failed run: nothing new starts,
	// and runs still in flight are killed and left out of the results.
	StopOnFailure bool

	// Tee, if set, is called as each run starts; the run's output is
	// copied to the returned writer as well as captured, and the writer is
	// closed when the run ends.
	Tee func(index int) io.WriteCloser

	// OnResult, if set, is called with each result as its run finishes.
	// Calls are never concurrent.
	OnResult func(Result)
}

// Result is the outcome of one run.
type Result struct {
	Index    int    // 1-based run number
	ExitCode int    // 128+N if killed by signal N, -1 if it timed out
	Signal   string // name of the signal that killed the run, e.g. "SIGSEGV"
	TimedOut bool
	Warmup   bool
	Duration time.Duration

	// Output is stdout and stderr, interleaved as written. To keep memory
	// bounded over thousands of runs, Run keeps it only for the
	// lowest-numbered failed run, and only its last 1 MiB; it is nil for
	// every other result.
	Output []byte
	// OutputDropped counts the bytes cut from the start of Output.
	OutputDropped int64
}

// Passed reports whether the run exited 0 within its time limit.
func (r Result) Passed() bool { return r.ExitCode == 0 && !r.TimedOut }

// errCanceled marks a run that was cut short because Run is stopping.
var errCanceled = errors.New("run canceled")

// Run runs cfg.Argv cfg.Runs times, up to cfg.Parallel at once, and returns
// the results ordered by run number.
//
// If ctx is canceled, runs in flight are killed and left out of the
// results, no new runs start, and Run returns the runs that had completed.
// The error is non-nil only if the command could not be run at all.
func Run(ctx context.Context, cfg Config) ([]Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu       sync.Mutex
		done     = newCollector()
		firstErr error
	)
	record := func(r Result, err error) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case err == nil:
			r = done.add(r)
			if cfg.OnResult != nil {
				cfg.OnResult(r)
			}
			if cfg.StopOnFailure && !r.Passed() {
				cancel()
			}
		case !errors.Is(err, errCanceled) && firstErr == nil:
			firstErr = err
			cancel()
		}
	}
	more := func(i int) bool { return cfg.Runs == 0 || i <= cfg.Runs }

	next := 1
	for ; next <= cfg.Warmup && more(next) && ctx.Err() == nil; next++ {
		record(runOnce(ctx, cfg, next))
	}

	workers := max(cfg.Parallel, 1)
	if cfg.Runs > 0 {
		workers = min(workers, cfg.Runs-next+1)
	}
	var wg sync.WaitGroup
	jobs := make(chan int)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				record(runOnce(ctx, cfg, i))
			}
		}()
	}

feed:
	for i := next; more(i) && ctx.Err() == nil; i++ {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

	results := done.results
	slices.SortFunc(results, func(a, b Result) int { return a.Index - b.Index })
	return results, firstErr
}

func runOnce(ctx context.Context, cfg Config, index int) (Result, error) {
	if ctx.Err() != nil {
		return Result{}, errCanceled
	}
	runCtx := ctx
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	out := newTailBuffer(maxOutput)
	var w io.Writer = out
	if cfg.Tee != nil {
		tee := cfg.Tee(index)
		defer tee.Close()
		w = io.MultiWriter(out, tee)
	}

	// The command writes to a pipe of our own rather than to w, so that
	// Wait returns as soon as it exits. Something it left running in the
	// background may hold the pipe open for longer, and that is neither
	// part of the run's duration nor a reason to call it a timeout.
	pr, pw, err := os.Pipe()
	if err != nil {
		return Result{}, fmt.Errorf("run %d: %w", index, err)
	}
	cmd := exec.CommandContext(runCtx, cfg.Argv[0], cfg.Argv[1:]...)
	cmd.Stdout = pw
	cmd.Stderr = pw
	killWholeGroup(cmd)

	start := time.Now()
	err = cmd.Start()
	pw.Close() // the command has its own copy
	waitOutput := copyOutput(w, pr)
	if err == nil {
		err = cmd.Wait()
	}
	elapsed := time.Since(start)
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	stopping := ctx.Err() != nil
	waitOutput()

	output := out.Bytes() // may count a partial first line as dropped
	r := Result{
		Index:         index,
		Warmup:        index <= cfg.Warmup,
		Duration:      elapsed,
		Output:        output,
		OutputDropped: out.dropped,
	}
	if err == nil {
		return r, nil
	}
	if stopping {
		// Killed because Run is stopping, not because of anything the
		// command did.
		return r, errCanceled
	}

	var exitErr *exec.ExitError
	switch {
	case timedOut:
		r.TimedOut = true
		r.ExitCode = -1
	case errors.As(err, &exitErr):
		r.ExitCode, r.Signal = exitStatus(exitErr.ProcessState)
	default:
		return r, fmt.Errorf("run %d: %w", index, err)
	}
	return r, nil
}

// copyOutput copies what the command writes to r on to w, in the
// background. Once the command has exited, wait waits for the copy to
// reach the end of the output, but no longer than waitDelay, and closes r.
func copyOutput(w io.Writer, r *os.File) (wait func()) {
	done := make(chan struct{})
	go func() {
		io.Copy(w, r)
		close(done)
	}()
	return func() {
		defer r.Close()
		select {
		case <-done:
		case <-time.After(waitDelay):
			r.Close() // ends the copy
			<-done
		}
	}
}
