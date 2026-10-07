package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/alexbeatnik/manul-browser/core/pkg/config"
	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/explain"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

// Each test here pins a behaviour that was wrong and looked right: the step
// passed, or failed for a reason that pointed somewhere else.

func regEl(id int, tag, text string) dom.ElementSnapshot {
	return dom.ElementSnapshot{
		ID:          id,
		Tag:         tag,
		XPath:       fmt.Sprintf("/html/body/%s[%d]", tag, id),
		VisibleText: text,
		IsVisible:   true,
		Rect:        dom.Rect{Top: float64(id * 40), Left: 10, Width: 100, Height: 30, Bottom: float64(id*40 + 30), Right: 110},
	}
}

func regConfig() config.Config {
	cfg := config.Default()
	cfg.DefaultTimeout = 300 * time.Millisecond
	return cfg
}

// runSource parses src and runs it as a hunt against page.
func runSource(t *testing.T, page *MockPage, src string) (*explain.HuntResult, error) {
	t.Helper()
	hunt, err := dsl.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))
	return rt.RunHunt(context.Background(), hunt)
}

// ── State is readable on a disabled element ──────────────────────────────────

// The scorer zeroes a disabled element, so the one element `is disabled` asks
// about could never be found, and the step failed as "not found".
func TestVerify_DisabledElementIsFound(t *testing.T) {
	button := regEl(1, "button", "Submit")
	button.IsDisabled = true
	page := &MockPage{Elements: []dom.ElementSnapshot{button, regEl(2, "p", "hello")}}

	if _, err := runSource(t, page, "VERIFY that 'Submit' is disabled\n"); err != nil {
		t.Errorf("is disabled: %v", err)
	}

	_, err := runSource(t, page, "VERIFY that 'Submit' is enabled\n")
	if err == nil || !strings.Contains(err.Error(), "actual state disabled") {
		t.Errorf("is enabled: want a state mismatch, got %v", err)
	}
}

func TestVerify_DisabledCheckboxStateIsReadable(t *testing.T) {
	box := regEl(1, "input", "")
	box.InputType = "checkbox"
	box.LabelText = "Terms"
	box.IsChecked = true
	box.IsDisabled = true
	page := &MockPage{Elements: []dom.ElementSnapshot{box}}

	if _, err := runSource(t, page, "VERIFY that 'Terms' is checked\n"); err != nil {
		t.Errorf("a read-only checkbox should still report its state: %v", err)
	}
}

func TestWaitFor_SeesDisabledElement(t *testing.T) {
	button := regEl(1, "button", "Submit")
	button.IsDisabled = true
	page := &MockPage{Elements: []dom.ElementSnapshot{button}}
	rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))

	for _, state := range []string{"disabled", "visible"} {
		ok, err := rt.elementStateSatisfied(context.Background(), dsl.Command{}, "Submit", state)
		if err != nil || !ok {
			t.Errorf("state %q: satisfied=%v err=%v", state, ok, err)
		}
	}
	if ok, _ := rt.elementStateSatisfied(context.Background(), dsl.Command{}, "Submit", "enabled"); ok {
		t.Error("a disabled button reported as enabled")
	}
}

// ── A target that is not there ───────────────────────────────────────────────

// A type hint and the right tag clear the confidence bar on their own, so each
// of these used to act on whichever element of the right kind came first.
func TestAction_TargetWithNoMatchOnThePageIsNotFound(t *testing.T) {
	email := regEl(3, "input", "")
	email.InputType = "text"
	email.Placeholder = "Email"
	terms := regEl(4, "input", "")
	terms.InputType = "checkbox"
	terms.LabelText = "Accept terms"
	page := &MockPage{Elements: []dom.ElementSnapshot{
		regEl(1, "button", "Pay now"), regEl(2, "button", "Cancel order"), email, terms,
	}}

	for _, step := range []string{
		"CLICK the 'Delete account' button",
		"CLICK 'Delete account'",
		"DOUBLE CLICK the 'Delete account' button",
		"HOVER over the 'Delete account' button",
		"FILL 'Phone number' field with '123'",
		"CHECK the checkbox for 'Newsletter'",
	} {
		res, err := runSource(t, page, step+"\n")
		if err == nil {
			t.Errorf("%q acted on a page that has no such target", step)
			continue
		}
		if got := res.Results[0].FailureReason; got != explain.ReasonNotFound {
			t.Errorf("%q: failure reason %q, want not_found (%v)", step, got, err)
		}
	}
	if len(page.Clicks) != 0 || len(page.Inputs) != 0 || terms.IsChecked {
		t.Errorf("the page was touched: clicks=%d inputs=%v", len(page.Clicks), page.Inputs)
	}

	// What is on the page still resolves, exactly and loosely.
	for _, step := range []string{
		"CLICK the 'Pay now' button",
		"CLICK the 'Cancel' button",
		"FILL 'Email' field with 'a@b.c'",
		"CHECK the checkbox for 'Accept terms'",
	} {
		if _, err := runSource(t, page, step+"\n"); err != nil {
			t.Errorf("%q: %v", step, err)
		}
	}
}

