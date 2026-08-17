// Package browser — WebDriver BiDi backend (Firefox).
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/bidi"
	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
)

// Compile-time proof that the BiDi backend satisfies the same contracts the
// CDP one does. A Page method added to the interface must be implemented here
// too, or this file stops building.
var (
	_ Backend = (*BiDiBrowser)(nil)
	_ Page    = (*BiDiPage)(nil)
)

// BiDiBrowser is the WebDriver BiDi implementation of Backend, used for
// Firefox. Firefox dropped CDP in version 141, so BiDi is not an alternative
// there — it is the protocol.
type BiDiBrowser struct {
	endpoint string
}

// NewBiDiBrowser creates a BiDiBrowser for the given endpoint, which is either
// a BiDi WebSocket URL ("ws://127.0.0.1:9222/session") or an HTTP endpoint to
// discover one from.
func NewBiDiBrowser(endpoint string) *BiDiBrowser {
	return &BiDiBrowser{endpoint: endpoint}
}

// Firefox permits exactly one WebDriver session per browser, and a session is
// bound to its socket. A second connection would therefore fail session.new
// rather than open a second view of the same browser — so every page of one
// endpoint shares a single connection, and pages address themselves by
// browsing-context id instead of by socket (the CDP backend's model).
var (
	sharedConnMu sync.Mutex
	sharedConns  = map[string]*bidi.Conn{}
)

func sharedConn(ctx context.Context, endpoint string) (*bidi.Conn, error) {
	sharedConnMu.Lock()
	defer sharedConnMu.Unlock()
	if c, ok := sharedConns[endpoint]; ok {
		if !c.Closed() {
			return c, nil
		}
		// The browser behind it went away (killed, crashed, or a previous run
		// on the same port). Drop it and dial the current one.
		delete(sharedConns, endpoint)
	}
	c, err := bidi.Connect(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("browser: connect bidi %q: %w", endpoint, err)
	}
	sharedConns[endpoint] = c
	return c, nil
}

// FirstPage attaches to the first top-level browsing context.
func (b *BiDiBrowser) FirstPage(ctx context.Context) (Page, error) {
	conn, err := sharedConn(ctx, b.endpoint)
	if err != nil {
		return nil, err
	}
	contexts, err := bidi.GetTree(ctx, conn, "")
	if err != nil {
		return nil, fmt.Errorf("browser: get context tree: %w", err)
	}
	if len(contexts) == 0 {
		return nil, fmt.Errorf("browser: no browsing context found")
	}
	return &BiDiPage{conn: conn, contextID: contexts[0].ID}, nil
}

// PageMatching attaches to the first top-level context whose URL contains the
// given substring (case-insensitive), falling back to FirstPage for an empty
// substring — the same contract the CDP backend offers.
func (b *BiDiBrowser) PageMatching(ctx context.Context, urlSubstr string) (Page, error) {
	if urlSubstr == "" {
		return b.FirstPage(ctx)
	}
	conn, err := sharedConn(ctx, b.endpoint)
	if err != nil {
		return nil, err
	}
	contexts, err := bidi.GetTree(ctx, conn, "")
	if err != nil {
		return nil, fmt.Errorf("browser: get context tree: %w", err)
	}
	needle := strings.ToLower(urlSubstr)
	for _, c := range contexts {
		if strings.Contains(strings.ToLower(c.URL), needle) {
			return &BiDiPage{conn: conn, contextID: c.ID}, nil
		}
	}
	return nil, fmt.Errorf("browser: no page with URL containing %q (found %d contexts)", urlSubstr, len(contexts))
}

// NewPage opens a fresh blank tab.
func (b *BiDiBrowser) NewPage(ctx context.Context) (Page, error) {
	page, _, err := b.OpenTarget(ctx, "about:blank")
	return page, err
}

// OpenTarget creates a new tab at url and returns it with its context id, for
// CloseTarget to reap later.
func (b *BiDiBrowser) OpenTarget(ctx context.Context, url string) (Page, string, error) {
	conn, err := sharedConn(ctx, b.endpoint)
	if err != nil {
		return nil, "", err
	}
	contextID, err := bidi.CreateContext(ctx, conn)
	if err != nil {
		return nil, "", fmt.Errorf("browser: create context: %w", err)
	}
	page := &BiDiPage{conn: conn, contextID: contextID}
	if url != "" && url != "about:blank" {
		if err := bidi.Navigate(ctx, conn, contextID, url); err != nil {
			_ = bidi.CloseContext(context.Background(), conn, contextID)
			return nil, "", fmt.Errorf("browser: navigate new context: %w", err)
		}
	}
	return page, contextID, nil
}

// CloseTarget closes a tab by context id.
func (b *BiDiBrowser) CloseTarget(ctx context.Context, targetID string) error {
	conn, err := sharedConn(ctx, b.endpoint)
	if err != nil {
		return err
	}
	return bidi.CloseContext(ctx, conn, targetID)
}

// Close is a no-op: the connection is shared by every page of this endpoint
// and outlives any one Browser handle, exactly as CDPBrowser.Close does not
// kill the browser it is pointed at.
func (b *BiDiBrowser) Close() error { return nil }

