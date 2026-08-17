// Package browser — engine selection: which browser to start, and which
// protocol to drive it with once it is up.
package browser

import (
	"context"
	"fmt"
	"strings"
)

// Engine names accepted by Launch and by the `browser` config field.
const (
	EngineChromium = "chromium"
	EngineFirefox  = "firefox"
)

// NormalizeEngine maps a user-supplied browser name onto an engine this build
// can launch. An empty name is Chromium, the default the engine has always had.
//
// "electron" is accepted as Chromium because it is the deprecated spelling of
// browser_mode "attach" (see config.ResolveBrowserMode); it reaches Launch only
// when a caller has also asked for launch mode explicitly.
func NormalizeEngine(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "chromium", "chrome", "chromium-browser", "msedge", "edge", "electron":
		return EngineChromium, nil
	case "firefox", "mozilla", "gecko":
		return EngineFirefox, nil
	default:
		return "", fmt.Errorf("unsupported browser %q: want %q or %q", name, EngineChromium, EngineFirefox)
	}
}

// LaunchOptions configures the browser process to spawn. Most fields apply to
// both engines; the ones that do not say so.
type LaunchOptions struct {
	// Browser selects the engine: "chromium" (default) or "firefox".
	Browser string
	// Port for the browser's remote debugging protocol. Default: 9222.
	Port int
	// UserDataDir is the browser profile directory.
	// If empty, a unique temp directory is created per run and cleaned up on Close.
	UserDataDir string
	// DisableGPU disables GPU acceleration. Chromium only. Default: true.
	DisableGPU bool
	// Headless runs the browser without a visible window.
	Headless bool
	// ExecutablePath overrides the browser binary location.
	ExecutablePath string
	// Channel selects a system binary by name — for Chromium (chrome,
	// chrome-beta, chrome-dev, chromium, msedge) or Firefox (firefox,
	// firefox-esr, firefox-dev, firefox-nightly). Empty = platform defaults.
	// Set by `--channel` / MANUL_CHANNEL.
	Channel string
}

// ChromeOptions is the former name of LaunchOptions, from when Chromium was
// the only engine. Retained so existing callers keep compiling.
type ChromeOptions = LaunchOptions

// DefaultLaunchOptions returns sensible defaults for automation.
// UserDataDir is left empty so the launcher creates a unique temp directory.
func DefaultLaunchOptions() LaunchOptions {
	return LaunchOptions{
		Browser:    EngineChromium,
		Port:       9222,
		DisableGPU: true,
		Headless:   false,
	}
}

// DefaultChromeOptions is DefaultLaunchOptions under its former name.
func DefaultChromeOptions() LaunchOptions { return DefaultLaunchOptions() }

// Process is a browser the engine started and owns: an endpoint to drive it
// through, and the teardown that kills it. Both engines' processes satisfy it,
// which is what lets every caller launch one without knowing which it got.
type Process interface {
	// Endpoint is what Connect takes: a CDP HTTP endpoint for Chromium, a
	// WebDriver BiDi WebSocket URL for Firefox.
	Endpoint() string
	// Close kills the browser and removes the profile directory the launcher
	// created. It does not close individual pages.
	Close() error
}

// Launch starts the browser named by opts.Browser and blocks until it is
// reachable, or the context expires.
func Launch(ctx context.Context, opts LaunchOptions) (Process, error) {
	engine, err := NormalizeEngine(opts.Browser)
	if err != nil {
		return nil, err
	}
	if engine == EngineFirefox {
		return LaunchFirefox(ctx, opts)
	}
	return LaunchChrome(ctx, opts)
}

// Backend is a live connection to a browser: the Browser interface plus the
// tab-level operations the engine needs (picking a tab by URL, opening and
// reaping background tabs). Both protocol backends implement all of it.
type Backend interface {
	Browser

	// PageMatching attaches to the page whose URL contains urlSubstr
	// (case-insensitive); an empty substring means FirstPage.
	PageMatching(ctx context.Context, urlSubstr string) (Page, error)

	// OpenTarget opens a background tab at url and returns it with the id
	// CloseTarget reaps it by.
	OpenTarget(ctx context.Context, url string) (Page, string, error)

	// CloseTarget closes a tab by the id OpenTarget returned.
	CloseTarget(ctx context.Context, targetID string) error
}

// Connect returns the backend that speaks the protocol behind endpoint.
//
// The endpoint says which: a ws:// or wss:// URL is WebDriver BiDi, which is
// what Firefox has spoken since it removed CDP in 141, and everything else is
// a CDP HTTP endpoint. Callers therefore do not have to carry the engine name
// alongside the endpoint — Process.Endpoint already encodes it.
func Connect(endpoint string) Backend {
	e := strings.ToLower(strings.TrimSpace(endpoint))
	if strings.HasPrefix(e, "ws://") || strings.HasPrefix(e, "wss://") {
		return NewBiDiBrowser(endpoint)
	}
	return NewCDPBrowser(endpoint)
}
