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
max        1.921s

failed runs
  run 7   exit 1
  run 15  exit 1
```

| Flag | Description |
| --- | --- |
| `-n`, `--runs N` | number of runs (default 10) |
| `-h`, `--help` | show help |
| `--version` | print the version |

Exit status is 0 if every run passed, 1 if any failed and 2 on usage errors.

## License

MIT
