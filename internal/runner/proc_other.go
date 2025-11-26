//go:build !unix

package runner

import (
	"os"
	"os/exec"
)

// killWholeGroup is a no-op without Unix process groups: canceling the
// context kills only the command's own process.
func killWholeGroup(cmd *exec.Cmd) {}

// exitStatus returns the process's exit code; there are no signals to
// report.
func exitStatus(state *os.ProcessState) (code int, signal string) {
	return state.ExitCode(), ""
}
