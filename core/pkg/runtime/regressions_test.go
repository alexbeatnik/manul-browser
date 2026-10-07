package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// ── Reading a field back ─────────────────────────────────────────────────────

// fieldProbePage answers every probe with what the extraction probe sends for
// a form control.
type fieldProbePage struct {
	*MockPage
	answer string
}

func (p fieldProbePage) CallProbe(context.Context, string, any) ([]byte, error) {
	return []byte(p.answer), nil
}

// EXTRACT read text nodes only, so a field came back as its own label. Now the
// probe answers with the field, and an empty one is an answer — not the
// "not found or empty" that empty text is.
func TestExtract_FieldValueIsReadEvenWhenEmpty(t *testing.T) {
	for _, tc := range []struct{ answer, want string }{
		{`{"field":true,"value":"ada@example.com"}`, "ada@example.com"},
		{`{"field":true,"value":""}`, ""},
	} {
		rt := New(regConfig(), fieldProbePage{&MockPage{}, tc.answer}, utils.NewLoggerTo(nopWriter{}, nil))
		rt.vars.Set("e", "stale", LevelRow)
		res, err := rt.RunCommand(context.Background(), dsl.Command{Type: dsl.CmdExtract, Target: "Email", ExtractVar: "e"})
		if err != nil {
			t.Fatalf("%s: %v", tc.answer, err)
		}
		if got, _ := rt.vars.Resolve("e"); got != tc.want || res.ActionValue != tc.want {
			t.Errorf("%s: extracted %q into {e}=%q, want %q", tc.answer, res.ActionValue, got, tc.want)
		}
		if !ExtractedFromField(res) {
			t.Errorf("%s: not reported as a field", tc.answer)
		}
	}

	rt := New(regConfig(), fieldProbePage{&MockPage{}, ""}, utils.NewLoggerTo(nopWriter{}, nil))
	res, err := rt.RunCommand(context.Background(), dsl.Command{Type: dsl.CmdExtract, Target: "Email", ExtractVar: "e"})
	if err == nil || ExtractedFromField(res) {
		t.Errorf("empty text passed as found: err=%v", err)
	}
}

// The label's own text is the field's name, so it outranked the field — and a
// label has no value, so a filled textarea verified as "".
func TestVerifyField_ValueIsReadOffTheFieldNotItsLabel(t *testing.T) {
	notes := regEl(2, "textarea", "")
	notes.LabelText = "Notes"
	notes.Value = "line one"
	notes.Placeholder = "anything"
	// As the snapshot reports a wrapping <label>: its text is its own label.
	label := regEl(1, "label", "Notes")
	label.LabelText = "Notes"
	page := &MockPage{Elements: []dom.ElementSnapshot{label, notes}}

	if _, err := runSource(t, page, "VERIFY 'Notes' field has value 'line one'\n"); err != nil {
		t.Errorf("value: %v", err)
	}
	if _, err := runSource(t, page, "VERIFY 'Notes' field has placeholder 'anything'\n"); err != nil {
		t.Errorf("placeholder: %v", err)
	}
}

func TestVerifyField_DisabledFieldStillHasAValue(t *testing.T) {
	field := regEl(1, "input", "")
	field.InputType = "text"
	field.LabelText = "Plan"
	field.Value = "Free"
	field.IsDisabled = true
	page := &MockPage{Elements: []dom.ElementSnapshot{field, regEl(2, "p", "hello")}}

	if _, err := runSource(t, page, "VERIFY 'Plan' field has value 'Free'\n"); err != nil {
		t.Error(err)
	}
}

// ── FOR EACH ─────────────────────────────────────────────────────────────────

// @var: values are substituted at parse time, so the loop was handed the list
// where it expected a variable name, found no such variable, and ran its body
// zero times without a word.
func TestForEach_CollectionDeclaredWithVar(t *testing.T) {
	res, err := runSource(t, &MockPage{}, "@var: {names} = Ada, Grace\nFOR EACH {n} IN {names}:\n    PRINT 'n={n}'\n")
	if err != nil {
		t.Fatal(err)
	}
	var printed []string
	for _, r := range res.Results {
		if r.CommandType == string(dsl.CmdPrint) {
			printed = append(printed, r.ActionValue)
		}
	}
	if strings.Join(printed, ",") != "n=Ada,n=Grace" {
		t.Errorf("printed %v", printed)
	}
}

