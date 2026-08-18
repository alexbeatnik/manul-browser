// Package browser — Firefox process lifecycle management.
//
// Firefox is driven over WebDriver BiDi, not CDP. Its CDP support was
// experimental, deprecated in Firefox 129 behind `remote.active-protocols`, and
// removed outright in Firefox 141 along with that preference — so a current
// Firefox has no CDP endpoint to offer and nothing here tries to ask for one.
// `--remote-debugging-port` still exists; it now starts the BiDi agent.
package browser

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/bidi"
)

// FirefoxProcess manages a Firefox browser process spawned for automation.
type FirefoxProcess struct {
	cmd         *exec.Cmd
	port        int
	wsURL       string
	profileDir  string
	ownsProfile bool // true when we created the dir and should clean it up
}

// firefoxChannelBinaries maps a channel name to the concrete binaries to probe
// (in order), mirroring channelBinaries for Chromium.
var firefoxChannelBinaries = map[string][]string{
	"firefox":         {"firefox"},
	"firefox-esr":     {"firefox-esr", "firefox"},
	"firefox-dev":     {"firefox-developer-edition", "firefox"},
	"firefox-beta":    {"firefox-beta", "firefox"},
	"firefox-nightly": {"firefox-nightly", "firefox-trunk"},
}

// bidiBannerTimeout bounds the wait for Firefox to announce its BiDi endpoint
// on stderr. A cold profile on a slow machine takes several seconds.
const bidiBannerTimeout = 30 * time.Second

// LaunchFirefox starts a Firefox process with the WebDriver BiDi agent
// enabled. It blocks until Firefox has announced its BiDi endpoint (or the
// context expires). If opts.UserDataDir is empty, a unique temp profile is
// created and owned by the returned FirefoxProcess.
func LaunchFirefox(ctx context.Context, opts LaunchOptions) (*FirefoxProcess, error) {
	firefoxPath := opts.ExecutablePath
	if firefoxPath == "" {
		var err error
		firefoxPath, err = findFirefox(opts.Channel)
		if err != nil {
			return nil, err
		}
	}
	if opts.Port == 0 {
		opts.Port = DefaultLaunchOptions().Port
	}

	ownsDir := false
	if opts.UserDataDir == "" {
		dir, err := os.MkdirTemp("", "manul-firefox-*")
		if err != nil {
			return nil, fmt.Errorf("create firefox temp dir: %w", err)
		}
		opts.UserDataDir = dir
		ownsDir = true
	}

	// Firefox takes automation preferences from the profile, not the command
	// line — there is no flag equivalent of most of these.
	if err := writeFirefoxPrefs(opts.UserDataDir); err != nil {
		if ownsDir {
			_ = os.RemoveAll(opts.UserDataDir)
		}
		return nil, fmt.Errorf("write firefox prefs: %w", err)
	}

	args := []string{
		"--remote-debugging-port", fmt.Sprintf("%d", opts.Port),
		// Never hand the command to an already-running Firefox: without this,
		// starting a second one just opens a tab in the user's session and the
		// process we are holding exits immediately.
		"--no-remote",
		"--profile", opts.UserDataDir,
	}
	if opts.Headless {
		args = append(args, "--headless")
	}

	// Firefox must outlive ctx for the same reason Chrome does — see the note
	// in LaunchChrome. ctx bounds startup only.
	cmd := exec.Command(firefoxPath, args...)
	cmd.Stdout = nil
	// stderr carries the "WebDriver BiDi listening on ws://…" banner, which is
	// how the endpoint is discovered.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		if ownsDir {
			_ = os.RemoveAll(opts.UserDataDir)
		}
		return nil, fmt.Errorf("firefox stderr pipe: %w", err)
	}
	cmd.Env = os.Environ()
	setProcGroup(cmd)

	if err := cmd.Start(); err != nil {
		if ownsDir {
			_ = os.RemoveAll(opts.UserDataDir)
		}
		return nil, fmt.Errorf("start firefox: %w", err)
	}

	fp := &FirefoxProcess{
		cmd:         cmd,
		port:        opts.Port,
		profileDir:  opts.UserDataDir,
		ownsProfile: ownsDir,
	}

	// The banner reader also drains stderr for the life of the process: a
	// browser whose stderr pipe fills up blocks on its next write.
	banner := make(chan string, 1)
	go scanBiDiBanner(stderr, banner)

	fallback := fmt.Sprintf("ws://127.0.0.1:%d%s", opts.Port, bidi.SessionPath)
	select {
	case ws := <-banner:
		// The banner names the origin only; the upgrade happens at /session.
		fp.wsURL = bidi.NormalizeWebSocketURL(ws)
	case <-ctx.Done():
		_ = fp.Close()
		return nil, fmt.Errorf("firefox started but BiDi endpoint not announced: %w", ctx.Err())
	case <-time.After(bidiBannerTimeout):
		// No banner (a build that logs differently, or a redirected stderr).
		// The port is the contract either way, so fall back to probing it.
		if err := waitForPort(ctx, opts.Port, 5*time.Second); err != nil {
			_ = fp.Close()
			return nil, fmt.Errorf("firefox started but BiDi not reachable at %s: %w", fallback, err)
		}
		fp.wsURL = fallback
	}

	return fp, nil
}

// Endpoint returns the WebDriver BiDi WebSocket URL. Unlike Chrome's HTTP CDP
// endpoint this is already a socket URL, which is what Connect keys on.
func (fp *FirefoxProcess) Endpoint() string { return fp.wsURL }

