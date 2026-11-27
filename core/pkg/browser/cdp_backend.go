// Package browser — CDP backend implementation.
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/cdp"
	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
)

// CDPBrowser is the Chrome DevTools Protocol implementation of Browser.
type CDPBrowser struct {
	endpoint string
}

// NewCDPBrowser creates a CDPBrowser connected to the given HTTP endpoint.
// Example endpoint: "http://127.0.0.1:9222"
func NewCDPBrowser(endpoint string) *CDPBrowser {
	return &CDPBrowser{endpoint: endpoint}
}

// FirstPage attaches to the first available page target in the running Chrome instance.
func (b *CDPBrowser) FirstPage(ctx context.Context) (Page, error) {
	targets, err := cdp.ListTargets(ctx, b.endpoint)
	if err != nil {
		return nil, fmt.Errorf("browser: list targets: %w", err)
	}
	target, err := cdp.FindPageTarget(targets)
	if err != nil {
		return nil, fmt.Errorf("browser: find page target: %w", err)
	}
	conn, err := cdp.DialTarget(ctx, target.WSURL)
	if err != nil {
		return nil, fmt.Errorf("browser: dial target %q: %w", target.WSURL, err)
	}
	return newCDPPage(ctx, conn)
}

// PageMatching attaches to the first page-type target whose URL contains
// the given substring (case-insensitive). Returns an error when no target
// matches; callers should typically fall back to FirstPage or surface a
// clear error to the user.
//
// Chrome's /json/list orders targets most-recently-active first, so when
// the user has several tabs of the same site open, this picks the one
// they've used most recently. That matters for multi-tab CDP sessions
// where "the first page" would otherwise be a lottery.
func (b *CDPBrowser) PageMatching(ctx context.Context, urlSubstr string) (Page, error) {
	if urlSubstr == "" {
		return b.FirstPage(ctx)
	}
	targets, err := cdp.ListTargets(ctx, b.endpoint)
	if err != nil {
		return nil, fmt.Errorf("browser: list targets: %w", err)
	}
	needle := strings.ToLower(urlSubstr)
	for _, t := range targets {
		if t.Type != "page" {
			continue
		}
		if !strings.Contains(strings.ToLower(t.URL), needle) {
			continue
		}
		conn, err := cdp.DialTarget(ctx, t.WSURL)
		if err != nil {
			return nil, fmt.Errorf("browser: dial target %q: %w", t.WSURL, err)
		}
		return newCDPPage(ctx, conn)
	}
	return nil, fmt.Errorf("browser: no page target with URL containing %q (found %d targets)", urlSubstr, len(targets))
}

// NewPage opens a fresh blank tab. It is OpenTarget with a blank URL; the
// caller is responsible for closing the tab via CloseTarget when done (the
// returned Page only owns the per-tab WebSocket, not the tab itself).
func (b *CDPBrowser) NewPage(ctx context.Context) (Page, error) {
	page, _, err := b.OpenTarget(ctx, "about:blank")
	return page, err
}

