package runtime

import (
	"context"
	"errors"
	"strings"

	"github.com/alexbeatnik/manul-browser/core/pkg/browser"
	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
)

// errPointerSwallowed is a click the page never received, twice.
var errPointerSwallowed = errors.New("the click did not reach the page: nothing on it received a pointer event, and the same happened when it was tried again. " +
	"Something outside the page took it — in a browser manul attached to, usually its password manager's prompt after a sign-in. " +
	"Dismiss the prompt in the browser, or let manul launch a browser of its own, and run the step again")

// pointerAction runs a click-like action on el and checks that the page
// received it.
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
func (rt *Runtime) pointerAction(ctx context.Context, el dom.ElementSnapshot, act func() error) error {
	page := rt.pageFor(el)
	// A disabled control receives no mouse events by design.
	watch := !el.IsDisabled && armPointerWatch(ctx, page)
	if err := act(); err != nil || !watch {
		return err
	}
	if pointerReached(ctx, page) {
		return nil
	}
	rt.logger.Warn("The click did not reach the page; trying it once more")
	if !armPointerWatch(ctx, page) {
		return nil
	}
	if err := act(); err != nil {
		return err
	}
	if pointerReached(ctx, page) {
		return nil
	}
	return errPointerSwallowed
}

func armPointerWatch(ctx context.Context, page browser.Page) bool {
	_, err := page.EvalJS(ctx, pagejs.ArmPointerWatch)
	return err == nil
}

func pointerReached(ctx context.Context, page browser.Page) bool {
	raw, err := page.EvalJS(ctx, pagejs.PointerReceived)
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(raw)) != "0"
}