// Close terminates the Firefox process and all its children, then removes the
// temp profile directory if we created it.
func (fp *FirefoxProcess) Close() error {
	if fp.cmd == nil || fp.cmd.Process == nil {
		return nil
	}
	killProcessTree(fp.cmd)
	if fp.ownsProfile && fp.profileDir != "" {
		_ = os.RemoveAll(fp.profileDir)
	}
	return nil
}

// scanBiDiBanner publishes the first BiDi WebSocket URL Firefox prints, then
// keeps reading so the pipe never fills.
func scanBiDiBanner(stderr io.ReadCloser, out chan<- string) {
	defer stderr.Close()
	scanner := bufio.NewScanner(stderr)
	// Firefox prints long lines when it is unhappy; the default 64KB cap is
	// enough, but a lone oversized line must not stop the drain.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	found := false
	for scanner.Scan() {
		if found {
			continue
		}
		if ws := parseBiDiBanner(scanner.Text()); ws != "" {
			found = true
			out <- ws
		}
	}
}

// parseBiDiBanner extracts the WebSocket URL from Firefox's startup banner,
// "WebDriver BiDi listening on ws://127.0.0.1:9222". It returns "" for any
// other line. The URL is returned as printed — see NormalizeWebSocketURL for
// the session path it omits.
func parseBiDiBanner(line string) string {
	idx := strings.Index(line, "ws://")
	if idx < 0 {
		return ""
	}
	url := strings.TrimSpace(line[idx:])
	// Trim anything the log line appended after the URL.
	if cut := strings.IndexAny(url, " \t\"'"); cut > 0 {
		url = url[:cut]
	}
	return url
}

// waitForPort blocks until the given local TCP port accepts a connection.
func waitForPort(ctx context.Context, port int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s: %w", addr, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// findFirefox searches for a Firefox binary in common locations.
func findFirefox(channel string) (string, error) {
	var candidates []string
	// Channel-selected binaries take precedence over platform defaults.
	if channel != "" {
		if bins, ok := firefoxChannelBinaries[strings.ToLower(channel)]; ok {
			candidates = append(candidates, bins...)
		} else {
			candidates = append(candidates, channel) // treat as a bare binary name
		}
	}
	switch runtime.GOOS {
	case "linux":
		candidates = append(candidates, "firefox", "firefox-esr")
	case "darwin":
		candidates = append(candidates,
			"/Applications/Firefox.app/Contents/MacOS/firefox",
			"firefox",
		)
	case "windows":
		candidates = append(candidates,
			`C:\Program Files\Mozilla Firefox\firefox.exe`,
			`C:\Program Files (x86)\Mozilla Firefox\firefox.exe`,
		)
	default:
		candidates = append(candidates, "firefox")
	}

	for _, c := range candidates {
		// Absolute path — check existence directly.
		if strings.Contains(c, "/") || strings.Contains(c, `\`) {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
			continue
		}
		// Short name — look up in PATH.
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("firefox not found; install Firefox or set it in PATH")
}

// firefoxPrefs are the profile preferences that keep an automated Firefox out
// of the way: no first-run tour, no update checks, no password or form
// autofill prompts, no notification permission dialogs. They are the Firefox
// counterpart of writeAutomationPrefs' Chrome Preferences file.
var firefoxPrefs = []string{
	// First run and session restore.
	`user_pref("browser.shell.checkDefaultBrowser", false);`,
	`user_pref("browser.startup.homepage", "about:blank");`,
	`user_pref("browser.startup.page", 0);`,
	`user_pref("browser.aboutwelcome.enabled", false);`,
	`user_pref("startup.homepage_welcome_url", "about:blank");`,
	`user_pref("startup.homepage_welcome_url.additional", "");`,
	`user_pref("browser.sessionstore.resume_from_crash", false);`,
	`user_pref("toolkit.startup.max_resumed_crashes", -1);`,
	// Updates and background networking.
	`user_pref("app.update.auto", false);`,
	`user_pref("app.update.enabled", false);`,
	`user_pref("browser.search.update", false);`,
	`user_pref("extensions.update.enabled", false);`,
	`user_pref("network.captive-portal-service.enabled", false);`,
	// Telemetry and data reporting.
	`user_pref("datareporting.policy.dataSubmissionEnabled", false);`,
	`user_pref("datareporting.healthreport.uploadEnabled", false);`,
	`user_pref("toolkit.telemetry.enabled", false);`,
	`user_pref("toolkit.telemetry.unified", false);`,
	// Passwords, autofill and the dialogs they raise.
	`user_pref("signon.rememberSignons", false);`,
	`user_pref("signon.autofillForms", false);`,
	`user_pref("extensions.formautofill.addresses.enabled", false);`,
	`user_pref("extensions.formautofill.creditCards.enabled", false);`,
	// Modals that would block a run.
	`user_pref("dom.disable_beforeunload", true);`,
	`user_pref("browser.tabs.warnOnClose", false);`,
	`user_pref("browser.tabs.warnOnCloseOtherTabs", false);`,
	`user_pref("permissions.default.desktop-notification", 2);`,
	// Downloads land somewhere predictable, without a prompt.
	`user_pref("browser.download.useDownloadDir", true);`,
	`user_pref("browser.download.folderList", 2);`,
}

// writeFirefoxPrefs writes a user.js into the profile. Firefox reads user.js on
// every start and copies it over prefs.js, so these survive a profile that has
// been used before — which a caller-supplied UserDataDir may well have been.
func writeFirefoxPrefs(profileDir string) error {
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return err
	}
	lines := append([]string{}, firefoxPrefs...)
	lines = append(lines, fmt.Sprintf(`user_pref("browser.download.dir", %q);`, filepath.ToSlash(os.TempDir())))
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filepath.Join(profileDir, "user.js"), []byte(content), 0o644)
}
