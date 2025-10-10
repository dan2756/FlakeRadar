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
	"syscall"

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
  -h, --help         show this help
      --version      print the version and exit

Exit status is 0 if every run passed, 1 if any run failed, 2 on usage
errors and 130 if interrupted.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
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

	results, err := runner.Run(ctx, runner.Config{
		Argv:     opts.argv,
		Runs:     opts.runs,
		Parallel: opts.parallel,
		Timeout:  opts.timeout,
	})
	if err != nil {
		fmt.Fprintf(stderr, "flaky: %v\n", err)
		return 2
	}
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "flaky: interrupted")
		return 130
	}

	writeReport(stdout, opts.display, results)
	for _, r := range results {
		if !r.Passed() {
			return 1
		}
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