// OpenTarget creates a new page target at url (a background tab — the user's
// active tab is not switched away from) and returns a connected Page plus the
// new target's ID. Use CloseTarget(targetID) to reap the tab. This is the
// browser-wide primitive behind background lookups.
func (b *CDPBrowser) OpenTarget(ctx context.Context, url string) (Page, string, error) {
	browserWS, err := cdp.BrowserWSURL(ctx, b.endpoint)
	if err != nil {
		return nil, "", fmt.Errorf("browser: browser ws: %w", err)
	}
	bconn, err := cdp.DialTarget(ctx, browserWS)
	if err != nil {
		return nil, "", fmt.Errorf("browser: dial browser ws: %w", err)
	}
	defer bconn.Close()

	raw, err := bconn.Call(ctx, "Target.createTarget", map[string]any{"url": url})
	if err != nil {
		return nil, "", fmt.Errorf("browser: createTarget: %w", err)
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.TargetID == "" {
		return nil, "", fmt.Errorf("browser: createTarget: bad response %s", string(raw))
	}

	wsURL, err := b.targetWSURL(ctx, created.TargetID)
	if err != nil {
		_ = b.CloseTarget(context.Background(), created.TargetID)
		return nil, "", err
	}
	conn, err := cdp.DialTarget(ctx, wsURL)
	if err != nil {
		_ = b.CloseTarget(context.Background(), created.TargetID)
		return nil, "", fmt.Errorf("browser: dial new target: %w", err)
	}
	page, err := newCDPPage(ctx, conn)
	if err != nil {
		_ = b.CloseTarget(context.Background(), created.TargetID)
		return nil, "", err
	}
	return page, created.TargetID, nil
}

// newCDPPage wraps a freshly dialled page connection. Every page gets its
// dialogs answered from the start — one that does not can hang on the first
// alert it meets — and is treated as focused, so focus handlers run when the
// engine focuses a field rather than at the next click.
func newCDPPage(ctx context.Context, conn *cdp.Conn) (*CDPPage, error) {
	if err := cdp.AcceptDialogs(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("browser: answer dialogs: %w", err)
	}
	if err := cdp.EmulateFocus(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("browser: emulate focus: %w", err)
	}
	// Best effort: without it the count is installed when a step first
	// arms, and misses only an app that kept its fetch from start-up.
	_ = cdp.AddScriptOnNewDocument(ctx, conn, "("+pagejs.RequestWatchFunction+")()")
	return &CDPPage{conn: conn}, nil
}

// targetWSURL polls /json/list until the freshly-created target exposes its
// per-target WebSocket URL (it can lag the createTarget reply by a few ms).
func (b *CDPBrowser) targetWSURL(ctx context.Context, targetID string) (string, error) {
	for {
		targets, err := cdp.ListTargets(ctx, b.endpoint)
		if err == nil {
			for _, t := range targets {
				if t.ID == targetID && t.WSURL != "" {
					return t.WSURL, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("browser: target %s ws url not ready: %w", targetID, ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// CloseTarget closes a tab by target ID via the browser-level connection.
func (b *CDPBrowser) CloseTarget(ctx context.Context, targetID string) error {
	browserWS, err := cdp.BrowserWSURL(ctx, b.endpoint)
	if err != nil {
		return err
	}
	bconn, err := cdp.DialTarget(ctx, browserWS)
	if err != nil {
		return err
	}
	defer bconn.Close()
	_, err = bconn.Call(ctx, "Target.closeTarget", map[string]any{"targetId": targetID})
	return err
}

// Close is a no-op for CDPBrowser — we don't own the browser process.
func (b *CDPBrowser) Close() error { return nil }

// ── CDPPage ───────────────────────────────────────────────────────────────────

// CDPPage is the CDP implementation of Page.
type CDPPage struct {
	conn    *cdp.Conn
	tracker *cdp.FrameTracker
	// id is this page's target id, learnt on first use (tabs.go).
	id string
	// mocks is nil until the first MOCK: no request is paused before then.
	mocks *mockTable
}

// ensureTracker lazily enables the Page/Runtime domains and starts tracking
// per-frame execution contexts. Done on first use so simple one-shot pages
// don't pay for event subscription they never need.
func (p *CDPPage) ensureTracker(ctx context.Context) (*cdp.FrameTracker, error) {
	if p.tracker != nil {
		return p.tracker, nil
	}
	t, err := cdp.NewFrameTracker(ctx, p.conn)
	if err != nil {
		return nil, err
	}
	p.tracker = t
	return t, nil
}

// Frames returns the page's frames (main + iframes) with their execution contexts.
func (p *CDPPage) Frames(ctx context.Context) ([]FrameRef, error) {
	t, err := p.ensureTracker(ctx)
	if err != nil {
		return nil, err
	}
	frames := t.Frames()
	out := make([]FrameRef, 0, len(frames))
	for _, f := range frames {
		out = append(out, FrameRef{Index: f.Index, URL: f.URL, Name: f.Name})
	}
	return out, nil
}

// EvalJSInFrame evaluates expr in the execution context of frame frameIndex.
func (p *CDPPage) EvalJSInFrame(ctx context.Context, frameIndex int, expr string) ([]byte, error) {
	if frameIndex == 0 {
		return p.EvalJS(ctx, expr)
	}
	return p.evalInFrame(ctx, frameIndex, autoInvoke(expr))
}

// CallProbeInFrame calls a JS arrow-function with a JSON arg in frame frameIndex.
func (p *CDPPage) CallProbeInFrame(ctx context.Context, frameIndex int, fn string, arg any) ([]byte, error) {
	if frameIndex == 0 {
		return p.CallProbe(ctx, fn, arg)
	}
	expr := fn
	if arg != nil {
		expr = fmt.Sprintf("(%s)(%s)", fn, cdp.MustMarshalString(arg))
	} else {
		expr = autoInvoke(fn)
	}
	return p.evalInFrame(ctx, frameIndex, expr)
}

// evalInFrame evaluates in the frame's own context — on the page's session,
// or on the session of the out-of-process iframe that holds it.
func (p *CDPPage) evalInFrame(ctx context.Context, frameIndex int, expr string) ([]byte, error) {
	t, err := p.ensureTracker(ctx)
	if err != nil {
		return nil, err
	}
	session, contextID := t.TargetForIndex(frameIndex)
	if contextID == 0 {
		return nil, errNoFrameContext(frameIndex)
	}
	raw, err := cdp.EvaluateInSession(ctx, p.conn, session, contextID, expr)
	if err != nil {
		return nil, err
	}
	return marshalEvalResult(raw)
}

// FrameOrigin implements FrameLocator. Each frame up the chain is measured in
// the document that holds it: DOM.getFrameOwner names its <iframe>, asked on
// the session of the parent frame, and FrameBoxOn measures that element.
func (p *CDPPage) FrameOrigin(ctx context.Context, frameIndex int) (x, y, w, h float64, err error) {
	t, err := p.ensureTracker(ctx)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	byID := map[string]cdp.FrameInfo{}
	var frame cdp.FrameInfo
	for _, f := range t.Frames() {
		byID[f.FrameID] = f
		if f.Index == frameIndex {
			frame = f
		}
	}
	if frame.FrameID == "" || frameIndex == 0 {
		return 0, 0, 0, 0, fmt.Errorf("no frame %d", frameIndex)
	}
	first := true
	for f := frame; f.ParentID != ""; {
		parent, ok := byID[f.ParentID]
		if !ok {
			return 0, 0, 0, 0, fmt.Errorf("the parent of frame %s is not known", f.FrameID)
		}
		bx, by, bw, bh, err := p.ownerBox(ctx, parent.SessionID, f.FrameID)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		x, y = x+bx, y+by
		if first {
			w, h, first = bw, bh, false
		}
		f = parent
	}
	return x, y, w, h, nil
}

// PlaceFrames implements FramePlacer: every frame measured at once, each in
// the document that holds it, then summed up the chain.
func (p *CDPPage) PlaceFrames(ctx context.Context) (map[int]Placement, error) {
	t, err := p.ensureTracker(ctx)
	if err != nil {
		return nil, err
	}
	frames := t.Frames()
	byID := map[string]cdp.FrameInfo{}
	for _, f := range frames {
		byID[f.FrameID] = f
	}
	var mu sync.Mutex
	local := map[string]Placement{}
	var wg sync.WaitGroup
	for _, f := range frames {
		parent, ok := byID[f.ParentID]
		if f.Index == 0 || !ok {
			continue
		}
		wg.Add(1)
		go func(f, parent cdp.FrameInfo) {
			defer wg.Done()
			x, y, w, h, err := p.ownerBox(ctx, parent.SessionID, f.FrameID)
			if err != nil {
				return
			}
			mu.Lock()
			local[f.FrameID] = Placement{X: x, Y: y, W: w, H: h}
			mu.Unlock()
		}(f, parent)
	}
	wg.Wait()
	out := map[int]Placement{}
	for _, f := range frames {
		place, ok := local[f.FrameID]
		if !ok {
			continue
		}
		for up := byID[f.ParentID]; ok && up.ParentID != ""; up = byID[up.ParentID] {
			var box Placement
			if box, ok = local[up.FrameID]; ok {
				place.X, place.Y = place.X+box.X, place.Y+box.Y
			}
		}
		if ok {
			out[f.Index] = place
		}
	}
	return out, nil
}

func (p *CDPPage) ownerBox(ctx context.Context, session, frameID string) (x, y, w, h float64, err error) {
	raw, err := p.conn.CallSession(ctx, session, "DOM.getFrameOwner", map[string]any{"frameId": frameID})
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("frame owner: %w", err)
	}
	var owner struct {
		BackendNodeID int `json:"backendNodeId"`
	}
	if err := json.Unmarshal(raw, &owner); err != nil || owner.BackendNodeID == 0 {
		return 0, 0, 0, 0, fmt.Errorf("frame owner: bad reply %s", raw)
	}
	raw, err = p.conn.CallSession(ctx, session, "DOM.resolveNode", map[string]any{"backendNodeId": owner.BackendNodeID})
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("frame owner: %w", err)
	}
	var node struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(raw, &node); err != nil || node.Object.ObjectID == "" {
		return 0, 0, 0, 0, fmt.Errorf("frame owner: bad reply %s", raw)
	}
	defer func() {
		_, _ = p.conn.CallSession(context.Background(), session, "Runtime.releaseObject", map[string]any{"objectId": node.Object.ObjectID})
	}()
	raw, err = p.conn.CallSession(ctx, session, "Runtime.callFunctionOn", map[string]any{
		"objectId":            node.Object.ObjectID,
		"functionDeclaration": pagejs.FrameBoxOn,
		"returnByValue":       true,
	})
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("frame box: %w", err)
	}
	var res struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("frame box: bad reply %s", raw)
	}
	return frameBox([]byte(res.Result.Value))
}

// autoInvoke wraps a bare arrow/function literal so it is executed, mirroring
// CDPPage.EvalJS's behaviour for the per-frame path.
// errNoFrameContext is what a frame with no execution context yet answers.
// Context 0 means "the default one" to Runtime.evaluate, so asking such a
// frame anything used to ask the main document instead, and its "snapshot"
// came back as a second copy of the page.
func errNoFrameContext(frameIndex int) error {
	return fmt.Errorf("frame %d has no execution context yet", frameIndex)
}

func autoInvoke(expr string) string {
	trimmed := strings.TrimSpace(expr)
	if strings.HasPrefix(trimmed, "() =>") || strings.HasPrefix(trimmed, "()=>") ||
		strings.HasPrefix(trimmed, "function") {
		return "(" + trimmed + ")()"
	}
	return expr
}

func marshalEvalResult(raw interface{}) ([]byte, error) {
	if raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return json.Marshal(v)
	}
}

func (p *CDPPage) Navigate(ctx context.Context, url string) error {
	if err := cdp.Navigate(ctx, p.conn, url); err != nil {
		return err
	}

	// Two-phase wait to avoid the "stale readyState" race condition:
	// Phase 1 — wait for readyState to leave "complete", indicating
	//   the browser has begun the new navigation. We cap this at 500ms
	//   so same-page navigations (anchors, cached pages) don't hang.
	phase1Deadline := time.Now().Add(500 * time.Millisecond)
	const poll = 50 * time.Millisecond
	for time.Now().Before(phase1Deadline) {
		raw, err := cdp.Evaluate(ctx, p.conn, "document.readyState")
		if err != nil {
			// JS eval can fail briefly during navigation — that itself
			// means the navigation has started.
			break
		}
		if stateStr, ok := raw.(string); ok && stateStr != "complete" {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}

	// Phase 2 — wait for the new page to reach readyState=="complete".
	return p.WaitForLoad(ctx)
}

func (p *CDPPage) EvalJS(ctx context.Context, expr string) ([]byte, error) {
	// Callers commonly pass an arrow-function literal like `() => {...}` —
	// intending to run it, not just evaluate the function object. Plain
	// Runtime.evaluate on such an expression returns the function (which
	// serializes to `{}` under returnByValue) and produces zero scan
	// results. Auto-invoke it so the JS body actually runs.
	wrappedExpr := expr
	trimmed := strings.TrimSpace(expr)
	if strings.HasPrefix(trimmed, "() =>") || strings.HasPrefix(trimmed, "()=>") ||
		strings.HasPrefix(trimmed, "function") {
		wrappedExpr = "(" + trimmed + ")()"
	}

	raw, err := cdp.Evaluate(ctx, p.conn, wrappedExpr)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		b, e := json.Marshal(v)
		return b, e
	}
}

func (p *CDPPage) CallProbe(ctx context.Context, fn string, arg any) ([]byte, error) {
	raw, err := cdp.CallFunctionOn(ctx, p.conn, fn, arg)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		b, e := json.Marshal(v)
		return b, e
	}
}

func (p *CDPPage) Click(ctx context.Context, x, y float64) error {
	return cdp.Click(ctx, p.conn, x, y)
}

func (p *CDPPage) Focus(ctx context.Context, id int, xpath string) error {
	return p.conn.Focus(ctx, id, xpath)
}

func (p *CDPPage) SetInputValue(ctx context.Context, id int, xpath, value string) error {
	return p.conn.SetInputValue(ctx, id, xpath, value)
}

func (p *CDPPage) ScrollIntoView(ctx context.Context, id int, xpath string) error {
	return p.conn.ScrollIntoView(ctx, id, xpath)
}
func (p *CDPPage) SetChecked(ctx context.Context, id int, xpath string, checked bool) error {
	return p.conn.SetChecked(ctx, id, xpath, checked)
}

func (p *CDPPage) ScrollPage(ctx context.Context, direction, container string) error {
	return cdp.ScrollPage(ctx, p.conn, direction, container)
}

func (p *CDPPage) DoubleClick(ctx context.Context, x, y float64) error {
	return cdp.DoubleClick(ctx, p.conn, x, y)
}

func (p *CDPPage) RightClick(ctx context.Context, x, y float64) error {
	return cdp.RightClick(ctx, p.conn, x, y)
}

func (p *CDPPage) Hover(ctx context.Context, x, y float64) error {
	return cdp.Hover(ctx, p.conn, x, y)
}

func (p *CDPPage) DragAndDrop(ctx context.Context, fromX, fromY, toX, toY float64) error {
	return cdp.DragAndDrop(ctx, p.conn, fromX, fromY, toX, toY)
}

func (p *CDPPage) SetFileInput(ctx context.Context, id int, xpath string, filePaths []string) error {
	return p.conn.SetFileInput(ctx, id, xpath, filePaths)
}

func (p *CDPPage) setFileInputInFrame(ctx context.Context, frameIndex, id int, xpath string, filePaths []string) error {
	t, err := p.ensureTracker(ctx)
	if err != nil {
		return err
	}
	session, contextID := t.TargetForIndex(frameIndex)
	if contextID == 0 {
		return errNoFrameContext(frameIndex)
	}
	return p.conn.SetFileInputIn(ctx, session, contextID, id, xpath, filePaths)
}

func (p *CDPPage) Screenshot(ctx context.Context) ([]byte, error) {
	return cdp.Screenshot(ctx, p.conn)
}

func (p *CDPPage) Mock(ctx context.Context, rule MockRule) error {
	if p.mocks == nil {
		p.mocks = &mockTable{}
		cdp.OnRequestPaused(p.conn, p.answerPaused)
	}
	p.mocks.set(rule)
	// Only URLs some rule could match are paused; the rest never leave
	// Chrome. A rule's pattern is matched against the end of the URL.
	patterns := p.mocks.patterns()
	for i := range patterns {
		patterns[i] = "*" + patterns[i]
	}
	return cdp.InterceptRequests(ctx, p.conn, patterns)
}

// ClearMocks forgets every MOCK rule given to this page. Interception stays
// on; with no rule left to match, each paused request goes to the network.
func (p *CDPPage) ClearMocks() {
	if p.mocks != nil {
		p.mocks.clear()
	}
}

// answerPaused gives one paused request its mock, or sends it on its way.
func (p *CDPPage) answerPaused(req cdp.PausedRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), mockAnswerTimeout)
	defer cancel()
	header := func(name string) string {
		for k, v := range req.Headers {
			if strings.EqualFold(k, name) {
				return v
			}
		}
		return ""
	}
	if a := p.mocks.answer(req.Method, req.URL, header); a != nil {
		_ = cdp.FulfillRequest(ctx, p.conn, req.ID, a.Status, a.Headers, a.Body)
		return
	}
	_ = cdp.ContinueRequest(ctx, p.conn, req.ID)
}

