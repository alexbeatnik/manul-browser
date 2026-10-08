package browser

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestParseDevToolsActivePort(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int
		ok   bool
	}{
		{"port and browser path", "41873\n/devtools/browser/0b1c-4d", 41873, true},
		{"crlf line ending", "41873\r\n/devtools/browser/0b1c-4d", 41873, true},
		{"port line only", "41873\n", 41873, true},
		{"empty", "", 0, false},
		// Chrome caught mid-write: these digits may be the start of a longer port.
		{"unterminated port", "4187", 0, false},
		{"garbage", "not a port\n/devtools/browser/0b1c-4d", 0, false},
		{"zero", "0\n/devtools/browser/0b1c-4d", 0, false},
		{"out of range", "70000\n/devtools/browser/0b1c-4d", 0, false},
		{"negative", "-1\n", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseDevToolsActivePort([]byte(c.data))
			if got != c.want || ok != c.ok {
				t.Fatalf("parseDevToolsActivePort(%q) = %d, %v; want %d, %v", c.data, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestWaitForDevToolsPort_ReadsExistingFile(t *testing.T) {
	dir := t.TempDir()
	writeDevToolsPort(t, dir, "41873\n/devtools/browser/0b1c-4d")

	port, err := waitForDevToolsPort(context.Background(), dir)
	if err != nil {
		t.Fatalf("waitForDevToolsPort: %v", err)
	}
	if port != 41873 {
		t.Fatalf("port = %d, want 41873", port)
	}
}

// The file does not exist when Chrome is started; it appears once the port is
// listening, and may be seen half written on the way.
func TestWaitForDevToolsPort_WaitsForFileToAppear(t *testing.T) {
	dir := t.TempDir()
	go func() {
		time.Sleep(120 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, devToolsActivePortFile), []byte("4187"), 0o644)
		time.Sleep(120 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, devToolsActivePortFile), []byte("41873\n/devtools/browser/0b1c-4d"), 0o644)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	port, err := waitForDevToolsPort(ctx, dir)
	if err != nil {
		t.Fatalf("waitForDevToolsPort: %v", err)
	}
	if port != 41873 {
		t.Fatalf("port = %d, want 41873", port)
	}
}

func TestWaitForDevToolsPort_TimesOut(t *testing.T) {
	for _, content := range []string{"", "garbage\n"} {
		dir := t.TempDir()
		if content != "" {
			writeDevToolsPort(t, dir, content)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		port, err := waitForDevToolsPort(ctx, dir)
		cancel()
		if err == nil {
			t.Fatalf("content %q: got port %d, want a timeout", content, port)
		}
	}
}

func TestRemoveStaleDevToolsPort(t *testing.T) {
	dir := t.TempDir()
	if err := removeStaleDevToolsPort(dir); err != nil {
		t.Fatalf("a profile with no port file must not be an error: %v", err)
	}

	writeDevToolsPort(t, dir, "9222\n/devtools/browser/old")
	if err := removeStaleDevToolsPort(dir); err != nil {
		t.Fatalf("removeStaleDevToolsPort: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, devToolsActivePortFile)); !os.IsNotExist(err) {
		t.Fatalf("stale port file survived: %v", err)
	}
}

// A reused profile holds the port of its previous run. With Port 0 it must be
// gone before Chrome is started, or it would be read back as the new port.
// Nothing is spawned: the binary does not exist, so the launch fails at exec.
func TestLaunchChrome_RemovesStalePortFileBeforeStart(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-chrome")

	dir := t.TempDir()
	writeDevToolsPort(t, dir, "9222\n/devtools/browser/old")
	if _, err := LaunchChrome(context.Background(), LaunchOptions{ExecutablePath: missing, UserDataDir: dir}); err == nil {
		t.Fatal("expected a launch failure")
	}
	if _, err := os.Stat(filepath.Join(dir, devToolsActivePortFile)); !os.IsNotExist(err) {
		t.Fatalf("stale port file survived a Port 0 launch: %v", err)
	}

	// An explicit port never reads the file, so it is left alone.
	writeDevToolsPort(t, dir, "9222\n/devtools/browser/old")
	if _, err := LaunchChrome(context.Background(), LaunchOptions{ExecutablePath: missing, UserDataDir: dir, Port: 9777}); err == nil {
		t.Fatal("expected a launch failure")
	}
	if _, err := os.Stat(filepath.Join(dir, devToolsActivePortFile)); err != nil {
		t.Fatalf("an explicit-port launch touched the port file: %v", err)
	}
}

func TestChromeArgs_DebuggingPort(t *testing.T) {
	opts := DefaultLaunchOptions()
	opts.UserDataDir = "/tmp/profile"

	args := chromeArgs(opts)
	if !slices.Contains(args, "--remote-debugging-port=0") {
		t.Fatalf("default launch must ask Chrome for a free port; args: %v", args)
	}
	if !slices.Contains(args, "--user-data-dir=/tmp/profile") {
		t.Fatalf("missing --user-data-dir; args: %v", args)
	}

	opts.Port = 9777
	args = chromeArgs(opts)
	if !slices.Contains(args, "--remote-debugging-port=9777") {
		t.Fatalf("an explicit port must be passed through; args: %v", args)
	}
	if slices.Contains(args, "--remote-debugging-port=0") {
		t.Fatalf("explicit port launch still asks for port 0; args: %v", args)
	}
}

func writeDevToolsPort(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, devToolsActivePortFile), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", devToolsActivePortFile, err)
	}
}