// ── BiDiPage ──────────────────────────────────────────────────────────────────

// BiDiPage is the BiDi implementation of Page. It is a browsing-context id on
// a shared connection, not a socket of its own.
type BiDiPage struct {
	conn      *bidi.Conn
	contextID string
}

// ContextID exposes the browsing-context id this page drives.
func (p *BiDiPage) ContextID() string { return p.contextID }

func (p *BiDiPage) eval(ctx context.Context, expr string) (interface{}, error) {
	return bidi.Evaluate(ctx, p.conn, p.contextID, expr)
}

func (p *BiDiPage) Navigate(ctx context.Context, url string) error {
	if err := bidi.Navigate(ctx, p.conn, p.contextID, url); err != nil {
		return err
	}
	// browsingContext.navigate already waits for the load, but the readyState
	// poll is what the rest of the engine treats as "ready" — a page that
	// finished loading during the navigate call passes it immediately.
	return p.WaitForLoad(ctx)
}

func (p *BiDiPage) EvalJS(ctx context.Context, expr string) ([]byte, error) {
	raw, err := p.eval(ctx, autoInvoke(expr))
	if err != nil {
		return nil, err
	}
	return marshalEvalResult(raw)
}

func (p *BiDiPage) CallProbe(ctx context.Context, fn string, arg any) ([]byte, error) {
	raw, err := bidi.CallFunction(ctx, p.conn, p.contextID, fn, arg)
	if err != nil {
		return nil, err
	}
	return marshalEvalResult(raw)
}

func (p *BiDiPage) Click(ctx context.Context, x, y float64) error {
	return bidi.Click(ctx, p.conn, p.contextID, x, y)
}

func (p *BiDiPage) Focus(ctx context.Context, id int, xpath string) error {
	_, err := p.eval(ctx, pagejs.Focus(id, xpath))
	return err
}

func (p *BiDiPage) SetInputValue(ctx context.Context, id int, xpath, value string) error {
	_, err := p.eval(ctx, pagejs.SetInputValue(id, xpath, value))
	return err
}

func (p *BiDiPage) SetChecked(ctx context.Context, id int, xpath string, checked bool) error {
	_, err := p.eval(ctx, pagejs.SetChecked(id, xpath, checked))
	return err
}

func (p *BiDiPage) ScrollIntoView(ctx context.Context, id int, xpath string) error {
	_, err := p.eval(ctx, pagejs.ScrollIntoView(id, xpath))
	return err
}

func (p *BiDiPage) ScrollPage(ctx context.Context, direction, container string) error {
	_, err := p.eval(ctx, pagejs.ScrollPage(direction, container))
	return err
}

func (p *BiDiPage) DoubleClick(ctx context.Context, x, y float64) error {
	return bidi.DoubleClick(ctx, p.conn, p.contextID, x, y)
}

func (p *BiDiPage) RightClick(ctx context.Context, x, y float64) error {
	return bidi.RightClick(ctx, p.conn, p.contextID, x, y)
}

func (p *BiDiPage) Hover(ctx context.Context, x, y float64) error {
	return bidi.Hover(ctx, p.conn, p.contextID, x, y)
}

func (p *BiDiPage) DragAndDrop(ctx context.Context, fromX, fromY, toX, toY float64) error {
	return bidi.DragAndDrop(ctx, p.conn, p.contextID, fromX, fromY, toX, toY)
}

// SetFileInput resolves the file input with the shared page JS and hands the
// node reference to input.setFiles — BiDi's counterpart of CDP's
// DOM.setFileInputFiles.
func (p *BiDiPage) SetFileInput(ctx context.Context, id int, xpath string, filePaths []string) error {
	sharedID, err := bidi.EvaluateNode(ctx, p.conn, p.contextID, pagejs.FileInput(id, xpath))
	if err != nil {
		return fmt.Errorf("SetFileInput: resolve file input: %w", err)
	}
	return bidi.SetFiles(ctx, p.conn, p.contextID, sharedID, filePaths)
}

func (p *BiDiPage) Screenshot(ctx context.Context) ([]byte, error) {
	return bidi.CaptureScreenshot(ctx, p.conn, p.contextID)
}

func (p *BiDiPage) WaitForResponse(ctx context.Context, urlPattern string, timeout time.Duration) error {
	return bidi.WaitForResponse(ctx, p.conn, p.contextID, urlPattern, timeout)
}

func (p *BiDiPage) HighlightElement(ctx context.Context, id int, xpath string, durationMS int) error {
	_, err := p.eval(ctx, pagejs.Highlight(id, xpath, durationMS))
	return err
}

func (p *BiDiPage) ClearHighlight(ctx context.Context) error {
	_, err := p.eval(ctx, pagejs.ClearHighlight())
	return err
}

func (p *BiDiPage) GetElementCenter(ctx context.Context, id int, xpath string) (float64, float64, error) {
	val, err := p.eval(ctx, pagejs.ElementCenter(id, xpath))
	if err != nil {
		return 0, 0, err
	}
	var coords struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := decodeJSONString(val, &coords); err != nil {
		return 0, 0, err
	}
	return coords.X, coords.Y, nil
}

