# flaky

Run a command N times and see how often it passes and how long it takes.

`flaky` is for the test that fails "sometimes": run it twenty times, get a pass
rate and the run numbers that failed, and stop guessing.

## Install

```sh
go install github.com/dan2756/flaky@latest
```

## Usage

```sh
flaky -n 20 -- go test -count=1 ./pkg/...
flaky -n 100 -j 8 --timeout 30s -- go test -count=1 -run TestCache ./cache
flaky -n 50 'make test 2>&1 | grep -v noise'
```

Everything after `--` is the command and its arguments. A single quoted argument
that contains spaces or shell syntax is run with `sh -c`.

```
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

failed runs
  run 7   exit 1   1.523s
  run 15  timeout  30.001s
```

The timing block shows the spread of run times: median and p95 use linear
interpolation between samples, stddev is the sample standard deviation and cv
(coefficient of variation, stddev / mean) makes the spread comparable between
fast and slow commands.

| Flag | Description |
| --- | --- |
| `-n`, `--runs N` | number of runs (default 10) |
| `-j`, `--parallel N` | run up to N at once (default 1) |
| `-t`, `--timeout D` | kill a run that takes longer than D (`30s`, `2m`) and count it as a failure |
| `-h`, `--help` | show help |
| `--version` | print the version |

Each run starts in its own process group. On a timeout, or when flaky itself is
interrupted, the whole group is killed, so `go test`'s test binaries and every
stage of a pipeline go with it. Timed-out runs are left out of the timing
numbers, since their duration is just the limit.

Exit status is 0 if every run passed, 1 if any failed, 2 on usage errors and
130 if interrupted.

## License

MIT
