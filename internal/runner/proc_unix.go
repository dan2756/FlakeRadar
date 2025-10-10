//go:build unix

package runner

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
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
