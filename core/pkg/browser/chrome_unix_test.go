//go:build !windows

package browser

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// A Firefox relaunched by its updater runs outside the process group the
// engine started. It still carries the launch's own profile directory, which is
// what killProfileStragglers finds it by.
func TestKillProfileStragglersKillsProcessOutsideTheGroup(t *testing.T) {
	if _, err := exec.LookPath("pgrep"); err != nil {
		t.Skip("pgrep not available")
	}
	profile := t.TempDir()
	// "; :" keeps sh from exec-ing sleep, so the profile stays on a command line.
	cmd := exec.Command("sh", "-c", "sleep 30; :", profile)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // not our group
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	other := t.TempDir()
	killProfileStragglers(other) // someone else's profile: must not touch ours
	select {
	case <-done:
		t.Fatal("a process with a different profile was killed")
	case <-time.After(200 * time.Millisecond):
	}

	killProfileStragglers(profile)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("process carrying the profile dir survived")
	}
}
