package runtime

import (
	"strings"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
)

// pointerPage answers PointerReceived from a script, one answer per check.
func pointerPage(answers ...string) (*MockPage, *int) {
	checks := 0
	page := &MockPage{Elements: []dom.ElementSnapshot{regEl(1, "button", "Add to cart")}}
	page.EvalResult = func(expr string) ([]byte, bool) {
		if expr == pagejs.PointerReceived {
			a := "1"
			if checks < len(answers) {
				a = answers[checks]
			}
			checks++
			return []byte(a), true
		}
		return nil, false
	}
	return page, &checks
}

// In a browser the engine did not launch, the password manager's prompt after
// a sign-in took the clicks meant for the page, and CLICK passed with the
// cart still empty.
func TestPointer_AClickThePageNeverReceivedFailsTheStep(t *testing.T) {
	page, _ := pointerPage("0", "0")
	_, err := runSource(t, page, "CLICK the 'Add to cart' button\n")
	if err == nil || !strings.Contains(err.Error(), "did not reach the page") {
		t.Fatalf("want a swallowed-click failure, got %v", err)
	}
	if len(page.Clicks) != 2 {
		t.Errorf("clicked %d times, want 2: once, and once more after nothing arrived", len(page.Clicks))
	}
}

// The page saw nothing of the first click, so trying again cannot act twice;
// a prompt the first click dismissed lets the second through.
func TestPointer_ASecondClickThatArrivesPasses(t *testing.T) {
	page, _ := pointerPage("0", "1")
	if _, err := runSource(t, page, "CLICK the 'Add to cart' button\n"); err != nil {
		t.Fatal(err)
	}
	if len(page.Clicks) != 2 {
		t.Errorf("clicked %d times, want 2", len(page.Clicks))
	}
}

func TestPointer_AClickThatArrivesIsNotRepeated(t *testing.T) {
	for name, answer := range map[string]string{"received": "1", "navigated": "2"} {
		page, checks := pointerPage(answer)
		if _, err := runSource(t, page, "CLICK the 'Add to cart' button\n"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(page.Clicks) != 1 || *checks != 1 {
			t.Errorf("%s: %d clicks, %d checks; want 1 and 1", name, len(page.Clicks), *checks)
		}
	}
}

// A disabled control receives no mouse events by design: not a swallowed
// click.
func TestPointer_ADisabledControlIsNotChecked(t *testing.T) {
	page, checks := pointerPage("0", "0")
	page.Elements[0].IsDisabled = true
	_, _ = runSource(t, page, "CLICK the 'Add to cart' button\n")
	if *checks != 0 {
		t.Errorf("checked a disabled control %d times", *checks)
	}
}

// The quickstart's slow page drops a line a second after it loads. A click
// on 'Save settings' measured just before landed just after, where the button
// no longer was, and passed with nothing saved. Such a press is stopped in
// the page and made again where the element is now.
func TestPointer_APressThatWentToAnotherElementIsMadeAgain(t *testing.T) {
	page, _ := pointerPage("3", "1")
	if _, err := runSource(t, page, "CLICK the 'Add to cart' button\n"); err != nil {
		t.Fatal(err)
	}
	if len(page.Clicks) != 2 {
		t.Errorf("clicked %d times, want 2: the stopped press and the one where the button is now", len(page.Clicks))
	}
}

func TestPointer_AnElementThatKeepsMovingFailsTheStep(t *testing.T) {
	page, _ := pointerPage("3", "3", "3")
	_, err := runSource(t, page, "CLICK the 'Add to cart' button\n")
	if err == nil || !strings.Contains(err.Error(), "moves as it is clicked, and nothing was clicked") {
		t.Fatalf("want a moving-element failure, got %v", err)
	}
	if len(page.Clicks) != pointerMoves {
		t.Errorf("clicked %d times, want %d", len(page.Clicks), pointerMoves)
	}
}
