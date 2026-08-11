package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/config"
)

// runLines feeds the given request lines through a server and returns the
// decoded output lines. No browser is involved: every case here is answered
// before a session would be needed.
func runLines(t *testing.T, lines ...string) []map[string]any {
	t.Helper()

	in := strings.NewReader(strings.Join(lines, "\n") + "\n")
	var out bytes.Buffer

	opts := Options{
		EngineVersion: "9.9.9",
		Config:        config.Default(),
		Schema:        func() map[string]any { return map[string]any{"verbs": []string{"CLICK"}} },
	}
	if err := Serve(context.Background(), in, &out, opts); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var got []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("output line is not JSON: %q: %v", l, err)
		}
		got = append(got, m)
	}
	return got
}

// The ready event must be the very first line: a binding that does not see it
// treats the process as failed rather than hanging.
func TestReadyIsFirstLine(t *testing.T) {
	got := runLines(t, `{"id":1,"cmd":"close"}`)
	if len(got) < 1 {
		t.Fatal("no output")
	}
	first := got[0]
	if first["event"] != "ready" {
		t.Fatalf("first line is not the ready event: %v", first)
	}
	if first["protocol"] != ProtocolVersion {
		t.Errorf("protocol = %v, want %v", first["protocol"], ProtocolVersion)
	}
	if first["engine"] != "9.9.9" {
		t.Errorf("engine = %v, want 9.9.9", first["engine"])
	}
	// The ready event carries no id — it is not a reply to anything.
	if _, ok := first["id"]; ok {
		t.Error("ready event must not carry an id")
	}
}

func TestCloseEndsTheLoop(t *testing.T) {
	// The line after close must never be read.
	got := runLines(t, `{"id":1,"cmd":"close"}`, `{"id":2,"cmd":"schema"}`)
	if len(got) != 2 {
		t.Fatalf("want ready + close reply only, got %d lines: %v", len(got), got)
	}
	if got[1]["ok"] != true {
		t.Errorf("close should succeed: %v", got[1])
	}
}

func TestUnknownCommand(t *testing.T) {
	got := runLines(t, `{"id":7,"cmd":"teleport"}`)
	last := got[len(got)-1]
	if last["ok"] != false {
		t.Fatalf("unknown cmd should fail: %v", last)
	}
	errObj, _ := last["error"].(map[string]any)
	if errObj["code"] != CodeBadRequest {
		t.Errorf("code = %v, want %v", errObj["code"], CodeBadRequest)
	}
	if last["id"] != float64(7) {
		t.Errorf("id = %v, want 7 — the reply must correlate", last["id"])
	}
}

// A malformed line is answered and the session survives it.
func TestMalformedLineIsNotFatal(t *testing.T) {
	got := runLines(t, `{not json`, `{"id":2,"cmd":"schema"}`)
	if len(got) != 3 {
		t.Fatalf("want ready + error + schema reply, got %d: %v", len(got), got)
	}
	if got[1]["ok"] != false {
		t.Errorf("malformed line should produce an error reply: %v", got[1])
	}
	if got[1]["id"] != nil {
		t.Errorf("id = %v, want null when it could not be recovered", got[1]["id"])
	}
	if got[2]["ok"] != true {
		t.Errorf("the session should survive: %v", got[2])
	}
}

func TestMissingCmd(t *testing.T) {
	got := runLines(t, `{"id":3}`)
	last := got[len(got)-1]
	errObj, _ := last["error"].(map[string]any)
	if last["ok"] != false || errObj["code"] != CodeBadRequest {
		t.Errorf("want bad_request, got %v", last)
	}
}

func TestSchemaIsInjected(t *testing.T) {
	got := runLines(t, `{"id":1,"cmd":"schema"}`)
	last := got[len(got)-1]
	if last["ok"] != true {
		t.Fatalf("schema failed: %v", last)
	}
	res, _ := last["result"].(map[string]any)
	if res["verbs"] == nil {
		t.Errorf("schema payload not passed through: %v", res)
	}
}

// Every page command needs a session; none of them may silently open one.
// Args are otherwise valid here so the request reaches the session check.
func TestPageCommandsRequireOpen(t *testing.T) {
	cases := map[string]string{
		"map":      `{"id":1,"cmd":"map"}`,
		"read":     `{"id":1,"cmd":"read","args":{"label":"Total"}}`,
		"run-step": `{"id":1,"cmd":"run-step","args":{"step":"CLICK the 'Login' button"}}`,
		"run":      `{"id":1,"cmd":"run","args":{"source":"CLICK the 'Login' button"}}`,
		"vars":     `{"id":1,"cmd":"vars"}`,
		"state":    `{"id":1,"cmd":"state"}`,
	}
	for cmd, line := range cases {
		t.Run(cmd, func(t *testing.T) {
			got := runLines(t, line)
			last := got[len(got)-1]
			if last["ok"] != false {
				t.Fatalf("%s without open should fail: %v", cmd, last)
			}
			errObj, _ := last["error"].(map[string]any)
			if errObj["code"] != CodeNotOpen {
				t.Errorf("code = %v, want %v", errObj["code"], CodeNotOpen)
			}
		})
	}
}

