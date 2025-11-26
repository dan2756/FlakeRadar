# flaky

Run a command N times and see how often it passes and how long it takes.

`flaky` is for the test that fails "sometimes": run it twenty times, get a pass
rate, the run numbers that failed and the output of the first one, and stop
guessing.

## Install

```sh
go install github.com/dan2756/flaky@latest
```

## Usage

```sh
flaky [flags] [--] command [args...]
flaky [flags] 'shell command'
```

Everything after `--` is the command and its arguments. A single quoted argument
that contains spaces or shell syntax is run with `sh -c`.

```sh
# Is it flaky?
flaky -n 20 -- go test -count=1 ./pkg/...

# Hammer one test, eight at a time, killing any run that hangs.
flaky -n 200 -j 8 --timeout 30s -- go test -count=1 -run 'TestCache$' ./cache

# Keep going until it breaks, then show why.
flaky --until-fail -j 4 -- go test -count=1 -race -run TestScheduler ./sched

# Anything a shell can run.
flaky -n 50 'make integration 2>&1 | grep -v DEBUG'

# Gate CI on "passes at least 95% of the time".
flaky -n 40 --threshold 95 --quiet -- ./scripts/e2e.sh
```

Pass `-count=1` to `go test`, or it will happily serve the first result from
its cache for every run after that.

```
$ flaky -n 20 --timeout 30s -- go test -count=1 ./pkg/...
....F.........F.....  20/20
--- output of run 7 (exit 1), last 4 of 212 lines ---
--- FAIL: TestCacheEviction (0.02s)
    cache_test.go:41: got 3 entries, want 2
FAIL
FAIL	example.com/app/pkg/cache	0.031s

command    go test -count=1 ./pkg/...
runs       20
passes     18
fails      2
pass rate  90.0%

min        1.214s
mean       1.432s
median     1.398s
p95        1.877s
max        1.921s
stddev     0.181s
cv          12.6%
           (timing excludes 1 timed-out run)

failed runs
  run 7   exit 1   1.523s
  run 15  timeout  30.001s

FAIL  18/20 runs passed (threshold 100%)
```

The timing block shows the spread of run times: median and p95 interpolate
between samples, stddev is the sample standard deviation, and cv (coefficient
of variation, stddev / mean) makes the spread comparable between fast and slow
commands.

## Flags

| Flag | Description |
| --- | --- |
| `-n`, `--runs N` | number of runs (default 10) |
| `-j`, `--parallel N` | run up to N at once (default 1) |
| `-t`, `--timeout D` | kill a run that takes longer than D (`30s`, `2m`) and count it as a failure |
| `--fail-fast` | stop at the first failure |
| `--until-fail` | keep going until a run fails: up to `-n` runs if given, no limit otherwise |
| `--warmup K` | run K runs first, one at a time, and leave them out of the timing stats |
| `--threshold P` | minimum pass rate in percent for exit status 0 (default 100) |
| `--json` | print every run and the summary as JSON |
| `--show-output N` | show the last N lines of the first failed run's output (default 50, `0` for all) |
| `-q`, `--quiet` | show neither command output nor progress |
| `-v`, `--verbose` | stream every run's output live, each line prefixed with `[run]` |
| `-h`, `--help` | show help |
| `--version` | print the version |

## Output

Command output (stdout and stderr together) is captured, not streamed. When
everything is done, flaky prints the output of the first failed run, then the
report. `--verbose` streams all of it instead, line by line with the run number
in front, which stays readable with `-j`.

While runs are going, a progress line on stderr shows `.` for each pass and `F`
for each failure, with a counter. It is only drawn when stderr is a terminal,
so logs and pipes get plain output.

## Hunting a flake

`--until-fail` is for the test that fails once in a few hundred runs. Without
`-n` it runs until something fails (or you press Ctrl-C), then shows that
run's output:

```
$ flaky --until-fail -j 4 -- go test -count=1 -race -run TestScheduler ./sched
............................................................F  61
--- output of run 63 (exit 1) ---
--- FAIL: TestScheduler (0.31s)
    sched_test.go:88: job 12 ran twice
FAIL
...

runs       61 (stopped at the first failure)
...
```

`--fail-fast` stops the same way but keeps the default of 10 runs, for when
you only want to know whether it passes. Either way, runs still in flight when
the failure comes in are killed and left out of the report, which is why the
run numbers can skip.

Ctrl-C stops everything, kills the runs in flight and reports on the ones that
finished (exit status 130). A second Ctrl-C exits at once.

`--warmup K` treats the first K runs as warmup: they run one at a time before
the rest, still count as passes or failures, but are left out of the timing,
so a cold build cache or first-run compile doesn't skew it.

## Timeouts

Each run starts in its own process group. On a timeout, or when flaky itself is
interrupted, the whole group is killed, so `go test`'s test binaries and every
stage of a pipeline go with it. Timed-out runs count as failures but are left
out of the timing numbers, since their duration is just the limit.

## JSON

`--json` replaces the text report with a single JSON document on stdout. The
command's output is not included; with `--verbose` it streams to stderr.
Trimmed to two of the twenty runs:

```json
{
  "command": "go test -count=1 ./pkg/...",
  "argv": ["go", "test", "-count=1", "./pkg/..."],
  "runs": [
    { "index": 1, "exit_code": 0, "duration_ms": 1398.214, "timed_out": false, "passed": true },
    { "index": 7, "exit_code": 1, "duration_ms": 1523.007, "timed_out": false, "passed": false }
  ],
  "summary": {
    "runs": 20,
    "passes": 18,
    "fails": 2,
    "pass_rate": 90,
    "threshold": 100,
    "ok": false,
    "timed_runs": 19,
    "duration_ms": { "min": 1214.33, "mean": 1432.1, "median": 1398.214, "p95": 1877.5, "max": 1921.02, "stddev": 181.2 },
    "cv": 0.1265,
    "failed_runs": [7, 15]
  }
}
```

A timed-out run has `"timed_out": true` and `"exit_code": -1`. A run killed by
a signal gets the exit code a shell would report, 128 plus the signal number,
and a `"signal"` field such as `"SIGSEGV"`; the text report lists it as
`killed by SIGSEGV`. `timed_runs` is how many runs the duration stats cover,
leaving out warmup and timed-out runs.

## Exit status

| Code | Meaning |
| --- | --- |
| 0 | the pass rate is at least `--threshold` |
| 1 | it is not |
| 2 | usage error, or the command could not be started |
| 130 | interrupted |

## License

MIT
