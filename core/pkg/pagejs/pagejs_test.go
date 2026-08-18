package pagejs

import (
	"strings"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/core"
)

// Every element builder resolves the same way: the engine's registry first,
// the XPath second. Both backends depend on that, and on the id and XPath
// actually reaching the page.
func TestBuildersEmbedTheLocator(t *testing.T) {
	const xpath = `//button[@id="go"]`
	builders := map[string]string{
		"Focus":          Focus(7, xpath),
		"SetInputValue":  SetInputValue(7, xpath, "x"),
		"SetChecked":     SetChecked(7, xpath, true),
		"ScrollIntoView": ScrollIntoView(7, xpath),
		"FileInput":      FileInput(7, xpath),
		"Highlight":      Highlight(7, xpath, 500),
		"ElementCenter":  ElementCenter(7, xpath),
	}
	for name, js := range builders {
		if !strings.Contains(js, "window.__manulReg[7]") {
			t.Errorf("%s: does not read the element registry: %s", name, js)
		}
		if !strings.Contains(js, `document.evaluate("//button[@id=\"go\"]"`) {
			t.Errorf("%s: XPath missing or unescaped: %s", name, js)
		}
	}
}

// Values reach the page as JS string literals. A value carrying a quote or a
// newline must not be able to end the literal and run as code — %q is what
// prevents that, and this is the test that keeps it there.
func TestSetInputValueEscapesTheValue(t *testing.T) {
	js := SetInputValue(1, "//input", "it's \"quoted\"\n; alert(1)")
	if strings.Contains(js, "; alert(1)\n") {
		t.Fatalf("value broke out of its literal: %s", js)
	}
	for _, want := range []string{`it's`, `\"quoted\"`, `\n; alert(1)`} {
		if !strings.Contains(js, want) {
			t.Fatalf("escaped value missing %q: %s", want, js)
		}
	}
}

func TestSetCheckedCarriesTheDesiredState(t *testing.T) {
	on := SetChecked(1, "//input", true)
	off := SetChecked(1, "//input", false)
	if !strings.Contains(on, "el.checked !== true") {
		t.Fatalf("checked=true not applied: %s", on)
	}
	if !strings.Contains(off, "el.checked !== false") {
		t.Fatalf("checked=false not applied: %s", off)
	}
}

func TestScrollPage(t *testing.T) {
	if got := ScrollPage("down", ""); got != "window.scrollBy(0, 500);" {
		t.Fatalf("plain scroll down = %q", got)
	}
	if got := ScrollPage("up", ""); got != "window.scrollBy(0, -500);" {
		t.Fatalf("plain scroll up = %q", got)
	}

	// The generic-list strategy is a named container, not a locator.
	list := ScrollPage("down", string(core.ScrollStrategyGenericList))
	if !strings.Contains(list, `[role="listbox"]`) {
		t.Fatalf("generic list strategy lost its selectors: %s", list)
	}

	// A CSS container resolves with querySelector, an XPath one with evaluate.
	css := ScrollPage("down", ".results")
	if !strings.Contains(css, `document.querySelector(".results")`) {
		t.Fatalf("CSS container not resolved: %s", css)
	}
	xp := ScrollPage("down", "//div[@id='results']")
	if !strings.Contains(xp, "document.evaluate(") {
		t.Fatalf("XPath container not resolved: %s", xp)
	}
	// An explicit xpath= prefix is stripped before use.
	prefixed := ScrollPage("down", "xpath=//div[@id='results']")
	if strings.Contains(prefixed, "xpath=") {
		t.Fatalf("xpath= prefix leaked into the page: %s", prefixed)
	}
}

// The coordinate probes return their result as a JSON string, which is what
// both backends parse. Dropping the JSON.stringify would hand back an object
// and break the parse on every backend at once.
func TestCoordinateProbesReturnJSONStrings(t *testing.T) {
	if !strings.Contains(ElementCenter(1, "//div"), "JSON.stringify({x:") {
		t.Fatal("ElementCenter no longer stringifies its result")
	}
	drag := DragCenters(1, "//a", 2, "//b")
	if !strings.Contains(drag, "JSON.stringify({") {
		t.Fatal("DragCenters no longer stringifies its result")
	}
	// One scroll, both measurements — measuring twice is what made drags land
	// on stale coordinates. (Counting calls, not the word: the comment above
	// the call explains why it is there.)
	if n := strings.Count(drag, "scrollIntoView({"); n != 1 {
		t.Fatalf("DragCenters must scroll exactly once, found %d: %s", n, drag)
	}
}

// SelectOption is the one builder whose completion value the engine acts on:
// a select that matched nothing has to be distinguishable from one that did.
func TestSelectOptionReportsWhatItDid(t *testing.T) {
	js := SelectOption("//select[@id='sort']", "Від дешевих до дорогих")

	for _, want := range []string{
		"matched: false", // the miss path exists at all
		"matched: true",
		"options:",             // and carries the labels that do exist
		"toLowerCase",          // compared case-folded
		`replace(/\s+/g, " ")`, // and whitespace-normalised
		"getOwnPropertyDescriptor",
		`new Event("input"`,
		`new Event("change"`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("select script is missing %q:\n%s", want, js)
		}
	}
}

// Both the locator and the label are embedded as literals, so neither a quote
// in an XPath nor a backslash in a label can end the string early.
func TestSelectOptionQuotesItsArguments(t *testing.T) {
	js := SelectOption(`//select[@aria-label="Сортування"]`, `a" \ b`)
	if !strings.Contains(js, `\"Сортування\"`) {
		t.Errorf("xpath quotes not escaped:\n%s", js)
	}
	if !strings.Contains(js, `"a\" \\ b"`) {
		t.Errorf("label not escaped as a literal:\n%s", js)
	}
}

func TestHighlightCarriesItsDuration(t *testing.T) {
	if !strings.Contains(Highlight(1, "//div", 750), "}, 750);") {
		t.Fatal("highlight duration lost")
	}
}
