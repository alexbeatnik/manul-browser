package runtime

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/config"
	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/explain"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

// The sort control of a real catalogue page (rozetka.com.ua/ua/grass_cutters):
// a native <select> whose option labels are a sentence each. An agent writing
// the step has to reproduce one of them exactly.
var sortOptions = []string{
	"Від дешевих до дорогих",
	"Від дорогих до дешевих",
	"Новинки",
	"За рейтингом",
}

// selectPage builds a page whose only element is that sort control.
func selectPage() *MockPage {
	m := &MockPage{
		URL:   "https://example.test/catalogue/",
		Title: "Catalogue",
		Elements: []dom.ElementSnapshot{{
			ID:        1,
			XPath:     "//select[@id='sort']",
			Tag:       "select",
			AriaLabel: "Сортування",
			IsVisible: true,
			Rect:      dom.Rect{Top: 10, Left: 10, Width: 200, Height: 40},
		}},
	}
	m.Elements[0].Normalize()
	return m
}

func runSelect(t *testing.T, m *MockPage, value string) explain.ExecutionResult {
	t.Helper()
	rt := New(config.Config{}, m, utils.NewLoggerTo(nopWriter{}, nil))
	hunt := &dsl.Hunt{Commands: []dsl.Command{{
		Type:     dsl.CmdSelect,
		Target:   "Сортування",
		TypeHint: "dropdown",
		Value:    value,
	}}}
	res, _ := rt.RunHunt(context.Background(), hunt)
	if res == nil || len(res.Results) == 0 {
		t.Fatal("hunt produced no step result")
	}
	return res.Results[0]
}

// A SELECT naming an option the page does not have must fail.
//
// It used to pass. The injected script walked el.options, matched nothing,
// returned undefined, and nobody looked at the result — so the step reported
// success over an untouched page. Found on a live catalogue: "SELECT
// 'Спочатку дешеві'" answered ok=true while the sort stayed on
// sort=expensive, and so did the lower-cased spelling of a real option.
//
// That is what makes a driver's imprecise label unrecoverable: an LLM that
// invents "Спочатку дешеві" is told it succeeded, so it never retries with a
// label that exists. A miss has to be reported as a miss.
func TestSelect_MissingOptionFailsTheStep(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"label the page does not have", "Спочатку дешеві"},
		{"label of a different control", "Показувати по 60"},
		// A padded or differently-cased spelling of a real option is NOT here
		// on purpose: those name an option that exists, and belong in
		// TestSelect_MatchesOptionLabelLoosely.
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := selectPage()
			// Answer the way the page would: report whether an option
			// matched. Only the exact spelling exists in the DOM.
			m.EvalResult = func(expr string) ([]byte, bool) {
				if !strings.Contains(expr, "options") {
					return nil, false
				}
				for _, opt := range sortOptions {
					if strings.Contains(expr, opt) {
						return []byte(`true`), true
					}
				}
				return []byte(`false`), true
			}

			res := runSelect(t, m, tc.value)

			if res.Success {
				t.Fatalf("step passed though no option was selected (value %q)", tc.value)
			}
			if res.FailureReason != explain.ReasonNotFound &&
				res.FailureReason != explain.ReasonActionFailed {
				t.Errorf("failure reason = %q, want not_found or action_failed", res.FailureReason)
			}
			if !strings.Contains(res.Error, strings.TrimSpace(tc.value)) {
				t.Errorf("error should name the option that was not found, got %q", res.Error)
			}
		})
	}
}

// The two spellings above are the same option to a human, and the engine
// resolves elements case-insensitively everywhere else. Options should be
// matched the same way: normalise whitespace and case before comparing, so a
// label that is right apart from its capitals still selects.
func TestSelect_MatchesOptionLabelLoosely(t *testing.T) {
	for _, value := range []string{
		"Від дешевих до дорогих",
		"від дешевих до дорогих",
		"  Від дешевих до дорогих ",
		"ВІД ДЕШЕВИХ ДО ДОРОГИХ",
	} {
		t.Run(value, func(t *testing.T) {
			m := selectPage()
			var script string
			m.EvalResult = func(expr string) ([]byte, bool) {
				if !strings.Contains(expr, "options") {
					return nil, false
				}
				script = expr
				return []byte(`true`), true
			}

			res := runSelect(t, m, value)
			if !res.Success {
				t.Fatalf("step failed: %s", res.Error)
			}
			if script == "" {
				t.Fatal("no select script was injected")
			}
			// The comparison has to be normalised in the page, where the
			// option text lives — the engine cannot know it from here.
			lower := strings.ToLower(script)
			if !strings.Contains(lower, "tolowercase") {
				t.Errorf("select script compares option text case-sensitively:\n%s", script)
			}
			if !strings.Contains(lower, "trim") && !strings.Contains(lower, `replace(/\s+/g`) {
				t.Errorf("select script does not normalise whitespace:\n%s", script)
			}
		})
	}
}

// Setting .value straight bypasses the value tracker frameworks keep, and a
// bare 'change' is not what a user interaction produces. FILL already goes
// through the native setter and emits both events (see pkg/pagejs); SELECT
// writes its own script and does neither, so a framework-controlled select
// can ignore the whole thing.
func TestSelect_ScriptDrivesFrameworkBoundSelects(t *testing.T) {
	m := selectPage()
	var script string
	m.EvalResult = func(expr string) ([]byte, bool) {
		if !strings.Contains(expr, "options") {
			return nil, false
		}
		script = expr
		return []byte(`true`), true
	}

	if res := runSelect(t, m, "Новинки"); !res.Success {
		t.Fatalf("step failed: %s", res.Error)
	}
	if script == "" {
		t.Fatal("no select script was injected")
	}
	for _, want := range []string{"input", "change"} {
		if !strings.Contains(script, "'"+want+"'") && !strings.Contains(script, `"`+want+`"`) {
			t.Errorf("select script never dispatches %q:\n%s", want, script)
		}
	}
	if !strings.Contains(script, "getOwnPropertyDescriptor") {
		t.Errorf("select script assigns .value directly instead of through the native setter:\n%s", script)
	}
}

// A label must survive the trip into the script as a string literal. Escaping
// only `"` by hand leaves a backslash or a newline to produce a SyntaxError in
// the page — which, like every other failure here, used to be silent.
func TestSelect_EscapesTheOptionLabel(t *testing.T) {
	m := selectPage()
	var script string
	m.EvalResult = func(expr string) ([]byte, bool) {
		if !strings.Contains(expr, "options") {
			return nil, false
		}
		script = expr
		return []byte(`false`), true
	}

	const label = "10\" \\ 12\nnext"
	_ = runSelect(t, m, label)
	if script == "" {
		t.Fatal("no select script was injected")
	}
	// Quoted exactly once, by the same rules the page will unquote by.
	if !strings.Contains(script, strconv.Quote(label)) {
		t.Errorf("label is not embedded as an escaped literal (want %s):\n%s",
			strconv.Quote(label), script)
	}
	if strings.Contains(script, "\n"+"next") {
		t.Errorf("a raw newline reached the script, which would not parse:\n%s", script)
	}
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
