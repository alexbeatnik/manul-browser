package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alexbeatnik/manul-browser/core/pkg/browser"
	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
)

// errPointerSwallowed is a click the page never received, twice.
var errPointerSwallowed = errors.New("the click did not reach the page: nothing on it received a pointer event, and the same happened when it was tried again. " +
	"Something outside the page took it — in a browser manul attached to, usually its password manager's prompt after a sign-in. " +
	"Dismiss the prompt in the browser, or let manul launch a browser of its own, and run the step again")

// pointerAction runs a click-like action on el at (x, y) and checks that the
// press reached it.
//
// Clicks go to coordinates, and what is on top at that moment receives them.
// clickPoint already refuses an element another one on the page covers; what
// it cannot see is browser UI above the page. In a browser the engine did not
// launch, the password manager's prompt after a sign-in took the clicks meant
// for the page, and CLICK 'Add to cart' passed with the cart still empty.
//
// So the element's document counts the pointer events it receives during the
// action. None at all means the click went elsewhere, and the page saw
// nothing — so trying again cannot act twice, and a prompt the first click
// dismissed lets the second through. Nothing the second time fails the step.
// A document that went away, or cannot be asked, did receive the action: it
// navigated, or is navigating.
//
// A press the page received but another element would have taken — the page
// moved between measuring and clicking — is stopped before the page sees it
// (pagejs.ArmPointerWatch), and the element is measured and clicked again
// where it is now, at most pointerMoves times.
func (rt *Runtime) pointerAction(ctx context.Context, el dom.ElementSnapshot, target string, x, y float64, act func(x, y float64) error) error {
	page := rt.pageFor(el)
	if el.IsDisabled {
		// A disabled control receives no mouse events by design.
		return act(x, y)
	}
	for swallowed, moved := 0, 0; ; {
		if !armPointerWatch(ctx, page, el) {
			return act(x, y)
		}
		if err := act(x, y); err != nil {
			return err
		}
		switch pointerOutcome(ctx, page) {
		case pointerMissed:
			moved++
			by := pointerMissedBy(ctx, page)
			if moved >= pointerMoves {
				return fmt.Errorf("the click on '%s' went to %s instead, %d times: the element moves as it is clicked, and nothing was clicked", target, by, moved)
			}
			rt.logger.Warn("'%s' moved as it was clicked, and the press went to %s; it was stopped, and the click is made where '%s' is now", target, by, target)
			nx, ny, err := rt.clickPoint(ctx, el, target)
			if err != nil {
				return err
			}
			x, y = nx, ny
		case pointerNone:
			if swallowed++; swallowed >= 2 {
				return errPointerSwallowed
			}
			rt.logger.Warn("The click did not reach the page; trying it once more")
		default:
			return nil
		}
	}
}

// pointerMoves is how many times a press may go to another element, because
// the one it was for moved, before the step fails.
const pointerMoves = 3

type pointerResult int

const (
	pointerReached pointerResult = iota
	pointerNone
	pointerMissed
)

func armPointerWatch(ctx context.Context, page browser.Page, el dom.ElementSnapshot) bool {
	_, err := page.EvalJS(ctx, pagejs.ArmPointerWatch(el.ID, el.XPath))
	return err == nil
}

// pointerOutcome is what the document saw of the press. A page that cannot be
// asked is one in transit: it received the action.
func pointerOutcome(ctx context.Context, page browser.Page) pointerResult {
	raw, err := page.EvalJS(ctx, pagejs.PointerReceived)
	if err != nil {
		return pointerReached
	}
	switch strings.TrimSpace(string(raw)) {
	case "0":
		return pointerNone
	case "3":
		return pointerMissed
	}
	return pointerReached
}

func pointerMissedBy(ctx context.Context, page browser.Page) string {
	raw, err := page.EvalJS(ctx, pagejs.PointerMissedBy)
	if err != nil || len(raw) == 0 {
		return "something else"
	}
	return string(raw)
}