func (p *BiDiPage) GetDragCenters(ctx context.Context, srcID int, srcXPath string, dstID int, dstXPath string) (float64, float64, float64, float64, error) {
	val, err := p.eval(ctx, pagejs.DragCenters(srcID, srcXPath, dstID, dstXPath))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	var coords struct {
		X1 float64 `json:"x1"`
		Y1 float64 `json:"y1"`
		X2 float64 `json:"x2"`
		Y2 float64 `json:"y2"`
	}
	if err := decodeJSONString(val, &coords); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("drag centres: %w", err)
	}
	return coords.X1, coords.Y1, coords.X2, coords.Y2, nil
}

// decodeJSONString unpacks the JSON.stringify results the coordinate probes in
// pkg/pagejs return: the value arrives as a JS string, not an object.
func decodeJSONString(val interface{}, out interface{}) error {
	s, ok := val.(string)
	if !ok {
		return fmt.Errorf("unexpected evaluate result format: %T", val)
	}
	return json.Unmarshal([]byte(s), out)
}

func (p *BiDiPage) DispatchKey(ctx context.Context, key string, modifiers int) error {
	return bidi.DispatchKey(ctx, p.conn, p.contextID, normalizeKeyName(key), modifiers)
}

func (p *BiDiPage) CurrentURL(ctx context.Context) (string, error) {
	val, err := p.eval(ctx, pagejs.CurrentURL)
	if err != nil {
		return "", err
	}
	if s, ok := val.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("unexpected evaluation result for URL: %v", val)
}

func (p *BiDiPage) WaitForLoad(ctx context.Context) error {
	const pollInterval = 150 * time.Millisecond
	for {
		raw, err := p.eval(ctx, pagejs.ReadyState)
		if err == nil && raw != nil {
			if state, ok := raw.(string); ok && state == "complete" {
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

func (p *BiDiPage) Wait(ctx context.Context, duration time.Duration) error {
	select {
	case <-time.After(duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Frames returns the main frame followed by its iframes, depth-first. BiDi
// gives every frame a browsing-context id of its own, so there is no realm
// bookkeeping to do — the tree is the answer.
func (p *BiDiPage) Frames(ctx context.Context) ([]FrameRef, error) {
	contexts, err := p.frameContexts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FrameRef, 0, len(contexts))
	for i, c := range contexts {
		// Name is left empty: getTree reports no frame name, and nothing in the
		// engine reads it — frames are addressed by index.
		out = append(out, FrameRef{Index: i, URL: c.URL})
	}
	return out, nil
}

func (p *BiDiPage) EvalJSInFrame(ctx context.Context, frameIndex int, expr string) ([]byte, error) {
	if frameIndex == 0 {
		return p.EvalJS(ctx, expr)
	}
	contextID, err := p.frameContextID(ctx, frameIndex)
	if err != nil {
		return nil, err
	}
	raw, err := bidi.Evaluate(ctx, p.conn, contextID, autoInvoke(expr))
	if err != nil {
		return nil, err
	}
	return marshalEvalResult(raw)
}

func (p *BiDiPage) CallProbeInFrame(ctx context.Context, frameIndex int, fn string, arg any) ([]byte, error) {
	if frameIndex == 0 {
		return p.CallProbe(ctx, fn, arg)
	}
	contextID, err := p.frameContextID(ctx, frameIndex)
	if err != nil {
		return nil, err
	}
	raw, err := bidi.CallFunction(ctx, p.conn, contextID, fn, arg)
	if err != nil {
		return nil, err
	}
	return marshalEvalResult(raw)
}

// frameContexts flattens the page's context tree depth-first, main frame first.
func (p *BiDiPage) frameContexts(ctx context.Context) ([]bidi.Context, error) {
	tree, err := bidi.GetTree(ctx, p.conn, p.contextID)
	if err != nil {
		return nil, err
	}
	var flat []bidi.Context
	var walk func(nodes []bidi.Context)
	walk = func(nodes []bidi.Context) {
		for _, n := range nodes {
			flat = append(flat, n)
			walk(n.Children)
		}
	}
	walk(tree)
	if len(flat) == 0 {
		return nil, fmt.Errorf("browser: context %s has no frames", p.contextID)
	}
	return flat, nil
}

// frameContextID resolves a frame index to its browsing-context id. The tree is
// re-read on every call: an iframe that navigated gets a new context, and a
// stale id evaluates into a document that no longer exists.
func (p *BiDiPage) frameContextID(ctx context.Context, frameIndex int) (string, error) {
	contexts, err := p.frameContexts(ctx)
	if err != nil {
		return "", err
	}
	if frameIndex < 0 || frameIndex >= len(contexts) {
		return "", fmt.Errorf("browser: frame index %d out of range (%d frames)", frameIndex, len(contexts))
	}
	return contexts[frameIndex].ID, nil
}

// Close releases the page handle. The connection stays open because it is
// shared with every other page of this browser; the tab itself is closed by
// CloseTarget, and the browser by Process.Close.
func (p *BiDiPage) Close() error { return nil }