// The target is on the page — as a caption beside an unlabelled field. Pass 1
// used to settle for the first field in the document on the strength of its
// tag, before the caption was ever looked at.
func TestFill_CaptionBesideAFieldBeatsAnUnrelatedField(t *testing.T) {
	search := regEl(1, "input", "")
	search.InputType = "text"
	search.Placeholder = "Search"
	caption := regEl(2, "div", "Phone number")
	phone := regEl(3, "input", "")
	phone.InputType = "text"
	page := &MockPage{Elements: []dom.ElementSnapshot{search, caption, phone}}

	if _, err := runSource(t, page, "FILL 'Phone number' field with '123'\n"); err != nil {
		t.Fatal(err)
	}
	if _, wrong := page.Inputs[search.XPath]; wrong {
		t.Errorf("typed into the search box: %v", page.Inputs)
	}
}

func TestSelect_CustomDropdownMissingOptionFails(t *testing.T) {
	combo := regEl(1, "div", "Colour")
	combo.Role = "combobox"
	page := &MockPage{Elements: []dom.ElementSnapshot{
		combo, regEl(2, "li", "Red"), regEl(3, "li", "Green"), regEl(4, "button", "Buy"),
	}}

	_, err := runSource(t, page, "SELECT 'Purple' from the 'Colour' dropdown\n")
	if err == nil || !strings.Contains(err.Error(), "could not find option") {
		t.Fatalf("want a missing-option failure, got %v", err)
	}
	// One click opens the list. A second would be the bystander it used to pick.
	if len(page.Clicks) != 1 {
		t.Errorf("got %d clicks, want 1", len(page.Clicks))
	}

	page.Clicks = nil
	if _, err := runSource(t, page, "SELECT 'Green' from the 'Colour' dropdown\n"); err != nil {
		t.Errorf("an option that exists: %v", err)
	}
}

func TestDrag_MissingSourceOrDestinationFails(t *testing.T) {
	page := &MockPage{Elements: []dom.ElementSnapshot{regEl(1, "div", "Card A"), regEl(2, "div", "Column B")}}

	if _, err := runSource(t, page, "DRAG 'Card A' and drop it into 'Column B'\n"); err != nil {
		t.Fatalf("both present: %v", err)
	}
	_, err := runSource(t, page, "DRAG 'Sprocket' and drop it into 'Column B'\n")
	if err == nil || !strings.Contains(err.Error(), "drag source not found") {
		t.Errorf("missing source: %v", err)
	}
	_, err = runSource(t, page, "DRAG 'Card A' and drop it into 'Nowhere'\n")
	if err == nil || !strings.Contains(err.Error(), "drag destination not found") {
		t.Errorf("missing destination: %v", err)
	}
}

func TestHighlight_UnresolvableTargetFails(t *testing.T) {
	page := &MockPage{Elements: []dom.ElementSnapshot{regEl(1, "button", "Save")}}
	if _, err := runSource(t, page, "HIGHLIGHT the 'Save' button\n"); err != nil {
		t.Errorf("present: %v", err)
	}
	if _, err := runSource(t, page, "HIGHLIGHT the 'Qwertyuiop'\n"); err == nil {
		t.Error("a target nothing matches was highlighted")
	}
}

