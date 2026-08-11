package config

import "strings"

// Browser modes returned by ResolveBrowserMode.
const (
	// ModeLaunch starts a fresh Chrome that the engine owns and closes.
	ModeLaunch = "launch"
	// ModeAttach drives a Chrome that is already running. The engine does not
	// close it — it did not open it.
	ModeAttach = "attach"
)

// ResolveBrowserMode answers the one question every session must settle: start
// a new Chrome, or drive one that is already running.
//
// Historically that bit was spread across two keys that could disagree —
// CDPEndpoint (set ⇒ attach) and Browser ("electron" ⇒ attach). BrowserMode is
// the explicit answer; the rest are inference kept for existing configs.
//
// Precedence, highest first:
//
//  1. BrowserMode, when it names a known mode.
//  2. Browser == "electron" — the deprecated spelling of attach.
//  3. CDPEndpoint being set at all.
//  4. Launch.
func (c Config) ResolveBrowserMode() string {
	switch strings.ToLower(strings.TrimSpace(c.BrowserMode)) {
	case ModeAttach:
		return ModeAttach
	case ModeLaunch:
		return ModeLaunch
	}

	if strings.EqualFold(strings.TrimSpace(c.Browser), "electron") {
		return ModeAttach
	}
	if strings.TrimSpace(c.CDPEndpoint) != "" {
		return ModeAttach
	}
	return ModeLaunch
}

// BrowserModeIsDeprecatedSpelling reports whether attach was selected only by
// the legacy `browser: electron` alias, so callers can warn once instead of
// silently honouring a spelling that is on its way out.
func (c Config) BrowserModeIsDeprecatedSpelling() bool {
	switch strings.ToLower(strings.TrimSpace(c.BrowserMode)) {
	case ModeAttach, ModeLaunch:
		return false
	}
	return strings.EqualFold(strings.TrimSpace(c.Browser), "electron")
}

// DefaultCDPEndpoint is the endpoint used for attach when none is configured.
const DefaultCDPEndpoint = "http://127.0.0.1:9222"

// AttachEndpoint returns the endpoint an attach session should dial.
func (c Config) AttachEndpoint() string {
	if e := strings.TrimSpace(c.CDPEndpoint); e != "" {
		return e
	}
	return DefaultCDPEndpoint
}
