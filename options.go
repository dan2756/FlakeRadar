package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type options struct {
	runs       int
	parallel   int
	timeout    time.Duration
	showOutput int
	quiet      bool
	verbose    bool
	version    bool

	argv    []string // what to execute
	display string   // how the command is shown in the report
}

func parseArgs(args []string) (*options, error) {
	o := &options{}
	fs := flag.NewFlagSet("flaky", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	fs.IntVar(&o.runs, "n", 10, "")
	fs.IntVar(&o.runs, "runs", 10, "")
	fs.IntVar(&o.parallel, "j", 1, "")
	fs.IntVar(&o.parallel, "parallel", 1, "")
	fs.DurationVar(&o.timeout, "t", 0, "")
	fs.DurationVar(&o.timeout, "timeout", 0, "")
	fs.IntVar(&o.showOutput, "show-output", 50, "")
	fs.BoolVar(&o.quiet, "q", false, "")
	fs.BoolVar(&o.quiet, "quiet", false, "")
	fs.BoolVar(&o.verbose, "v", false, "")
	fs.BoolVar(&o.verbose, "verbose", false, "")
	fs.BoolVar(&o.version, "version", false, "")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if o.version {
		return o, nil
	}
	if fs.NArg() == 0 {
		return nil, errors.New("missing command")
	}
	if o.runs < 1 {
		return nil, fmt.Errorf("-n must be at least 1, got %d", o.runs)
	}
	if o.parallel < 1 {
		return nil, fmt.Errorf("-j must be at least 1, got %d", o.parallel)
	}
	if o.timeout < 0 {
		return nil, fmt.Errorf("--timeout must not be negative, got %v", o.timeout)
	}
	if o.showOutput < 0 {
		return nil, fmt.Errorf("--show-output must not be negative, got %d", o.showOutput)
	}
	if o.quiet && o.verbose {
		return nil, errors.New("--quiet and --verbose don't go together")
	}
	o.argv, o.display = commandArgv(fs.Args())
	return o, nil
}

// shellChars only mean something to a shell. A lone command argument that
// contains any of them is handed to sh -c rather than executed directly.
const shellChars = " \t\n|&;<>()$`\\\"'*?[#~"

// commandArgv turns the positional arguments into the argv to execute and
// the string to show for it.
func commandArgv(args []string) (argv []string, display string) {
	if len(args) == 1 && strings.ContainsAny(args[0], shellChars) {
		return []string{"sh", "-c", args[0]}, args[0]
	}
	return args, shellJoin(args)
}

// shellJoin joins args into a string that a shell would split back into
// the same words.
func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, shellChars) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
