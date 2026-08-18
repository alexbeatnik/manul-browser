package browser

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeEngine(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", EngineChromium, false},
		{"chromium", EngineChromium, false},
		{"Chrome", EngineChromium, false},
		{"msedge", EngineChromium, false},
		// The deprecated spelling of browser_mode "attach" still names a
		// Chromium-family browser when a caller launches one anyway.
		{"electron", EngineChromium, false},
		{"firefox", EngineFirefox, false},
		{"Firefox", EngineFirefox, false},
		{"mozilla", EngineFirefox, false},
		{"gecko", EngineFirefox, false},
		{"  firefox  ", EngineFirefox, false},
		// webkit was listed in the config docs but never implemented; it used
		// to launch Chrome silently, which is worse than saying so.
		{"webkit", "", true},
		{"safari", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeEngine(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("NormalizeEngine(%q) = %q, want an error", tc.in, got)
			}
			if !strings.Contains(err.Error(), "firefox") {
				t.Fatalf("error should name the supported engines: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NormalizeEngine(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("NormalizeEngine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLaunch_RejectsUnsupportedEngineBeforeSpawning(t *testing.T) {
	_, err := Launch(context.Background(), LaunchOptions{Browser: "webkit"})
	if err == nil {
		t.Fatal("expected an unsupported engine to fail")
	}
	if !strings.Contains(err.Error(), "webkit") {
		t.Fatalf("error should name the rejected engine: %v", err)
	}
}

// Connect keys on the endpoint, not on a separately-carried engine name: a
// ws:// URL is BiDi (Firefox), everything else is CDP. Getting this backwards
// would speak the wrong protocol at a browser that cannot answer it.
func TestConnect_SelectsBackendByEndpointScheme(t *testing.T) {
	cases := []struct {
		endpoint string
		wantBiDi bool
	}{
		{"ws://127.0.0.1:9222/session", true},
		{"WS://127.0.0.1:9222/session", true},
		{"wss://example.test/session", true},
		{"  ws://127.0.0.1:9222/session", true},
		{"http://127.0.0.1:9222", false},
		{"127.0.0.1:9222", false},
		{"", false},
	}
	for _, tc := range cases {
		b := Connect(tc.endpoint)
		_, isBiDi := b.(*BiDiBrowser)
		if isBiDi != tc.wantBiDi {
			t.Fatalf("Connect(%q) picked bidi=%v, want %v", tc.endpoint, isBiDi, tc.wantBiDi)
		}
	}
}

// DefaultChromeOptions is the old name of DefaultLaunchOptions; callers still
// use it, and it must keep producing the same defaults.
func TestDefaultOptionsAgree(t *testing.T) {
	if DefaultChromeOptions() != DefaultLaunchOptions() {
		t.Fatal("DefaultChromeOptions drifted from DefaultLaunchOptions")
	}
	if DefaultLaunchOptions().Browser != EngineChromium {
		t.Fatal("the default engine must stay Chromium")
	}
}
