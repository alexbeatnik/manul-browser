package runtime

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/config"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
)

// ---- ConfidenceLabel ---------------------------------------------------------

func TestWhatIfConfidenceLabel(t *testing.T) {
	cases := []struct {
		score int
		want  string
	}{
		{0, "IMPOSSIBLE"},
		{1, "LOW"},
		{4, "LOW"},
		{5, "MODERATE"},
		{7, "MODERATE"},
		{8, "HIGH"},
		{10, "HIGH"},
	}
	for _, tc := range cases {
		got := WhatIfResult{Score: tc.score}.ConfidenceLabel()
		if got != tc.want {
			t.Errorf("Score %d → %q, want %q", tc.score, got, tc.want)
		}
	}
}

// ---- FormatReport ------------------------------------------------------------

func TestFormatReport_OmitsEmptyOptionalLines(t *testing.T) {
	r := WhatIfResult{
		Step:        "Click the 'Login' button",
		Score:       9,
		TargetFound: true,
		Explanation: "found it",
	}
	out := r.FormatReport()

	for _, want := range []string{
		`WHAT-IF ANALYSIS: "Click the 'Login' button"`,
		"Confidence: 9/10 (HIGH)",
		"Explanation: found it",
		"END",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	// Risk, Suggestion, Target Element and Heuristic Score are all unset here.
	for _, unwanted := range []string{"Risk:", "Suggestion:", "Target Element:", "Heuristic Score:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("report should not contain %q:\n%s", unwanted, out)
		}
	}
}

// A zero heuristic score is a real measurement and must render; a system step
// that was never scored must not. Both have HeuristicScore == 0.
func TestFormatReport_DistinguishesZeroScoreFromUnscored(t *testing.T) {
	scored := WhatIfResult{Step: "x", hasHeuristic: true, HeuristicScore: 0}
	if !strings.Contains(scored.FormatReport(), "Heuristic Score: 0.000") {
		t.Errorf("a scored 0.0 should be reported:\n%s", scored.FormatReport())
	}

	unscored := WhatIfResult{Step: "x"}
	if strings.Contains(unscored.FormatReport(), "Heuristic Score:") {
		t.Errorf("an unscored step should omit the line:\n%s", unscored.FormatReport())
	}
}

// ---- isWhatIfSystemStep ------------------------------------------------------

