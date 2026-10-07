//go:build !windows

package browser

import (
	"os/exec"
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
