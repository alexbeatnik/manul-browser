package serve

import (
	"testing"
)

// EvalJS is not uniform about its output, and parsing it strictly turned every
// string a page produced into null. This is the guard for that.
func TestDecodeEvalResult(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want any
	}{
		{"empty is nil", "", nil},
		{"number", "2", float64(2)},
		{"boolean", "true", true},
		{"json null", "null", nil},
		{"quoted string", `"Widget Page"`, "Widget Page"},
		// The case that mattered: a bare, unquoted string result.
		{"bare string", "Widget Page", "Widget Page"},
		{"bare string that looks numeric-ish", "no", "no"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeEvalResult([]byte(tc.raw))
			if got != tc.want {
				t.Errorf("decodeEvalResult(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestDecodeEvalResult_Object(t *testing.T) {
	got, ok := decodeEvalResult([]byte(`{"a":1}`)).(map[string]any)
	if !ok {
		t.Fatalf("want a decoded object, got %T", got)
	}
	if got["a"] != float64(1) {
		t.Errorf("a = %#v, want 1", got["a"])
	}
}

// ── register ─────────────────────────────────────────────────────────────────

// Handlers describe the client, not the browser, so declaring them before a
// session exists must work — a decorator runs at import time.
func TestRegisterWorksBeforeOpen(t *testing.T) {
	got := runLines(t, `{"id":1,"cmd":"register","args":{"controls":[{"page":"Login Page","target":"Username"}],"calls":["compute_total"]}}`)
	last := got[len(got)-1]
	if last["ok"] != true {
		t.Fatalf("register failed: %v", last)
	}
	res, _ := last["result"].(map[string]any)
	if res["controls"] != float64(1) || res["calls"] != float64(1) {
		t.Errorf("counts = %v, want 1 and 1", res)
	}
}

// A control with no page applies everywhere, which is what a caller who left it
// out meant — it must not be rejected.
func TestRegisterControlWithoutPageIsAccepted(t *testing.T) {
	got := runLines(t, `{"id":1,"cmd":"register","args":{"controls":[{"target":"Signature Pad"}]}}`)
	last := got[len(got)-1]
	if last["ok"] != true {
		t.Errorf("a page-less control should register as a wildcard: %v", last)
	}
}

func TestRegisterRejectsEmptyNames(t *testing.T) {
	cases := map[string]string{
		"control without a target": `{"id":1,"cmd":"register","args":{"controls":[{"page":"P","target":"  "}]}}`,
		"call without a name":      `{"id":1,"cmd":"register","args":{"calls":["  "]}}`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			got := runLines(t, line)
			last := got[len(got)-1]
			errObj, _ := last["error"].(map[string]any)
			if last["ok"] != false || errObj["code"] != CodeBadRequest {
				t.Errorf("want bad_request, got %v", last)
			}
		})
	}
}

func TestRegisterWithNothingIsHarmless(t *testing.T) {
	got := runLines(t, `{"id":1,"cmd":"register","args":{}}`)
	last := got[len(got)-1]
	if last["ok"] != true {
		t.Errorf("an empty registration should succeed: %v", last)
	}
}
