// Command flaky runs a command several times and reports how often it
// passes and how much its run time varies.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"strconv"
	"syscall"

	"golang.org/x/term"

	"github.com/dan2756/flaky/internal/runner"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage: flaky [flags] [--] command [args...]
       flaky [flags] 'shell command'

Run a command several times and report how often it passes and how long
it takes. A single argument containing spaces or shell syntax is run with
sh -c.

Flags:
  -n, --runs N       number of runs (default 10)
  -j, --parallel N   run up to N at once (default 1)
  -t, --timeout D    kill a run after D (e.g. 30s, 2m) and count it as a
                     failure; the whole process group is killed
      --show-output N
                     show the last N lines of the first failed run's
                     output (default 50, 0 for all of it)
  -q, --quiet        show neither command output nor progress
  -v, --verbose      stream every run's output as it happens, each line
                     prefixed with its run number
  -h, --help         show this help
      --version      print the version and exit

Output is captured rather than streamed. On a terminal, a progress line shows
"." for each pass and "F" for each failure.

Exit status is 0 if every run passed, 1 if any run failed, 2 on usage
errors and 130 if interrupted.
`

// console is where flaky writes, and what it knows about the terminal.
type console struct {
	stdout, stderr io.Writer
	tty            bool // stderr is a terminal
	width          int  // its width in columns, 0 if unknown
}

func main() {
	con := console{stdout: os.Stdout, stderr: os.Stderr}
	if fd := int(os.Stderr.Fd()); term.IsTerminal(fd) {
		con.tty = true
		con.width, _, _ = term.GetSize(fd)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], con)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, con console) int {
	stdout, stderr := con.stdout, con.stderr
	opts, err := parseArgs(args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		fmt.Fprint(stdout, usage)
		return 0
	case err != nil:
		fmt.Fprintf(stderr, "flaky: %v\nRun 'flaky --help' for usage.\n", err)
		return 2
	case opts.version:
		fmt.Fprintf(stdout, "flaky %s\n", buildVersion())
		return 0
	}

	if _, err := exec.LookPath(opts.argv[0]); err != nil {
		fmt.Fprintf(stderr, "flaky: %v\n", err)
		return 2
	}

	cfg := runner.Config{
		Argv:     opts.argv,
		Runs:     opts.runs,
		Parallel: opts.parallel,
		Timeout:  opts.timeout,
	}
	var prog *progress
	switch {
	case opts.verbose:
		out := &lockedWriter{w: stdout}
		width := len(strconv.Itoa(opts.runs))
		cfg.Tee = func(index int) io.WriteCloser {
			return newPrefixWriter(out, fmt.Sprintf("[%*d] ", width, index))
		}
	case con.tty && !opts.quiet:
		prog = newProgress(stderr, opts.runs, con.width)
		cfg.OnResult = prog.add
	}

	results, err := runner.Run(ctx, cfg)
	if prog != nil {
		prog.finish()
	}
	if err != nil {
		fmt.Fprintf(stderr, "flaky: %v\n", err)
		return 2
	}
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "flaky: interrupted")
		return 130
	}

	failed := false
	for _, r := range results {
		if !r.Passed() {
			if !failed && !opts.quiet && !opts.verbose {
				writeOutput(stdout, r, opts.showOutput)
			}
			failed = true
		}
	}
	if opts.verbose {
		fmt.Fprintln(stdout)
	}
	writeReport(stdout, opts.display, results)
	if failed {
		return 1
	}
	return 0
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return version
}