func TestIsWhatIfSystemStep(t *testing.T) {
	cases := []struct {
		name string
		cmd  dsl.Command
		want bool
	}{
		{"navigate", dsl.Command{Type: dsl.CmdNavigate}, true},
		{"wait", dsl.Command{Type: dsl.CmdWait}, true},
		{"scroll", dsl.Command{Type: dsl.CmdScroll}, true},
		{"click", dsl.Command{Type: dsl.CmdClick}, false},
		{"fill", dsl.Command{Type: dsl.CmdFill}, false},
		{"extract", dsl.Command{Type: dsl.CmdExtract}, false},
		// SET splits on whether it assigns a variable or fills a field.
		{"set variable", dsl.Command{Type: dsl.CmdSet, SetVar: "total"}, true},
		{"set field", dsl.Command{Type: dsl.CmdSet}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isWhatIfSystemStep(tc.cmd); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

// ---- pickWhatIfStep ----------------------------------------------------------

func TestPickWhatIfStep(t *testing.T) {
	rt := newTestRuntime(config.Default())
	rt.whatIfHistory = []WhatIfResult{
		{Step: "first"},
		{Step: "second"},
		{Step: "third"},
	}

	cases := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{"bare takes the newest", "!execute", "third", true},
		{"index is 1-based", "!execute 1", "first", true},
		{"index in the middle", "!execute 2", "second", true},
		{"index at the end", "!execute 3", "third", true},
		{"index past the end", "!execute 4", "", false},
		{"zero is not a valid index", "!execute 0", "", false},
		{"non-numeric argument", "!execute foo", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			got, ok := rt.pickWhatIfStep(&buf, tc.input)
			if ok != tc.ok || got != tc.want {
				t.Errorf("got (%q, %v) want (%q, %v)", got, ok, tc.want, tc.ok)
			}
			if !ok && buf.Len() == 0 {
				t.Error("a rejection should explain itself to the user")
			}
		})
	}
}

func TestPickWhatIfStep_EmptyHistory(t *testing.T) {
	rt := newTestRuntime(config.Default())
	var buf bytes.Buffer
	if _, ok := rt.pickWhatIfStep(&buf, "!execute"); ok {
		t.Error("!execute with no history should be rejected")
	}
	if !strings.Contains(buf.String(), "No evaluations") {
		t.Errorf("unhelpful message: %q", buf.String())
	}
}

// ---- takeWhatIfReplacement ---------------------------------------------------

func TestTakeWhatIfReplacement_IgnoresOtherErrors(t *testing.T) {
	rt := newTestRuntime(config.Default())
	rt.whatIfExecuteStep = "CLICK the 'Login' button"

	original := dsl.Command{Type: dsl.CmdClick, Raw: "original"}
	if _, ok := rt.takeWhatIfReplacement(errors.New("boom"), original); ok {
		t.Error("a plain error must not be treated as the replacement signal")
	}
	if rt.whatIfExecuteStep == "" {
		t.Error("a plain error must not consume the pending step")
	}
}

func TestTakeWhatIfReplacement_NilError(t *testing.T) {
	rt := newTestRuntime(config.Default())
	original := dsl.Command{Type: dsl.CmdClick, Raw: "original"}
	if _, ok := rt.takeWhatIfReplacement(nil, original); ok {
		t.Error("nil must not be treated as the replacement signal")
	}
}

func TestTakeWhatIfReplacement_SubstitutesAndClears(t *testing.T) {
	rt := newTestRuntime(config.Default())
	rt.whatIfExecuteStep = "NAVIGATE to https://example.com"

	original := dsl.Command{Type: dsl.CmdClick, Raw: "CLICK the 'Login' button"}
	got, ok := rt.takeWhatIfReplacement(errWhatIfReplace, original)
	if !ok {
		t.Fatal("the signal should be handled")
	}
	if got.Type != dsl.CmdNavigate {
		t.Errorf("got command type %v, want NAVIGATE", got.Type)
	}
	if rt.whatIfExecuteStep != "" {
		t.Error("the pending step must be cleared so it cannot be replayed")
	}
}

// A typo in the REPL must not abort the run: the original step is kept.
func TestTakeWhatIfReplacement_UnparseableKeepsOriginal(t *testing.T) {
	rt := newTestRuntime(config.Default())
	rt.whatIfExecuteStep = "@@@ not a step @@@"

	original := dsl.Command{Type: dsl.CmdClick, Raw: "CLICK the 'Login' button"}
	got, ok := rt.takeWhatIfReplacement(errWhatIfReplace, original)
	if !ok {
		t.Fatal("the signal should still be handled")
	}
	if got.Raw != original.Raw {
		t.Errorf("got %q, want the original step back", got.Raw)
	}
	if rt.whatIfExecuteStep != "" {
		t.Error("the pending step must be cleared even when it does not parse")
	}
}

func TestTakeWhatIfReplacement_ResolvesVariables(t *testing.T) {
	rt := newTestRuntime(config.Default())
	rt.vars.Set("site", "https://example.com", LevelMission)
	rt.whatIfExecuteStep = "NAVIGATE to {site}"

	original := dsl.Command{Type: dsl.CmdClick, Raw: "CLICK the 'Login' button"}
	got, ok := rt.takeWhatIfReplacement(errWhatIfReplace, original)
	if !ok {
		t.Fatal("the signal should be handled")
	}
	if strings.Contains(got.Raw, "{site}") {
		t.Errorf("variables should be resolved before parsing, got %q", got.Raw)
	}
}
