package main

import (
	"errors"
	"os/exec"
	"time"
)

type result struct {
	index    int // 1-based run number
	exitCode int
	duration time.Duration
}

func (r result) passed() bool { return r.exitCode == 0 }

// runOnce runs argv to completion with its output discarded. A non-zero
// exit is reported in the result; the error is only for commands that
// could not be started at all.
func runOnce(argv []string, index int) (result, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	start := time.Now()
	err := cmd.Run()
	r := result{index: index, duration: time.Since(start)}

	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		r.exitCode = exitErr.ExitCode()
	default:
		return r, err
	}
	return r, nil
}