func (p *CDPPage) WaitForResponse(ctx context.Context, urlPattern string, timeout time.Duration) error {
	return cdp.WaitForResponse(ctx, p.conn, urlPattern, timeout)
}

func (p *CDPPage) HighlightElement(ctx context.Context, id int, xpath string, durationMS int) error {
	return p.conn.HighlightElement(ctx, id, xpath, durationMS)
}

func (p *CDPPage) ClearHighlight(ctx context.Context) error {
	return p.conn.ClearHighlight(ctx)
}

func (p *CDPPage) GetDragCenters(ctx context.Context, srcID int, srcXPath string, dstID int, dstXPath string) (float64, float64, float64, float64, error) {
	return p.conn.GetDragCenters(ctx, srcID, srcXPath, dstID, dstXPath)
}

func (p *CDPPage) GetElementCenter(ctx context.Context, id int, xpath string) (float64, float64, error) {
	return p.conn.GetElementCenter(ctx, id, xpath)
}

func (p *CDPPage) DispatchKey(ctx context.Context, key string, modifiers int) error {
	key = normalizeKeyName(key)
	params := cdp.KeyEventParams{
		Key:                   key,
		Code:                  keyToCode(key),
		WindowsVirtualKeyCode: keyToVirtualCode(key),
		Modifiers:             modifiers,
	}
	// Character-producing keys must carry text on keyDown: Chrome treats a
	// keyDown without text as a rawKeyDown — no keypress fires, so PRESS Enter
	// never submitted forms on sites that listen for keypress/submit (the
	// "search stayed on the homepage" failure), and printable keys typed
	// nothing. The matching keyUp must NOT carry text.
	//
	// A chord is a command, not a character: with Control, Alt or Meta held
	// the key carries no text, or Control+A would type an "a" as well as
	// selecting everything. Shift alone still types.
	if text := keyText(key); text != "" && modifiers&^8 == 0 {
		params.Text = text
		params.UnmodifiedText = text
	}
	if err := cdp.DispatchKeyEvent(ctx, p.conn, "keyDown", params); err != nil {
		return err
	}
	params.Text = ""
	params.UnmodifiedText = ""
	return cdp.DispatchKeyEvent(ctx, p.conn, "keyUp", params)
}

