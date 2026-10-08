// Package browser — Chrome process lifecycle management.
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ChromeProcess manages a Chrome browser process spawned for automation.
type ChromeProcess struct {
	cmd         *exec.Cmd
	port        int
	userDataDir string
	ownsDataDir bool // true when we created the dir and should clean it up
}

// channelBinaries maps a channel name to the concrete binaries to probe (in
// order).
var channelBinaries = map[string][]string{
	"chrome":      {"google-chrome-stable", "google-chrome"},
	"chrome-beta": {"google-chrome-beta"},
	"chrome-dev":  {"google-chrome-unstable"},
	"chromium":    {"chromium", "chromium-browser"},
	"msedge":      {"microsoft-edge-stable", "microsoft-edge"},
}

// headlessWindowSize is the window a headless Chrome is given: the size
// headless Firefox uses unasked, so one hunt meets one layout in both.
const headlessWindowSize = "--window-size=1366,768"

// cdpStartupTimeout bounds the wait for a launched Chrome to answer on CDP,
// port discovery included.
const cdpStartupTimeout = 15 * time.Second

// devToolsActivePortFile is where Chrome reports the debugging port it chose.
// It is written into the profile directory once the port is listening: the
// port on the first line, the browser's WebSocket path on the second.
const devToolsActivePortFile = "DevToolsActivePort"

// LaunchChrome starts a Chrome process with remote debugging enabled.
// It blocks until Chrome's CDP endpoint is reachable (or context expires).
// If opts.UserDataDir is empty, a unique temp directory is created and owned
// by the returned ChromeProcess (removed when Close is called).
//
// opts.Port 0 asks Chrome for a free port (--remote-debugging-port=0) and
// reads the one it picked from DevToolsActivePort; Endpoint reports it. A
// non-zero port is passed through as given.
//
// opts.Browser is ignored here; Launch is the entry point that honours it.
func LaunchChrome(ctx context.Context, opts LaunchOptions) (*ChromeProcess, error) {
	chromePath := opts.ExecutablePath
	if chromePath == "" {
		var err error
		chromePath, err = findChrome(opts.Channel)
		if err != nil {
			return nil, err
		}
	}

	ownsDir := false
	if opts.UserDataDir == "" {
		dir, err := os.MkdirTemp("", "manul-chrome-*")
		if err != nil {
			return nil, fmt.Errorf("create chrome temp dir: %w", err)
		}
		opts.UserDataDir = dir
		ownsDir = true
	}

	// Write Chrome preferences to disable password manager at profile level.
	if err := writeAutomationPrefs(opts.UserDataDir); err != nil {
		if ownsDir {
			_ = os.RemoveAll(opts.UserDataDir)
		}
		return nil, fmt.Errorf("write chrome prefs: %w", err)
	}

	if opts.Port == 0 {
		// A profile that has been launched before still holds the port of that
		// run, and it would be read back as this one's.
		if err := removeStaleDevToolsPort(opts.UserDataDir); err != nil {
			if ownsDir {
				_ = os.RemoveAll(opts.UserDataDir)
			}
			return nil, err
		}
	}

	// Chrome must outlive ctx: the caller's context typically scopes one task
	// (a single prompt/pipeline turn in an embedding app), while the browser is
	// a long-lived resource shared across tasks. exec.CommandContext would kill
	// Chrome the moment that context is cancelled — the user watched the browser
	// close seconds after every action that had launched it. ctx still bounds
	// the startup phase (waitForCDP below); after that only Close kills Chrome.
	cmd := exec.Command(chromePath, chromeArgs(opts)...)
	// Detach stdout/stderr — Chrome is noisy by default.
	cmd.Stdout = nil
	cmd.Stderr = nil
	// Inherit environment (required for DISPLAY on Linux).
	cmd.Env = os.Environ()
	// Platform-specific process group setup (implemented in chrome_unix.go / chrome_windows.go).
	setProcGroup(cmd)

	if err := cmd.Start(); err != nil {
		if ownsDir {
			_ = os.RemoveAll(opts.UserDataDir)
		}
		return nil, fmt.Errorf("start chrome: %w", err)
	}
	tieToEngine(cmd)

	cp := &ChromeProcess{
		cmd:         cmd,
		port:        opts.Port,
		userDataDir: opts.UserDataDir,
		ownsDataDir: ownsDir,
	}

	ctx, cancel := context.WithTimeout(ctx, cdpStartupTimeout)
	defer cancel()

	if opts.Port == 0 {
		port, err := waitForDevToolsPort(ctx, opts.UserDataDir)
		if err != nil {
			_ = cp.Close()
			return nil, fmt.Errorf("chrome started but reported no debugging port: %w", err)
		}
		cp.port = port
	}

	// Wait for CDP endpoint to become reachable.
	endpoint := cp.Endpoint()
	if err := waitForCDP(ctx, endpoint, cdpStartupTimeout); err != nil {
		// Chrome started but CDP never became reachable — kill it.
		_ = cp.Close()
		return nil, fmt.Errorf("chrome started but CDP not reachable at %s: %w", endpoint, err)
	}

	return cp, nil
}

