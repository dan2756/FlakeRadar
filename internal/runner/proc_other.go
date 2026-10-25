//go:build !unix

package runner

import (
	"os"
	"os/exec"
)

// killWholeGroup is a no-op without Unix process groups: canceling the
// context kills only the command's own process.
func killWholeGroup(cmd *exec.Cmd) {}

// finishedVoluntarily is false: without signals a kill and an ordinary
// exit look the same, so a stop still drops any run that did not succeed.
func finishedVoluntarily(state *os.ProcessState) bool { return false }

// exitStatus returns the process's exit code; there are no signals to
// report.
func exitStatus(state *os.ProcessState) (code int, signal string) {
	return state.ExitCode(), ""
}
