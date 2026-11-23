package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alexbeatnik/manul-browser/core/pkg/bidi"
	"github.com/alexbeatnik/manul-browser/core/pkg/cdp"
)

// Tab is a top-level tab, identified by the backend's own id for it: a CDP
// target id, or a BiDi browsing-context id.
type Tab struct {
	ID  string
	URL string
}

// Tabbed is implemented by a page that can see the tabs it opens — a link
// with target="_blank", window.open — and tell when its own tab is closed.
//
// Both backends answer from a listing, not from events: Chrome and Firefox
// both list the new tab, with this page as its opener, before the click that
// opened it returns, so there is no subscription to get the order of wrong.
type Tabbed interface {
	Page
	// OpenedTabs lists the open top-level tabs whose opener is this page.
	// rel="noopener" does not hide one: the browser still records who opened it.
	OpenedTabs(ctx context.Context) ([]Tab, error)
	// AttachTab returns a page driving the tab with the given id.
	AttachTab(ctx context.Context, id string) (Page, error)
	// TabClosed reports whether this page's own tab has been closed.
	TabClosed(ctx context.Context) bool
	// ActivateTab brings this page's tab to the front, so a headed browser
	// shows the tab the engine is driving.
	ActivateTab(ctx context.Context) error
}

// ── CDP ───────────────────────────────────────────────────────────────────────

// targetID is this page's own target id. A page connection is asked rather
// than told: Target.getTargetInfo with no id describes the target it is on.
func (p *CDPPage) targetID(ctx context.Context) (string, error) {
	if p.id != "" {
		return p.id, nil
	}
	raw, err := p.conn.Call(ctx, "Target.getTargetInfo", nil)
	if err != nil {
		return "", err
	}
	var res struct {
		TargetInfo struct {
			TargetID string `json:"targetId"`
		} `json:"targetInfo"`
	}
	if err := json.Unmarshal(raw, &res); err != nil || res.TargetInfo.TargetID == "" {
		return "", fmt.Errorf("browser: getTargetInfo: bad response %s", raw)
	}
	p.id = res.TargetInfo.TargetID
	return p.id, nil
}

type cdpTargetInfo struct {
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
	URL      string `json:"url"`
	OpenerID string `json:"openerId"`
}

func (p *CDPPage) pageTargets(ctx context.Context) ([]cdpTargetInfo, error) {
	raw, err := p.conn.Call(ctx, "Target.getTargets", map[string]any{
		"filter": []map[string]any{{"type": "page"}},
	})
	if err != nil {
		return nil, err
	}
	var res struct {
		TargetInfos []cdpTargetInfo `json:"targetInfos"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("browser: getTargets: %w", err)
	}
	return res.TargetInfos, nil
}

func (p *CDPPage) OpenedTabs(ctx context.Context) ([]Tab, error) {
	self, err := p.targetID(ctx)
	if err != nil {
		return nil, err
	}
	targets, err := p.pageTargets(ctx)
	if err != nil {
		return nil, err
	}
	var tabs []Tab
	for _, t := range targets {
		if t.Type == "page" && t.OpenerID == self {
			tabs = append(tabs, Tab{ID: t.TargetID, URL: t.URL})
		}
	}
	return tabs, nil
}

// AttachTab dials the tab's own WebSocket. Its address is this page's with
// the target id swapped: ws://host:port/devtools/page/<id>.
func (p *CDPPage) AttachTab(ctx context.Context, id string) (Page, error) {
	self, err := p.targetID(ctx)
	if err != nil {
		return nil, err
	}
	own := p.conn.URL()
	if !strings.HasSuffix(own, "/"+self) {
		return nil, fmt.Errorf("browser: cannot derive the new tab's address from %q", own)
	}
	conn, err := cdp.DialTarget(ctx, strings.TrimSuffix(own, self)+id)
	if err != nil {
		return nil, fmt.Errorf("browser: dial new tab: %w", err)
	}
	return newCDPPage(ctx, conn)
}

// TabClosed: Chrome closes a tab's socket when the tab goes, and a tab that
// is still listed is not closed whatever its socket says.
func (p *CDPPage) TabClosed(ctx context.Context) bool {
	if p.conn.Closed() {
		return true
	}
	self, err := p.targetID(ctx)
	if err != nil {
		return false
	}
	targets, err := p.pageTargets(ctx)
	if err != nil {
		return false
	}
	for _, t := range targets {
		if t.TargetID == self {
			return false
		}
	}
	return true
}

// ── BiDi ──────────────────────────────────────────────────────────────────────

func (p *BiDiPage) OpenedTabs(ctx context.Context) ([]Tab, error) {
	contexts, err := bidi.GetTree(ctx, p.conn, "")
	if err != nil {
		return nil, err
	}
	var tabs []Tab
	for _, c := range contexts {
		if c.OriginalOpener == p.contextID {
			tabs = append(tabs, Tab{ID: c.ID, URL: c.URL})
		}
	}
	return tabs, nil
}

// AttachTab: every page of a Firefox shares its one connection, so a tab is
// a context id on it.
func (p *BiDiPage) AttachTab(ctx context.Context, id string) (Page, error) {
	return newBiDiPage(ctx, p.conn, id)
}

func (p *BiDiPage) TabClosed(ctx context.Context) bool {
	if p.conn.Closed() {
		return true
	}
	contexts, err := bidi.GetTree(ctx, p.conn, "")
	if err != nil {
		return false
	}
	for _, c := range contexts {
		if c.ID == p.contextID {
			return false
		}
	}
	return true
}

func (p *CDPPage) ActivateTab(ctx context.Context) error {
	_, err := p.conn.Call(ctx, "Page.bringToFront", nil)
	return err
}

func (p *BiDiPage) ActivateTab(ctx context.Context) error {
	_, err := p.conn.Call(ctx, "browsingContext.activate", map[string]any{"context": p.contextID})
	return err
}