// normalizeKeyName maps the spellings agents/LLMs actually emit ("enter",
// "ESC", "Return", "space") to the canonical DOM KeyboardEvent.key values
// Chrome expects. Unknown names pass through unchanged.
func normalizeKeyName(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "enter", "return":
		return "Enter"
	case "esc", "escape":
		return "Escape"
	case "tab":
		return "Tab"
	case "space", "spacebar":
		return " "
	case "backspace":
		return "Backspace"
	case "delete", "del":
		return "Delete"
	case "up", "arrowup":
		return "ArrowUp"
	case "down", "arrowdown":
		return "ArrowDown"
	case "left", "arrowleft":
		return "ArrowLeft"
	case "right", "arrowright":
		return "ArrowRight"
	case "home":
		return "Home"
	case "end":
		return "End"
	case "pageup", "page up":
		return "PageUp"
	case "pagedown", "page down":
		return "PageDown"
	}
	return strings.TrimSpace(key)
}

// keyText returns the character a key produces ("" for non-printing keys).
// Sent as keyDown text so Chrome generates the keypress/beforeinput events
// that form submission and typing depend on.
func keyText(key string) string {
	switch key {
	case "Enter":
		return "\r"
	case " ":
		return " "
	}
	if len([]rune(key)) == 1 {
		return key
	}
	return ""
}

