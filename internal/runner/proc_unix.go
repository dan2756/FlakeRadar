//go:build unix

package runner

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// killWholeGroup starts cmd in a process group of its own and makes
// canceling its context kill that whole group, so a timeout also takes
// down whatever the command started: the test binaries under go test,
// every stage of a shell pipeline.
func killWholeGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}

// exitStatus describes how a process ended. For one killed by a signal it
// follows the shell convention of 128 plus the signal number, and also
// returns the signal's name.
func exitStatus(state *os.ProcessState) (code int, signal string) {
	ws, ok := state.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return state.ExitCode(), ""
	}
	sig := ws.Signal()
	name := unix.SignalName(sig)
	if name == "" {
		name = sig.String()
	}
	return 128 + int(sig), name
}