func TestVerifyField_UnknownLabelIsNotFound(t *testing.T) {
	field := regEl(1, "input", "")
	field.InputType = "text"
	field.LabelText = "Email"
	page := &MockPage{Elements: []dom.ElementSnapshot{field}}

	// The only element has an empty value, which used to satisfy this.
	_, err := runSource(t, page, "VERIFY 'Postcode' field has value ''\n")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("want not found, got %v", err)
	}
}

// ── VERIFY ───────────────────────────────────────────────────────────────────

type brokenProbePage struct{ *MockPage }

func (p brokenProbePage) CallProbe(context.Context, string, any) ([]byte, error) {
	return nil, errors.New("execution context destroyed")
}

// With the probe failing, "not present" was never observed — it was assumed.
func TestVerify_NegatedDoesNotPassOnAProbeThatNeverAnswered(t *testing.T) {
	rt := New(regConfig(), brokenProbePage{&MockPage{}}, utils.NewLoggerTo(nopWriter{}, nil))
	hunt, _ := dsl.Parse(strings.NewReader("VERIFY that 'Error' is NOT present\n"))
	if _, err := rt.RunHunt(context.Background(), hunt); err == nil {
		t.Fatal("a VERIFY that could not read the page passed")
	}
}

func TestVerifySoftly_FailureIsRecordedAsSoftError(t *testing.T) {
	page := &MockPage{Elements: []dom.ElementSnapshot{regEl(1, "p", "hello")}}
	res, err := runSource(t, page, "VERIFY SOFTLY that 'Zzyzx banner' is present\nPRINT 'still running'\n")
	if err != nil || !res.Success {
		t.Fatalf("a soft failure must not fail the hunt: err=%v", err)
	}
	if len(res.SoftErrors) != 1 || !strings.Contains(res.SoftErrors[0], "Zzyzx banner") {
		t.Errorf("soft errors = %v", res.SoftErrors)
	}
}

// ── Conditions ───────────────────────────────────────────────────────────────

// The documented spelling is `is NOT present`; only the lower-case one worked.
func TestCondition_KeywordsAreCaseInsensitive(t *testing.T) {
	page := &MockPage{Elements: []dom.ElementSnapshot{regEl(1, "h1", "Dashboard")}}
	rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))

	for cond, want := range map[string]bool{
		"'Dashboard' is present":         true,
		"'Dashboard' IS PRESENT":         true,
		"'Dashboard' is NOT present":     false,
		"'Zzyzx' is NOT present":         true,
		"button 'Zzyzx' NOT EXISTS":      true,
		"TRUE":                           true,
		"'Is Present Banner' is present": false,
	} {
		got, err := rt.evaluateCondition(context.Background(), cond)
		if err != nil || got != want {
			t.Errorf("%q: got %v (err %v), want %v", cond, got, err, want)
		}
	}
}

// ── PRESS ────────────────────────────────────────────────────────────────────

type keyPage struct {
	*MockPage
	keys    []string
	focused []string
}

func (p *keyPage) DispatchKey(_ context.Context, key string, modifiers int) error {
	p.keys = append(p.keys, fmt.Sprintf("%s/%d", key, modifiers))
	return nil
}

func (p *keyPage) Focus(_ context.Context, _ int, xpath string) error {
	p.focused = append(p.focused, xpath)
	return nil
}

func TestSplitKeyChord(t *testing.T) {
	for chord, want := range map[string]string{
		"Enter":           "Enter/0",
		"Control+A":       "a/2",
		"ctrl+shift+P":    "p/10",
		"Shift+Tab":       "Tab/8",
		"Shift+A":         "A/8",
		"Alt+F4":          "F4/1",
		"Meta+K":          "k/4",
		"+":               "+/0",
		"Banana+A":        "Banana+A/0",
		"Control+":        "Control+/0",
		"Control + Enter": "Enter/2",
	} {
		key, mods := splitKeyChord(chord)
		if got := fmt.Sprintf("%s/%d", key, mods); got != want {
			t.Errorf("%q: got %s, want %s", chord, got, want)
		}
	}
}

