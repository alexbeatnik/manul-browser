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
