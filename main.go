// Command flaky runs a command several times and reports how often it
// passes and how much its run time varies.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage: flaky [flags] [--] command [args...]
       flaky [flags] 'shell command'

Run a command several times and report how often it passes and how long
it takes. A single argument containing spaces or shell syntax is run with
sh -c.

Flags:
  -n, --runs N   number of runs (default 10)
  -h, --help     show this help
      --version  print the version and exit

Exit status is 0 if every run passed, 1 if any run failed and 2 on usage
errors.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
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

	results := make([]result, 0, opts.runs)
	for i := 1; i <= opts.runs; i++ {
		r, err := runOnce(opts.argv, i)
		if err != nil {
			fmt.Fprintf(stderr, "flaky: run %d: %v\n", i, err)
			return 2
		}
		results = append(results, r)
	}

	writeReport(stdout, opts.display, results)
	for _, r := range results {
		if !r.passed() {
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