func TestPress_ChordAndTarget(t *testing.T) {
	field := regEl(1, "input", "")
	field.InputType = "text"
	field.LabelText = "Username"
	page := &keyPage{MockPage: &MockPage{Elements: []dom.ElementSnapshot{field, regEl(2, "button", "Go")}}}
	rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))

	hunt, _ := dsl.Parse(strings.NewReader("PRESS Control+A\nPRESS Tab ON 'Username'\n"))
	if _, err := rt.RunHunt(context.Background(), hunt); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(page.keys, " "); got != "a/2 Tab/0" {
		t.Errorf("keys = %q", got)
	}
	if len(page.focused) != 1 || page.focused[0] != field.XPath {
		t.Errorf("focused = %v, want the Username field", page.focused)
	}

	hunt, _ = dsl.Parse(strings.NewReader("PRESS Enter ON 'Zzyzx'\n"))
	if _, err := rt.RunHunt(context.Background(), hunt); err == nil {
		t.Error("PRESS ON a target that is not there should fail, not press anyway")
	}
}

// ── Variables ────────────────────────────────────────────────────────────────

func TestInterpolate_BareNameEndsAtAWordBoundary(t *testing.T) {
	sv := NewScopedVariables()
	sv.Set("i", "0", LevelRow)
	sv.Set("user", "ann", LevelRow)

	for in, want := range map[string]string{
		"row $i of $items":   "row 0 of $items",
		"$user/$username":    "ann/$username",
		"{user} ${user} $i.": "ann ann 0.",
		"cost: $5":           "cost: $5",
	} {
		if got := sv.Interpolate(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestRepeat_NamedLoopVariable(t *testing.T) {
	res, err := runSource(t, &MockPage{}, "REPEAT 2 TIMES as {n}:\n    PRINT 'n={n}'\n")
	if err != nil {
		t.Fatal(err)
	}
	var printed []string
	for _, r := range res.Results {
		if r.CommandType == string(dsl.CmdPrint) {
			printed = append(printed, r.ActionValue)
		}
	}
	if strings.Join(printed, ",") != "n=0,n=1" {
		t.Errorf("printed %v", printed)
	}
}

// ── Reporting ────────────────────────────────────────────────────────────────

// Every step used to report index 0, so the HTML report numbered them all [1].
func TestResults_CarryStepIndexAndBlock(t *testing.T) {
	res, err := runSource(t, &MockPage{}, "STEP 1: One\n    PRINT 'a'\n    PRINT 'b'\nSTEP 2: Two\n    PRINT 'c'\n")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range res.Results {
		got = append(got, fmt.Sprintf("%s#%d", r.StepBlock, r.StepIndex))
	}
	if want := "STEP 1: One#0,STEP 1: One#1,STEP 2: Two#0"; strings.Join(got, ",") != want {
		t.Errorf("got %v", got)
	}
}

// A variable holding a '%' is data. As a format string it printed %!d(MISSING).
func TestDebugVars_PercentSurvives(t *testing.T) {
	var out bytes.Buffer
	rt := New(regConfig(), &MockPage{}, utils.NewLoggerTo(&out, nil))
	hunt, _ := dsl.Parse(strings.NewReader("SET {q} = a%20b%d\nDEBUG VARS\n"))
	if _, err := rt.RunHunt(context.Background(), hunt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "q = a%20b%d") || strings.Contains(out.String(), "MISSING") {
		t.Errorf("log mangled the value:\n%s", out.String())
	}
}

func TestMdCell_TruncatesOnACharacterBoundary(t *testing.T) {
	cell := mdCell(strings.Repeat("ї", 80))
	if !utf8.ValidString(cell) {
		t.Errorf("truncation split a character: %q", cell)
	}
	if n := utf8.RuneCountInString(cell); n != 60 {
		t.Errorf("cell is %d characters, want 60", n)
	}
}

// ── Debugger ─────────────────────────────────────────────────────────────────

// The modal sets 'ABORT'; EvalJS returns it as bare bytes. The old check
// decoded it as JSON, which never succeeds on a bare word.
func TestDebugAbortRequested(t *testing.T) {
	for raw, want := range map[string]bool{
		"ABORT":   true,
		`"ABORT"`: true,
		"abort":   true,
		"":        false,
		"null":    false,
	} {
		if got := debugAbortRequested([]byte(raw)); got != want {
			t.Errorf("%q: got %v, want %v", raw, got, want)
		}
	}
}
