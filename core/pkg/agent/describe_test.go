package agent

import (
	"strings"
	"testing"
)

func TestDescribeForLLM_Success(t *testing.T) {
	o := StepOutcome{OK: true, Step: "CLICK 'Login'", Reason: ReasonOK, URL: "https://x/account"}
	out := o.DescribeForLLM()
	if !strings.HasPrefix(out, "OK: CLICK 'Login'") {
		t.Errorf("success line wrong: %q", out)
	}
	if !strings.Contains(out, "https://x/account") {
		t.Errorf("resulting URL missing: %q", out)
	}
	if strings.Contains(out, "WARNING") {
		t.Errorf("confident success must not warn: %q", out)
	}
}

func TestDescribeForLLM_WeakSuccessWarns(t *testing.T) {
	o := StepOutcome{
		OK: true, Step: "CLICK 'More information'", Reason: ReasonOK, Score: 0.26,
		Near: []Cand{{Text: "Learn more", Score: 0.26}},
	}
	out := o.DescribeForLLM()
	if !strings.Contains(out, "WARNING") || !strings.Contains(out, "WRONG element") {
		t.Errorf("weak match must warn: %q", out)
	}
	if !strings.Contains(out, "'Learn more' (0.26)") {
		t.Errorf("near candidates must be quoted with scores: %q", out)
	}
}

func TestDescribeForLLM_AmbiguousFailureExplains(t *testing.T) {
	o := StepOutcome{
		OK: false, Step: "CLICK 'перше відео'", Reason: ReasonAmbiguous,
		Error: "target resolution too ambiguous (confidence 0.033, runner-up 0.033)",
		Near:  []Cand{{Text: "Хімера — Love Will Find You", Score: 0.18}},
	}
	out := o.DescribeForLLM()
	if !strings.HasPrefix(out, "FAILED: CLICK 'перше відео'") {
		t.Errorf("failure must echo the step: %q", out)
	}
	if !strings.Contains(out, "describes an element") || !strings.Contains(out, "exact label") || !strings.Contains(out, "nothing was done") {
		t.Errorf("ambiguous advice missing: %q", out)
	}
	// 'All' on a page without it was a precise label, only absent.
	if strings.Contains(out, "too vague") {
		t.Errorf("a target that is not there is not too vague: %q", out)
	}
	if !strings.Contains(out, "'Хімера — Love Will Find You' (0.18)") {
		t.Errorf("real candidates must be listed: %q", out)
	}
}

func TestDescribeForLLM_NotFound(t *testing.T) {
	o := StepOutcome{OK: false, Step: "CLICK 'Nonexistent'", Reason: ReasonNotFound, URL: "https://x/cart"}
	out := o.DescribeForLLM()
	if !strings.Contains(out, "call map again") || !strings.Contains(out, "open that first") || !strings.Contains(out, "fail the same way") {
		t.Errorf("not-found advice missing: %q", out)
	}
	// On Comfy the label came from map and the page changed it; the model was
	// told it had made the label up.
	if strings.Contains(out, "made up") {
		t.Errorf("a label from map is not made up: %q", out)
	}
	if !strings.Contains(out, "The page open now is https://x/cart.") {
		t.Errorf("the page it is on is missing: %q", out)
	}
}

func TestRunOutcomeRenderForLLM(t *testing.T) {
	r := RunOutcome{
		OK: false, URL: "https://x/results", TotalSteps: 3, Passed: 2, Failed: 1,
		Results: []StepOutcome{
			{OK: true, Step: "NAVIGATE to https://x", Reason: ReasonOK},
			{OK: true, Step: "FILL 'Search' field with 'cats'", Reason: ReasonOK},
			{OK: false, Step: "CLICK 'first result'", Reason: ReasonAmbiguous},
		},
	}
	out := r.RenderForLLM()
	if !strings.HasPrefix(out, "FAILED: ran 3 step(s) — 2 passed, 1 failed.") {
		t.Errorf("verdict line wrong: %q", out)
	}
	if !strings.Contains(out, "https://x/results") {
		t.Errorf("final URL missing: %q", out)
	}
	if !strings.Contains(out, "STOPPED at the first failed step") {
		t.Errorf("stop-explanation missing: %q", out)
	}
	if !strings.Contains(out, "OK: NAVIGATE to https://x") ||
		!strings.Contains(out, "FAILED: CLICK 'first result'") {
		t.Errorf("per-step lines missing: %q", out)
	}
}

func TestDescribePageChange(t *testing.T) {
	same := PageState{Title: "Results", URL: "https://x/results"}
	out := DescribePageChange(same, same)
	if !strings.Contains(out, "did NOT change") || !strings.Contains(out, "NO effect") {
		t.Errorf("no-change case must be explicit: %q", out)
	}

	out = DescribePageChange(same, PageState{Title: "Video", URL: "https://x/watch"})
	if !strings.Contains(out, "CHANGED") ||
		!strings.Contains(out, "https://x/results") || !strings.Contains(out, "https://x/watch") {
		t.Errorf("change case must show before/after: %q", out)
	}
}

// A model is told the page's name in pages/ when it has one, with the
// address after it, and the address alone otherwise.
func TestDescribeForLLM_NamesThePage(t *testing.T) {
	ok := StepOutcome{OK: true, Step: "CLICK 'Sign in'", URL: "https://x/account", Page: "Account"}
	if out := ok.DescribeForLLM(); !strings.Contains(out, "the page is now Account (https://x/account)") {
		t.Errorf("success: %q", out)
	}
	miss := StepOutcome{OK: false, Step: "CLICK 'Pay'", Reason: ReasonNotFound, URL: "https://x/cart", Page: "Cart"}
	if out := miss.DescribeForLLM(); !strings.Contains(out, "The page open now is Cart (https://x/cart).") {
		t.Errorf("failure: %q", out)
	}
	pm := PageMap{URL: "https://x/cart", Page: "Cart", Groups: []MapGroup{{Name: "Main", Elements: []MapElement{{Label: "Pay", Role: "button"}}}}}
	if out := pm.RenderForLLM(0); !strings.Contains(out, "Current page: Cart (https://x/cart)") {
		t.Errorf("map: %q", out)
	}
	bare := StepOutcome{OK: true, Step: "CLICK 'Sign in'", URL: "https://x/account"}
	if out := bare.DescribeForLLM(); !strings.Contains(out, "the page is now https://x/account") || strings.Contains(out, "(") {
		t.Errorf("no name: %q", out)
	}
}
