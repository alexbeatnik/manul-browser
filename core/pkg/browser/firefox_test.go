package browser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The BiDi URL is discovered by reading Firefox's startup banner off stderr.
// Get this wrong and the launch either hangs for the full banner timeout or
// hands back an endpoint nothing answers on.
func TestParseBiDiBanner(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			"the banner Firefox actually prints",
			"WebDriver BiDi listening on ws://127.0.0.1:9222/session",
			"ws://127.0.0.1:9222/session",
		},
		{
			"timestamped log line",
			"1731000000000\tRemoteAgent\tINFO\tWebDriver BiDi listening on ws://127.0.0.1:4321/session",
			"ws://127.0.0.1:4321/session",
		},
		{
			"trailing text after the URL",
			`WebDriver BiDi listening on ws://127.0.0.1:9222/session (pid 1234)`,
			"ws://127.0.0.1:9222/session",
		},
		{"unrelated noise", "JavaScript error: resource://gre/modules/x.sys.mjs", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseBiDiBanner(tc.line); got != tc.want {
				t.Fatalf("parseBiDiBanner(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

// Firefox takes automation settings from the profile, not from flags, and it
// reads user.js on every start — which is what makes a reused profile behave
// like a fresh one.
func TestWriteFirefoxPrefs(t *testing.T) {
	dir := t.TempDir()
	if err := writeFirefoxPrefs(dir); err != nil {
		t.Fatalf("write prefs: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "user.js"))
	if err != nil {
		t.Fatalf("read user.js: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		`user_pref("browser.shell.checkDefaultBrowser", false);`,
		`user_pref("signon.rememberSignons", false);`,
		`user_pref("dom.disable_beforeunload", true);`,
		`user_pref("browser.download.dir",`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("user.js missing %s\ngot:\n%s", want, content)
		}
	}

	// Writing into a profile that already exists must not fail: callers pass
	// their own --user-data-dir and expect it to keep working.
	if err := writeFirefoxPrefs(dir); err != nil {
		t.Fatalf("rewrite prefs: %v", err)
	}
}

func TestFindFirefox_UnknownChannelIsTreatedAsABinaryName(t *testing.T) {
	// A channel that maps to nothing installed must fail with a message that
	// says what to do, not with a silent fallback to some other browser.
	_, err := findFirefox("definitely-not-a-real-firefox-channel")
	if err == nil {
		t.Skip("a binary by that name exists on this machine")
	}
	if !strings.Contains(err.Error(), "firefox not found") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

func TestLaunchFirefox_ReportsMissingBinary(t *testing.T) {
	// Nothing is spawned: an explicit path that does not exist must fail at
	// the exec, and must not leave a temp profile behind.
	before := tempProfileCount(t)
	_, err := LaunchFirefox(context.Background(), LaunchOptions{
		Browser:        EngineFirefox,
		ExecutablePath: filepath.Join(t.TempDir(), "no-such-firefox"),
		Port:           9999,
	})
	if err == nil {
		t.Fatal("expected a launch failure")
	}
	if after := tempProfileCount(t); after != before {
		t.Fatalf("a failed launch left a temp profile behind (%d → %d)", before, after)
	}
}

func tempProfileCount(t *testing.T) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "manul-firefox-*"))
	if err != nil {
		t.Fatalf("glob temp profiles: %v", err)
	}
	return len(matches)
}
