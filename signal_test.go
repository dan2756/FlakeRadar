//go:build unix

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestMain lets a test run this binary as flaky itself, to see what the
// whole process does when it gets a signal.
func TestMain(m *testing.M) {
	if os.Getenv("FLAKY_TEST_MAIN") == "1" {
		main()
	}
	os.Exit(m.Run())
}

func startMain(t *testing.T, stdout *os.File, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "FLAKY_TEST_MAIN=1")
	cmd.Stdout = stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

// runPids waits for n runs to have added their pid to file.
func runPids(t *testing.T, file string, n int) []int {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		data, _ := os.ReadFile(file)
		if fields := strings.Fields(string(data)); len(fields) >= n {
			pids := make([]int, len(fields))
			for i, f := range fields {
				pids[i], _ = strconv.Atoi(f)
			}
			return pids
		}
	}
	t.Fatalf("%d runs didn't start", n)
	return nil
}

// checkRunsGone waits for flaky to exit and checks that the runs went with
// it.
func checkRunsGone(t *testing.T, flaky *exec.Cmd, pids []int) {
	t.Helper()
	exited := make(chan struct{})
	go func() {
		flaky.Wait()
		close(exited)
	}()
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		flaky.Process.Kill()
		t.Error("flaky did not exit")
	}
	for _, pid := range pids {
		if syscall.Kill(pid, 0) == nil {
			t.Errorf("run with pid %d outlived flaky", pid)
			syscall.Kill(-pid, syscall.SIGKILL)
		}
	}
}

func TestHangupKillsRuns(t *testing.T) {
	if signal.Ignored(syscall.SIGHUP) {
		t.Skip("SIGHUP is ignored here, as under nohup")
	}
	pids := filepath.Join(t.TempDir(), "pids")
	flaky := startMain(t, nil, "-n", "2", "-j", "2", fmt.Sprintf("echo $$ >> '%s'; exec sleep 20", pids))
	running := runPids(t, pids, 2)
	flaky.Process.Signal(syscall.SIGHUP)
	checkRunsGone(t, flaky, running)
}

func TestClosedOutputKillsRuns(t *testing.T) {
	// As in flaky -v ... | head: whoever read flaky's output is gone.
	// The runs ignore SIGPIPE themselves, so they would carry on.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	pids := filepath.Join(t.TempDir(), "pids")
	script := fmt.Sprintf(`trap '' PIPE; echo $$ >> '%s'; for i in $(seq 200); do echo tick; sleep 0.1; done`, pids)
	flaky := startMain(t, w, "-v", "-n", "2", "-j", "2", script)
	w.Close()
	running := runPids(t, pids, 2)
	r.Close()
	checkRunsGone(t, flaky, running)
}