// keyToCode maps canonical key values to KeyboardEvent.code values for the
// common keys agents press. Unknown keys return "" (Chrome tolerates that).
func keyToCode(key string) string {
	switch key {
	case "Enter":
		return "Enter"
	case "Tab":
		return "Tab"
	case "Escape":
		return "Escape"
	case " ":
		return "Space"
	case "Backspace":
		return "Backspace"
	case "Delete":
		return "Delete"
	case "ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight",
		"Home", "End", "PageUp", "PageDown":
		return key
	}
	if len(key) == 1 {
		c := key[0]
		switch {
		case c >= 'a' && c <= 'z':
			return "Key" + string(c-32)
		case c >= 'A' && c <= 'Z':
			return "Key" + string(c)
		case c >= '0' && c <= '9':
			return "Digit" + string(c)
		}
	}
	return ""
}

// keyToVirtualCode maps common key names to Windows virtual key codes.
func keyToVirtualCode(key string) int {
	codes := map[string]int{
		"Enter": 13, "Tab": 9, "Escape": 27, "Backspace": 8,
		"Delete": 46, "ArrowUp": 38, "ArrowDown": 40,
		"ArrowLeft": 37, "ArrowRight": 39, "Home": 36, "End": 35,
		"PageUp": 33, "PageDown": 34, " ": 32, "F1": 112,
		"F2": 113, "F3": 114, "F4": 115, "F5": 116, "F6": 117,
		"F7": 118, "F8": 119, "F9": 120, "F10": 121, "F11": 122, "F12": 123,
	}
	if code, ok := codes[key]; ok {
		return code
	}
	// Printable ASCII: virtual key code is the uppercase character.
	if len(key) == 1 {
		c := key[0]
		switch {
		case c >= 'a' && c <= 'z':
			return int(c) - 32
		case c >= 'A' && c <= 'Z' || c >= '0' && c <= '9':
			return int(c)
		}
	}
	return 0
}

func (p *CDPPage) CurrentURL(ctx context.Context) (string, error) {
	return cdp.GetCurrentURL(ctx, p.conn)
}

func (p *CDPPage) WaitForLoad(ctx context.Context) error {
	// Poll document.readyState via JS instead of relying on CDP event registration.
	// Event-based approach misses loadEventFired when the page loads before we
	// register the handler. JS polling is always accurate.
	const pollInterval = 150 * time.Millisecond
	for {
		raw, err := cdp.Evaluate(ctx, p.conn, "document.readyState")
		if err == nil && raw != nil {
			if stateStr, ok := raw.(string); ok && stateStr == "complete" {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (p *CDPPage) Wait(ctx context.Context, duration time.Duration) error {
	select {
	case <-time.After(duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *CDPPage) Close() error {
	return p.conn.Close()
}
