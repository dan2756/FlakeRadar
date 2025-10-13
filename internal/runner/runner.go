// Package runner executes a command repeatedly and records how each run
// went.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
	Runs     int           // number of runs
	Parallel int           // maximum number of concurrent runs; below 1 means 1
	Timeout  time.Duration // per-run time limit; 0 means none

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
	Index    int // 1-based run number
	ExitCode int // -1 if the run did not exit on its own
	TimedOut bool
	Duration time.Duration
	Output   []byte // stdout and stderr, interleaved as written
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
		results  = make([]Result, 0, cfg.Runs)
		firstErr error
		wg       sync.WaitGroup
		jobs     = make(chan int)
	)
	for range min(max(cfg.Parallel, 1), cfg.Runs) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				r, err := runOnce(ctx, cfg, i)
				mu.Lock()
				switch {
				case err == nil:
					results = append(results, r)
					if cfg.OnResult != nil {
						cfg.OnResult(r)
					}
				case !errors.Is(err, errCanceled) && firstErr == nil:
					firstErr = err
					cancel()
				}
				mu.Unlock()
			}
		}()
	}

feed:
	for i := 1; i <= cfg.Runs; i++ {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

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

	var out bytes.Buffer
	var w io.Writer = &out
	if cfg.Tee != nil {
		tee := cfg.Tee(index)
		defer tee.Close()
		w = io.MultiWriter(&out, tee)
	}

	cmd := exec.CommandContext(runCtx, cfg.Argv[0], cfg.Argv[1:]...)
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.WaitDelay = waitDelay
	killWholeGroup(cmd)

	start := time.Now()
	err := cmd.Run()
	r := Result{Index: index, Duration: time.Since(start), Output: out.Bytes()}
	if errors.Is(err, exec.ErrWaitDelay) {
		// The command exited 0 but left something running that still
		// holds its output open. The exit status is what counts.
		err = nil
	}
	if err == nil {
		return r, nil
	}
	if ctx.Err() != nil {
		// Killed because Run is stopping, not because of anything the
		// command did.
		return r, errCanceled
	}

	var exitErr *exec.ExitError
	switch {
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		r.TimedOut = true
		r.ExitCode = -1
	case errors.As(err, &exitErr):
		r.ExitCode = exitErr.ExitCode()
	default:
		return r, fmt.Errorf("run %d: %w", index, err)
	}
	return r, nil
}
