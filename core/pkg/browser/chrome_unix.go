//go:build !windows

package browser

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// setProcGroup starts the command in its own process group so we can kill
// all child processes (Chrome spawns many) with a single signal.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// tieToEngine is where a started browser would be made to die with this
// process. Nothing portable does that here: Linux has a parent-death signal,
// but it follows the thread that forked, which the Go runtime is free to
// retire, and macOS has no equivalent. A browser outlives an engine that is
// killed outright; one that exits normally closes it.
func tieToEngine(cmd *exec.Cmd) {}

// killProcessTree terminates a launched browser and every process it spawned,
// via process group kill, escalating to SIGKILL if it does not go quietly.
func killProcessTree(cmd *exec.Cmd) {
	pid := cmd.Process.Pid
	// Kill the entire process group (negative PID).
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		<-done
	}
}

// killProfileStragglers kills every process still running with profileDir on
// its command line.
//
// Killing the process group is not always enough for Firefox. When an update
// has been downloaded, Firefox starts the updater, exits, and the updater
// relaunches Firefox with the same arguments — outside the group the engine
// started, so the group kill misses it. That Firefox then holds the BiDi port,
// and every later launch fails with "Maximum number of active sessions". The
// profile directory is a temp dir unique to this launch, which makes it a safe
// way to find such a process. Best effort: without pgrep nothing is killed.
func killProfileStragglers(profileDir string) {
	if profileDir == "" {
		return
	}
	out, err := exec.Command("pgrep", "-f", "--", regexp.QuoteMeta(profileDir)).Output()
	if err != nil {
		return // exit status 1 is "no match"; anything else, nothing to do
	}
	self := os.Getpid()
	for _, field := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(field)
		if err != nil || pid == self || pid <= 1 {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