// ── WAIT FOR ─────────────────────────────────────────────────────────────────

// One shared word was enough to keep a target "present": the scorer ranks by
// word overlap, and 'A checkbox' overlaps any sentence about checkboxes.
func TestWaitFor_GoneWhenOnlyAWordOfItRemains(t *testing.T) {
	page := &MockPage{Elements: []dom.ElementSnapshot{
		regEl(1, "p", "Elements (e.g., checkbox, input field) are changed asynchronously."),
	}}
	if _, err := runSource(t, page, "WAIT FOR 'A checkbox' to disappear\n"); err != nil {
		t.Errorf("still waiting for something that is gone: %v", err)
	}

	page.Elements = append(page.Elements, regEl(2, "div", "A checkbox"))
	if _, err := runSource(t, page, "WAIT FOR 'A checkbox' to be visible\n"); err != nil {
		t.Errorf("present: %v", err)
	}
}

// ── CHECK ────────────────────────────────────────────────────────────────────

// captionedBoxes is a list of rows like TodoMVC's: a checkbox with no name of
// its own, and beside it a <label> that is not bound to it.
func captionedBoxes() *MockPage {
	box := func(id int) dom.ElementSnapshot {
		el := regEl(id, "input", "")
		el.InputType = "checkbox"
		return el
	}
	return &MockPage{Elements: []dom.ElementSnapshot{
		box(1), regEl(2, "label", "call Ada"),
		box(3), regEl(4, "label", "call Grace"),
	}}
}

// The box is reached through its caption but cannot be found by it, so the
// check by name said it had never been ticked — and went on to try others.
// The page is asked about the control that was acted on instead.
func TestCheck_StateIsReadOffTheControlThatWasActedOn(t *testing.T) {
	page := captionedBoxes()
	// The page says the caption's own box is ticked; the snapshot, which can
	// only look boxes up by name, still shows every box clear.
	page.EvalResult = func(expr string) ([]byte, bool) {
		if strings.Contains(expr, "manulCheckable") && strings.Contains(expr, "found: true") {
			return []byte(`{"found":true,"checked":true}`), true
		}
		return nil, false
	}
	rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))

	caption := page.Elements[3]
	if err := rt.ensureCheckboxTargetState(context.Background(), caption, "call Grace", true, nil); err != nil {
		t.Fatal(err)
	}
	for _, el := range page.Elements {
		if el.IsChecked {
			t.Errorf("element %d was ticked by a retry the step had no need of", el.ID)
		}
	}
}

// The retry's plain rankings always return five of something. With the
// checkbox hint that was the page's first five checkboxes, whatever they
// belonged to, and each of them was ticked in turn.
func TestCheck_RetryCandidatesCarryTheTarget(t *testing.T) {
	named := func(id int, label string) dom.ElementSnapshot {
		el := regEl(id, "input", "")
		el.InputType = "checkbox"
		el.LabelText = label
		return el
	}
	page := &MockPage{Elements: []dom.ElementSnapshot{
		named(1, "Newsletter"), named(2, "Terms"), named(3, "Remember me"),
	}}
	rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))

	candidates, err := rt.collectCheckboxRetryCandidates(context.Background(), "call Grace", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range candidates {
		t.Errorf("%q would be ticked on a retry for 'call Grace'", c.Element.LabelText)
	}
}

// ── CLICK ────────────────────────────────────────────────────────────────────

// A click is dispatched at coordinates and goes to whatever is there. Under an
// open date picker that was a day of the month, and the step passed.
func TestClick_CoveredTargetIsNotClicked(t *testing.T) {
	covered := true
	page := &MockPage{Elements: []dom.ElementSnapshot{regEl(1, "button", "Submit")}}
	page.EvalResult = func(expr string) ([]byte, bool) {
		if !strings.Contains(expr, "elementFromPoint") {
			return nil, false
		}
		if covered {
			return []byte(`{"covered":true,"by":"<td.day>"}`), true
		}
		return []byte(`{"covered":false}`), true
	}

	for _, verb := range []string{"CLICK", "DOUBLE CLICK", "RIGHT CLICK"} {
		_, err := runSource(t, page, verb+" the 'Submit' button\n")
		if err == nil || !strings.Contains(err.Error(), "covered by <td.day>") {
			t.Errorf("%s: want a covered-target failure, got %v", verb, err)
		}
	}
	if len(page.Clicks) != 0 {
		t.Errorf("clicked anyway: %v", page.Clicks)
	}

	covered = false
	if _, err := runSource(t, page, "CLICK the 'Submit' button\n"); err != nil {
		t.Fatal(err)
	}
	if len(page.Clicks) != 1 {
		t.Errorf("clicks = %v, want one", page.Clicks)
	}
}

