package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/scorer"
)

// Explicit waits.
//
// The engine's ordinary actions already retry while resolving, so a hunt rarely
// needs to wait by hand. These exist for the cases where it does: a spinner that
// must clear before the next step means anything, or a widget that renders after
// an XHR the DSL has no other handle on.
const (
	// defaultWaitTimeout matches the Python engine's 15 seconds.
	defaultWaitTimeout = 15 * time.Second
	// waitPollInterval is a compromise: tight enough that a wait rarely costs
	// more than it must, loose enough not to hammer CDP with snapshots.
	waitPollInterval = 250 * time.Millisecond
)

// waitForElement blocks until the element named by cmd.Target reaches the
// requested state, or the timeout expires.
//
// States: visible (the default), hidden / disappear, enabled, disabled.
func (rt *Runtime) waitForElement(ctx context.Context, cmd dsl.Command) error {
	target := rt.resolveVariables(cmd.Target)
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("WAIT FOR: missing target")
	}

	state := strings.ToLower(strings.TrimSpace(cmd.WaitForState))
	if state == "" {
		state = "visible"
	}

	deadline := time.Now().Add(defaultWaitTimeout)
	var lastErr error

	for {
		// The snapshot cache exists to make resolution cheap within a step;
		// here it would make the wait blind to the change it is waiting for.
		rt.invalidateSnapshot()

		satisfied, err := rt.elementStateSatisfied(ctx, cmd, target, state)
		if err != nil {
			lastErr = err
		} else if satisfied {
			return nil
		}

		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("WAIT FOR %q to be %s: timed out after %s: %w",
					target, state, defaultWaitTimeout, lastErr)
			}
			return fmt.Errorf("WAIT FOR %q to be %s: timed out after %s",
				target, state, defaultWaitTimeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitPollInterval):
		}
	}
}

// elementStateSatisfied reports whether the target currently matches state.
func (rt *Runtime) elementStateSatisfied(ctx context.Context, cmd dsl.Command, target, state string) (bool, error) {
	elements, err := rt.loadSnapshot(ctx)
	if err != nil {
		return false, err
	}

	mode := string(cmd.InteractionMode)
	if mode == "" {
		mode = string(dsl.ModeNone)
	}
	ranked := scorer.Rank(target, cmd.TypeHint, mode, elements, 1, nil)

	// The scorer always ranks something, so "the top candidate exists" is not
	// the same as "the target is present". ThresholdAmbiguous is the same bar
	// the rest of the engine uses to call a target resolved; without it a wait
	// for an element that is not there passes on the first poll against
	// whatever scored highest.
	if len(ranked) == 0 || ranked[0].Explain.Score.Total < ThresholdAmbiguous {
		switch state {
		case "hidden", "disappear", "invisible", "gone":
			return true, nil
		}
		return false, nil
	}

	el := ranked[0].Element
	switch state {
	case "visible", "present", "shown":
		return el.IsVisible, nil
	case "hidden", "disappear", "invisible", "gone":
		return !el.IsVisible, nil
	case "enabled":
		return el.IsVisible && !el.IsDisabled, nil
	case "disabled":
		return el.IsDisabled, nil
	}
	return false, fmt.Errorf("unknown wait state %q", state)
}

// waitForSelector blocks until a CSS selector matches something in the DOM.
//
// Unlike every other targeting path this one bypasses the scorer entirely: the
// caller gave an exact selector, so honouring it literally is the whole point.
func (rt *Runtime) waitForSelector(ctx context.Context, selector string) error {
	selector = rt.resolveVariables(selector)
	if strings.TrimSpace(selector) == "" {
		return fmt.Errorf("WAIT FOR SELECTOR: missing selector")
	}

	sel, _ := json.Marshal(selector)
	js := `(() => { try { return document.querySelector(` + string(sel) + `) !== null; } catch (e) { return "bad-selector"; } })()`

	deadline := time.Now().Add(defaultWaitTimeout)
	for {
		raw, err := rt.page.EvalJS(ctx, js)
		if err == nil {
			// EvalJS returns booleans as JSON but strings as bare bytes, so the
			// sentinel is compared as text rather than decoded.
			text := strings.TrimSpace(strings.Trim(string(raw), `"`))
			if text == "bad-selector" {
				// An invalid selector will never match; waiting out the full
				// timeout would only delay a message the caller needs now.
				return fmt.Errorf("WAIT FOR SELECTOR %q: not a valid CSS selector", selector)
			}
			if text == "true" {
				return nil
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("WAIT FOR SELECTOR %q: timed out after %s", selector, defaultWaitTimeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitPollInterval):
		}
	}
}