// chromeArgs is the command line LaunchChrome starts Chrome with.
// opts.UserDataDir must already be set.
func chromeArgs(opts LaunchOptions) []string {
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", opts.Port),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-client-side-phishing-detection",
		"--disable-default-apps",
		"--disable-extensions",
		"--disable-hang-monitor",
		"--disable-popup-blocking",
		"--disable-prompt-on-repost",
		"--disable-sync",
		"--disable-translate",
		"--disable-search-engine-choice-screen",
		"--disable-features=PasswordLeakDetection,PasswordManagerOnboarding,PasswordCheck,ChromePasswordManagerUI,CredentialManager,AutofillServerCommunication,IdentityStatusDialog,GlobalMediaControls,MediaRouter,Translate,OptimizationHints",
		"--no-service-autorun",
		"--password-store=basic",
		"--disable-save-password-bubble",
		"--disable-component-update",
		"--disable-infobars",
		fmt.Sprintf("--user-data-dir=%s", opts.UserDataDir),
	}
	if opts.DisableGPU {
		args = append(args, "--disable-gpu")
	}
	if opts.Headless {
		// Headless Chrome's own default is an 800×600 window, small enough
		// that responsive sites serve their phone layout — a different page
		// from the one the same hunt drives in a headed browser, and from the
		// one headless Firefox draws at its default of 1366×768.
		args = append(args, "--headless=new", headlessWindowSize)
	}
	return args
}

// removeStaleDevToolsPort deletes the DevToolsActivePort a previous run left
// in userDataDir. A profile that has none is not an error.
func removeStaleDevToolsPort(userDataDir string) error {
	err := os.Remove(filepath.Join(userDataDir, devToolsActivePortFile))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale %s: %w", devToolsActivePortFile, err)
	}
	return nil
}

// parseDevToolsActivePort returns the port named on the first line of a
// DevToolsActivePort file. The line must be complete: a file caught half
// written would otherwise yield the leading digits of the real port.
func parseDevToolsActivePort(data []byte) (int, bool) {
	line, _, complete := strings.Cut(string(data), "\n")
	if !complete {
		return 0, false
	}
	port, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}

// waitForDevToolsPort polls userDataDir's DevToolsActivePort until Chrome has
// written a valid port into it, or ctx expires.
func waitForDevToolsPort(ctx context.Context, userDataDir string) (int, error) {
	path := filepath.Join(userDataDir, devToolsActivePortFile)
	for {
		// A missing or unreadable file is Chrome not having got there yet.
		if data, err := os.ReadFile(path); err == nil {
			if port, ok := parseDevToolsActivePort(data); ok {
				return port, nil
			}
		}
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("timeout waiting for %s: %w", path, ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Endpoint returns the HTTP CDP endpoint URL.
func (cp *ChromeProcess) Endpoint() string {
	return fmt.Sprintf("http://127.0.0.1:%d", cp.port)
}

// Close terminates the Chrome process and all its children, then removes the
// temp profile directory if we created it.
func (cp *ChromeProcess) Close() error {
	if cp.cmd == nil || cp.cmd.Process == nil {
		return nil
	}
	killProcessTree(cp.cmd)
	if cp.ownsDataDir && cp.userDataDir != "" {
		_ = os.RemoveAll(cp.userDataDir)
	}
	return nil
}

// findChrome searches for a Chrome binary in common locations.
func findChrome(channel string) (string, error) {
	var candidates []string
	// Channel-selected binaries take precedence over platform defaults.
	if channel != "" {
		if bins, ok := channelBinaries[strings.ToLower(channel)]; ok {
			candidates = append(candidates, bins...)
		} else {
			candidates = append(candidates, channel) // treat as a bare binary name
		}
	}
	switch runtime.GOOS {
	case "linux":
		candidates = append(candidates,
			"google-chrome-stable",
			"google-chrome",
			"chromium-browser",
			"chromium",
		)
	case "darwin":
		candidates = append(candidates,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"google-chrome",
			"chromium",
		)
	case "windows":
		candidates = append(candidates,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		)
	default:
		candidates = append(candidates, "google-chrome", "chromium")
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

	return "", fmt.Errorf("chrome not found; install Google Chrome or set it in PATH")
}

// waitForCDP polls the CDP /json endpoint until it responds or timeout expires.
// Uses context.WithTimeout so both the outer context deadline and the
// internal timeout are respected — whichever fires first wins.
func waitForCDP(ctx context.Context, endpoint string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	url := endpoint + "/json"
	client := &http.Client{Timeout: 2 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for CDP at %s: %w", endpoint, ctx.Err())
		default:
		}

		// Quick TCP check first (cheaper than full HTTP).
		u, _ := neturl.Parse(endpoint)
		conn, err := net.DialTimeout("tcp", u.Host, 500*time.Millisecond)
		if err != nil {
			select {
			case <-ctx.Done():
				return fmt.Errorf("timeout waiting for CDP at %s: %w", endpoint, ctx.Err())
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}
		conn.Close()

		// TCP is open — verify CDP responds.
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("create CDP probe request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for CDP at %s: %w", endpoint, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// writeAutomationPrefs writes a Chrome Preferences file that disables the
// password manager, autofill, credential prompts, and other dialogs that
// interfere with automation. This is necessary because CLI flags alone do not
// suppress all Chrome-managed modals (e.g. password breach warnings).
func writeAutomationPrefs(userDataDir string) error {
	defaultDir := filepath.Join(userDataDir, "Default")
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		return err
	}

	prefs := map[string]any{
		"credentials_enable_service":    false,
		"credentials_enable_autosignin": false,
		"profile": map[string]any{
			"password_manager_enabled":        false,
			"password_manager_leak_detection": false,
			"default_content_setting_values": map[string]any{
				"notifications": 2, // block
			},
		},
		"autofill": map[string]any{
			"profile_enabled":     false,
			"credit_card_enabled": false,
		},
		"savefile": map[string]any{
			"default_directory": os.TempDir(),
		},
		"download": map[string]any{
			"prompt_for_download": false,
		},
		"password_manager": map[string]any{
			"enabled": false,
		},
	}

	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(defaultDir, "Preferences"), data, 0o644)
}