// Request-shape problems are reported as such even before a session exists —
// otherwise the caller opens a browser only to hit the same error.
func TestShapeErrorsBeatSessionErrors(t *testing.T) {
	cases := map[string]string{
		"run-step without a step": `{"id":1,"cmd":"run-step","args":{}}`,
		"map with a typed arg":    `{"id":1,"cmd":"map","args":{"maxPerGroup":"lots"}}`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			got := runLines(t, line)
			last := got[len(got)-1]
			errObj, _ := last["error"].(map[string]any)
			if errObj["code"] != CodeBadRequest {
				t.Errorf("code = %v, want %v: %v", errObj["code"], CodeBadRequest, last)
			}
		})
	}
}

// Absent args are as valid as an empty object.
func TestArgsAreOptional(t *testing.T) {
	withArgs := runLines(t, `{"id":1,"cmd":"map","args":{}}`)
	without := runLines(t, `{"id":1,"cmd":"map"}`)

	codeOf := func(lines []map[string]any) any {
		last := lines[len(lines)-1]
		e, _ := last["error"].(map[string]any)
		return e["code"]
	}
	// Both reach the same place — needing a session, not failing to parse.
	if codeOf(withArgs) != CodeNotOpen || codeOf(without) != CodeNotOpen {
		t.Errorf("args:{} = %v, absent = %v; both should be not_open",
			codeOf(withArgs), codeOf(without))
	}
}

func TestBadArgsShape(t *testing.T) {
	got := runLines(t, `{"id":1,"cmd":"map","args":{"maxPerGroup":"lots"}}`)
	last := got[len(got)-1]
	errObj, _ := last["error"].(map[string]any)
	if errObj["code"] != CodeBadRequest {
		t.Errorf("a wrongly typed arg should be bad_request, got %v", last)
	}
}

// Shell pipelines and Windows editors prepend a BOM; it must not turn the
// first request into a parse error.
func TestLeadingBOMIsTolerated(t *testing.T) {
	got := runLines(t, "\ufeff"+`{"id":1,"cmd":"schema"}`)
	last := got[len(got)-1]
	if last["ok"] != true {
		t.Errorf("a BOM-prefixed request should still parse: %v", last)
	}
}

func TestBlankLinesAreSkipped(t *testing.T) {
	got := runLines(t, ``, `   `, `{"id":1,"cmd":"schema"}`)
	if len(got) != 2 {
		t.Fatalf("blank lines should produce no replies, got %d: %v", len(got), got)
	}
}

// ── browser mode resolution ──────────────────────────────────────────────────

func TestResolveBrowserMode(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{"default is launch", config.Config{}, config.ModeLaunch},
		{"explicit launch", config.Config{BrowserMode: "launch"}, config.ModeLaunch},
		{"explicit attach", config.Config{BrowserMode: "attach"}, config.ModeAttach},
		{"case insensitive", config.Config{BrowserMode: "ATTACH"}, config.ModeAttach},
		{"endpoint implies attach", config.Config{CDPEndpoint: "http://127.0.0.1:9222"}, config.ModeAttach},
		{"electron alias implies attach", config.Config{Browser: "electron"}, config.ModeAttach},
		// The explicit key settles the case the two legacy signals could not.
		{"explicit launch beats endpoint", config.Config{BrowserMode: "launch", CDPEndpoint: "http://x"}, config.ModeLaunch},
		{"explicit launch beats electron", config.Config{BrowserMode: "launch", Browser: "electron"}, config.ModeLaunch},
		{"unknown value falls through to inference", config.Config{BrowserMode: "nonsense"}, config.ModeLaunch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.ResolveBrowserMode(); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestDeprecatedSpellingDetection(t *testing.T) {
	if !(config.Config{Browser: "electron"}).BrowserModeIsDeprecatedSpelling() {
		t.Error("browser:electron alone should be flagged")
	}
	if (config.Config{Browser: "electron", BrowserMode: "attach"}).BrowserModeIsDeprecatedSpelling() {
		t.Error("an explicit browser_mode means the alias is not what decided it")
	}
	if (config.Config{CDPEndpoint: "http://x"}).BrowserModeIsDeprecatedSpelling() {
		t.Error("an endpoint is inference, not the deprecated spelling")
	}
}

func TestAttachEndpointFallback(t *testing.T) {
	if got := (config.Config{}).AttachEndpoint(); got != config.DefaultCDPEndpoint {
		t.Errorf("got %q, want the default endpoint", got)
	}
	if got := (config.Config{CDPEndpoint: "http://host:1234"}).AttachEndpoint(); got != "http://host:1234" {
		t.Errorf("a configured endpoint must win, got %q", got)
	}
}

// An unknown mode passed to open must be rejected rather than quietly
// falling back to launching a browser the caller did not ask for.
func TestOpenRejectsUnknownMode(t *testing.T) {
	got := runLines(t, `{"id":1,"cmd":"open","args":{"mode":"telepathy"}}`)
	last := got[len(got)-1]
	if last["ok"] != false {
		t.Fatalf("unknown mode should be rejected, got %v", last)
	}
	errObj, _ := last["error"].(map[string]any)
	if errObj["code"] != CodeBadRequest {
		t.Errorf("code = %v, want %v", errObj["code"], CodeBadRequest)
	}
}
