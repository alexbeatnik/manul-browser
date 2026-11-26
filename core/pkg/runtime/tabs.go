package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/browser"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
)

// blankTabWait bounds the wait for a new tab to leave about:blank. A tab is
// listed before its first navigation starts, and a snapshot taken then is of
// an empty page; but a tab may also be opened blank on purpose, and that
// should not cost a whole nav_timeout.
const blankTabWait = 3 * time.Second

// Page returns the page the next step runs against — the tab most recently
// followed or switched to, not necessarily the one the Runtime was built with.
func (rt *Runtime) Page() browser.Page { return rt.page }

// noteTabs makes the page the hunt began on tab 1. It runs before the first
// step; it is not done in New, which has no context to ask the page with.
func (rt *Runtime) noteTabs(ctx context.Context) {
	if rt.tabsNoted {
		return
	}
	rt.tabsNoted = true
	rt.tabs = []browser.Page{rt.page}
	rt.openerOf = map[browser.Page]browser.Page{}
	rt.seenTabs = map[string]bool{}
	// A tab already open when the hunt began was not opened by any of its
	// steps, so it is never followed.
	if tabbed, ok := rt.page.(browser.Tabbed); ok {
		if open, err := tabbed.OpenedTabs(ctx); err == nil {
			for _, t := range open {
				rt.seenTabs[t.ID] = true
			}
		}
	}
}

// followTabs runs before every step. A tab that the current page opened
// since the last step becomes the page the step runs against, and a page
// whose tab has been closed — an OAuth popup closes itself — hands back to
// the one that opened it, or, if that is gone too, to the last tab still open.
//
// Before this, CLICK on a target="_blank" link passed and every step after
// it looked for its target on the page that had been left behind.
func (rt *Runtime) followTabs(ctx context.Context) {
	rt.noteTabs(ctx)
	tabbed, ok := rt.page.(browser.Tabbed)
	if !ok {
		return
	}
	if tabbed.TabClosed(ctx) {
		closed := rt.page
		rt.pruneClosedTabs(ctx)
		next := rt.openerOf[closed]
		if !rt.isOpenTab(next) && len(rt.tabs) > 0 {
			next = rt.tabs[len(rt.tabs)-1]
		}
		if next == nil {
			return
		}
		rt.selectTab(ctx, next)
		url, _ := rt.page.CurrentURL(ctx)
		rt.logger.ActionDetail("↩️", "The tab was closed; back to %s", url)
		if tabbed, ok = rt.page.(browser.Tabbed); !ok {
			return
		}
	}

	open, err := tabbed.OpenedTabs(ctx)
	if err != nil {
		return
	}
	var fresh *browser.Tab
	for i := range open {
		if !rt.seenTabs[open[i].ID] {
			rt.seenTabs[open[i].ID] = true
			fresh = &open[i]
		}
	}
	if fresh == nil {
		return
	}

	next, err := tabbed.AttachTab(ctx, fresh.ID)
	if err != nil {
		rt.logger.Warn("A new tab was opened (%s), but it could not be attached: %v", fresh.URL, err)
		return
	}
	rt.openerOf[next] = rt.page
	rt.tabs = append(rt.tabs, next)
	rt.selectTab(ctx, next)
	rt.waitForNewTab(ctx)
	url, _ := rt.page.CurrentURL(ctx)
	rt.logger.ActionDetail("🗂️", "Following the new tab (tab %d): %s", len(rt.tabs), url)
}

// pruneClosedTabs drops every tab that has been closed, so positions count
// open tabs only.
func (rt *Runtime) pruneClosedTabs(ctx context.Context) {
	kept := rt.tabs[:0]
	for _, p := range rt.tabs {
		if t, ok := p.(browser.Tabbed); ok && t.TabClosed(ctx) {
			continue
		}
		kept = append(kept, p)
	}
	rt.tabs = kept
}

func (rt *Runtime) isOpenTab(p browser.Page) bool {
	for _, t := range rt.tabs {
		if p != nil && t == p {
			return true
		}
	}
	return false
}

func (rt *Runtime) waitForNewTab(ctx context.Context) {
	blank, cancel := context.WithTimeout(ctx, blankTabWait)
	for {
		url, err := rt.page.CurrentURL(blank)
		if err == nil && url != "" && url != "about:blank" {
			break
		}
		select {
		case <-blank.Done():
		case <-time.After(50 * time.Millisecond):
			continue
		}
		break
	}
	cancel()
	limited, cancelNav := rt.withNavTimeout(ctx)
	defer cancelNav()
	_ = rt.page.WaitForLoad(limited)
}

// switchTab is SWITCH TO TAB: by position among the open tabs the hunt knows,
// 1 being the one it began on, or by text found in a tab's title or URL.
// Only tabs the hunt began on or followed are candidates — not every tab the
// browser has open.
func (rt *Runtime) switchTab(ctx context.Context, cmd dsl.Command) (string, error) {
	rt.pruneClosedTabs(ctx)
	if want := strings.TrimSpace(rt.resolveVariables(cmd.Target)); want != "" {
		needle := strings.ToLower(want)
		for i, p := range rt.tabs {
			url, _ := p.CurrentURL(ctx)
			// Wrapped in JSON.stringify: a bare string comes back unquoted.
			var title string
			if raw, err := p.EvalJS(ctx, "JSON.stringify(document.title)"); err == nil {
				_ = json.Unmarshal(raw, &title)
			}
			if strings.Contains(strings.ToLower(title), needle) || strings.Contains(strings.ToLower(url), needle) {
				rt.selectTab(ctx, p)
				return fmt.Sprintf("tab %d: %s", i+1, url), nil
			}
		}
		return "", fmt.Errorf("SWITCH TO TAB: no open tab has %q in its title or URL (%d open)", want, len(rt.tabs))
	}

	raw := strings.TrimSpace(rt.resolveVariables(cmd.Value))
	n, err := strconv.Atoi(raw)
	if err != nil {
		return "", fmt.Errorf("SWITCH TO TAB: want a tab number or a quoted title, got %q", raw)
	}
	if n < 1 || n > len(rt.tabs) {
		return "", fmt.Errorf("SWITCH TO TAB %d: there are %d open tab(s)", n, len(rt.tabs))
	}
	p := rt.tabs[n-1]
	rt.selectTab(ctx, p)
	url, _ := p.CurrentURL(ctx)
	return fmt.Sprintf("tab %d: %s", n, url), nil
}

// selectTab makes p the page steps run against and brings its tab to the
// front. Activation failing is not the step's failure: the engine drives a
// tab whether or not it is the one on screen.
func (rt *Runtime) selectTab(ctx context.Context, p browser.Page) {
	if p != rt.page {
		rt.page = p
		rt.invalidateSnapshot()
	}
	if t, ok := p.(browser.Tabbed); ok {
		_ = t.ActivateTab(ctx)
	}
}

// pollTick starts one look of a step that waits — VERIFY, WAIT FOR: a fresh
// snapshot, and the tab a page opened since the last look. A click that opens
// its tab from a timer, a moment later, left the next step looking at the old
// page: the tab was looked for once, before the step, and not after.
func (rt *Runtime) pollTick(ctx context.Context) {
	rt.followTabs(ctx)
	rt.invalidateSnapshot()
}
