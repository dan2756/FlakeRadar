//go:build !unix

package runner

import "os/exec"

// killWholeGroup is a no-op without Unix process groups: canceling the
// context kills only the command's own process.
func killWholeGroup(cmd *exec.Cmd) {}
