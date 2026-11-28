package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/browser"
	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
)

const (
	// routeQuiet is how long a page must go without a change of its DOM or
	// of a request it started for the action before it is taken to have
	// drawn what the action asked for.
	routeQuiet = 200 * time.Millisecond
	// routeSettleTimeout bounds the wait for the action's requests. A request
	// held open — a long poll that happened to start with the action — costs
	// this much and no more; the step then goes on as it did before.
	routeSettleTimeout = 5 * time.Second
	// routeQuietTimeout bounds the wait for the DOM once those requests are
	// back: a page that never goes still, an animation that rewrites text,
	// is given this long to draw the answer.
	routeQuietTimeout = time.Second
	// routePollInterval is how often the page is asked.
	routePollInterval = 50 * time.Millisecond
	// actionGrace is how long an action that has set nothing off yet is
	// given to start something: a form that Enter submits navigates from a
	// task of its own, a framework sends its request from an effect after
	// it renders. Every action used to sleep 300–500 ms before its settle
	// instead, whether the page did anything or not: three quarters of a
	// suite's time on a page that answers at once.
	actionGrace = 50 * time.Millisecond
	// gracePollInterval is how often the page is asked during actionGrace.
	gracePollInterval = 10 * time.Millisecond
)

// settleNavigation waits, no longer than nav_timeout, for what the last
// action set off to land: another document, loaded, or — in the same
// document — the requests the action started, the address they moved the
// page to, and the page drawing their answer.
//
// A SELECT whose change handler reloads the page waited for nothing, and the
// FILL after it typed into the old document: on a slow server it was still
// there, "complete", until the response came and replaced it — empty field,
// green step. An evaluation that fails is a document in transit, so it counts
// as still leaving; one that answers nothing useful (no watch armed) as not.
//
// A single-page app says nothing of the kind. On YouTube, Enter in the search
// box changed the address at once and drew the results when its request came
// back; the map after the step showed the suggestions and no results. On
// Rozetka the request came first and the address changed only when it
// answered, so the step returned on the old address. And a click that only
// asks the server, Add to cart, left the old total for the step after it to
// read. So a request the action started is waited for, at most
// routeSettleTimeout, and then the DOM, at most routeQuietTimeout; an action
// that started none and went nowhere is not waited for at all.
func (rt *Runtime) settleNavigation(ctx context.Context) {
	limited, cancel := rt.withNavTimeout(ctx)
	defer cancel()
	for end := time.Now().Add(actionGrace); time.Now().Before(end) && !rt.actionStarted(limited); {
		select {
		case <-limited.Done():
			return
		case <-time.After(gracePollInterval):
		}
	}
	// From here on a request counts as the action's only if the address has
	// moved: the action has had its moment to start what it starts.
	_, _ = rt.page.EvalJS(limited, pagejs.CloseActionWindow)
	var w routeWait
	for {
		raw, err := rt.page.EvalJS(limited, pagejs.NavigationState)
		switch state := strings.TrimSpace(string(raw)); {
		case err != nil:
			// A page that cannot be evaluated is usually one in transit — or
			// one whose tab the action closed: a sign-in popup's "Authorize"
			// does, and the step waited out nav_timeout for it to come back.
			if t, ok := rt.page.(browser.Tabbed); ok && t.TabClosed(limited) {
				return
			}
		case state == "1":
			// Leaving: wait for the document that replaces it.
		case state == "2":
			_ = rt.page.WaitForLoad(limited)
			return
		default:
			if w.settled(rt.routeState(limited)) {
				_ = rt.page.WaitForLoad(limited)
				return
			}
		}
		select {
		case <-limited.Done():
			return
		case <-time.After(routePollInterval):
		}
	}
}

// actionStarted reports whether the last action has set anything off: the
// document is leaving or gone, the address moved, or a request it started is
// in flight. A page that cannot be asked is one in transit.
func (rt *Runtime) actionStarted(ctx context.Context) bool {
	raw, err := rt.page.EvalJS(ctx, pagejs.NavigationState)
	if err != nil || strings.TrimSpace(string(raw)) != "0" {
		return true
	}
	st := rt.routeState(ctx)
	return !st.ok || st.Pending > 0 || st.Moved
}

// routeStatus is pagejs.RouteState's answer.
type routeStatus struct {
	Moved   bool    `json:"moved"`
	Pending int     `json:"pending"`
	Idle    float64 `json:"idle"`
	// ok is false when the page could not be asked: in transit, look again.
	ok bool
	// known is false when it answered something else: nothing to go by.
	known bool
}

func (rt *Runtime) routeState(ctx context.Context) routeStatus {
	raw, err := rt.page.EvalJS(ctx, pagejs.RouteState)
	if err != nil {
		return routeStatus{}
	}
	st := routeStatus{ok: true}
	st.known = json.Unmarshal(raw, &st) == nil
	return st
}

// routeWait is the same-document half of settleNavigation, one answer at a
// time.
type routeWait struct {
	busySince  time.Time // first seen with the action's requests in flight
	quietSince time.Time // first seen with them all back
}

func (w *routeWait) settled(st routeStatus) bool {
	if !st.ok {
		return false
	}
	if !st.known {
		return true
	}
	now := time.Now()
	if st.Pending > 0 {
		if w.busySince.IsZero() {
			w.busySince = now
		}
		w.quietSince = time.Time{}
		return now.Sub(w.busySince) >= routeSettleTimeout
	}
	// The action asked for nothing and went nowhere.
	if !st.Moved && w.busySince.IsZero() {
		return true
	}
	if w.quietSince.IsZero() {
		w.quietSince = now
	}
	return time.Duration(st.Idle)*time.Millisecond >= routeQuiet || now.Sub(w.quietSince) >= routeQuietTimeout
}