// ── MOCK ─────────────────────────────────────────────────────────────────────

// MOCK was a patch over window.fetch in the current document. It is a rule
// handed to the page, which answers at the network.
func TestMock_HandsTheRuleToThePage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(file, []byte(`{"users":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	page := &MockPage{}
	if _, err := runSource(t, page, "MOCK get \"/api/users*\" with '"+file+"'\n"); err != nil {
		t.Fatal(err)
	}
	if len(page.Mocks) != 1 {
		t.Fatalf("rules = %v", page.Mocks)
	}
	rule := page.Mocks[0]
	if rule.Method != "GET" || rule.Pattern != "/api/users*" || string(rule.Body) != `{"users":[]}` || rule.ContentType != "application/json" {
		t.Errorf("rule = %+v", rule)
	}
	for _, expr := range page.EvalCalls {
		if strings.Contains(expr, "fetch") {
			t.Errorf("still patching fetch in the page: %s", expr)
		}
	}
}

// ── NAVIGATE ─────────────────────────────────────────────────────────────────

// stalledPage never finishes loading, and remembers how long it was asked to
// wait for a response.
type stalledPage struct {
	*MockPage
	responseTimeout time.Duration
}

func (p *stalledPage) WaitForLoad(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (p *stalledPage) WaitForResponse(_ context.Context, _ string, timeout time.Duration) error {
	p.responseTimeout = timeout
	return nil
}

// nav_timeout was declared, documented, and read by nothing: a page with one
// resource that never arrives held NAVIGATE for as long as its server liked.
func TestNavigate_GivesUpAfterNavTimeout(t *testing.T) {
	for _, tc := range []struct {
		readyState string
		wantErr    bool
	}{
		{"loading", true},      // not a page yet
		{"interactive", false}, // parsed and usable; only a straggler is missing
	} {
		page := &stalledPage{MockPage: &MockPage{}}
		page.EvalResult = func(expr string) ([]byte, bool) {
			if expr == "document.readyState" {
				return []byte(tc.readyState), true
			}
			return nil, false
		}
		cfg := regConfig()
		cfg.NavTimeout = 50 * time.Millisecond
		rt := New(cfg, page, utils.NewLoggerTo(nopWriter{}, nil))

		done := make(chan error, 1)
		go func() {
			_, err := rt.RunCommand(context.Background(), dsl.Command{Type: dsl.CmdNavigate, URL: "https://slow.test/"})
			done <- err
		}()
		select {
		case err := <-done:
			if tc.wantErr && (err == nil || !strings.Contains(err.Error(), "navigation timeout")) {
				t.Errorf("%s: want a navigation timeout, got %v", tc.readyState, err)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("%s: a parsed document is a page to carry on with, got %v", tc.readyState, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: NAVIGATE is still waiting long after nav_timeout", tc.readyState)
		}
	}
}

// The config contract gives a response nav_timeout to arrive in; the step
// timeout, a sixth of it by default, was what it actually got.
func TestWaitForResponse_IsGivenNavTimeout(t *testing.T) {
	page := &stalledPage{MockPage: &MockPage{}}
	cfg := regConfig()
	cfg.NavTimeout = 7 * time.Second
	rt := New(cfg, page, utils.NewLoggerTo(nopWriter{}, nil))

	if _, err := rt.RunCommand(context.Background(), dsl.Command{Type: dsl.CmdWaitForResponse, WaitResponseURL: "/api/cart"}); err != nil {
		t.Fatal(err)
	}
	if page.responseTimeout != 7*time.Second {
		t.Errorf("waited %s for the response, want nav_timeout", page.responseTimeout)
	}
}
