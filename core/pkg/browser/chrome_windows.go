//go:build windows

package browser

import (
	"os/exec"
	"time"
)

// setProcGroup is a no-op on Windows (process groups work differently).
func setProcGroup(cmd *exec.Cmd) {
	// Windows does not use Unix process groups.
	// Browser child processes will be terminated via Process.Kill().
}

// killProcessTree terminates a launched browser on Windows.
func killProcessTree(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}
