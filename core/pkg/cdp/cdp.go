// Package cdp provides a Chrome DevTools Protocol client for Manul Browser.
//
// This package implements the low-level WebSocket messenger and the
// command-level CDP calls (Navigate, Evaluate, Click, etc.) used by
// pkg/browser/cdp_backend.go.
package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
)

// ── Types ──────────────────────────────────────────────────────────────────────

// Target represents a single debuggable browser target (page, worker, etc.).
type Target struct {
	// ID is the unique target ID assigned by Chrome.
	ID string `json:"id"`
	// Type is the target type ("page", "background_page", "worker", etc.)
	Type string `json:"type"`
	// Title is the page title.
	Title string `json:"title"`
	// URL is the current URL of the target.
	URL string `json:"url"`
	// WSURL is the WebSocket debugger URL for this target.
	WSURL string `json:"webSocketDebuggerUrl"`
}

// KeyEventParams holds the browser input event parameters for keyboard dispatch.
type KeyEventParams struct {
	Type                  string `json:"type"`
	Key                   string `json:"key"`
	Code                  string `json:"code,omitempty"`
	WindowsVirtualKeyCode int    `json:"windowsVirtualKeyCode,omitempty"`
	Modifiers             int    `json:"modifiers,omitempty"`
	// Text/UnmodifiedText carry the character a keyDown produces. Chrome
	// treats a keyDown WITHOUT text as a rawKeyDown: no keypress event fires,
	// so Enter does not submit forms and printable keys type nothing. Senders
	// must set both for character-producing keys (Enter → "\r") and clear them
	// on the matching keyUp.
	Text           string `json:"text,omitempty"`
	UnmodifiedText string `json:"unmodifiedText,omitempty"`
}

// ── CDP Commands ───────────────────────────────────────────────────────────────

// Navigate instructs the browser to navigate to the given URL.
func Navigate(ctx context.Context, c *Conn, url string) error {
	_, err := c.Call(ctx, "Page.navigate", map[string]interface{}{"url": url})
	return err
}

// Evaluate runs JavaScript in the page context and returns the result.
func Evaluate(ctx context.Context, c *Conn, expression string) (interface{}, error) {
	res, err := c.Call(ctx, "Runtime.evaluate", map[string]interface{}{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
	})
	if err != nil {
		return nil, err
	}

	// {"result": {"type": "...", "value": ...}}
	var wrap struct {
		Result struct {
			Value interface{} `json:"value"`
			Type  string      `json:"type"`
		} `json:"result"`
		ExceptionDetails interface{} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(res, &wrap); err != nil {
		return nil, fmt.Errorf("unmarshal evaluate result: %w", err)
	}
	if wrap.ExceptionDetails != nil {
		return nil, fmt.Errorf("js exception: %v", wrap.ExceptionDetails)
	}
	return wrap.Result.Value, nil
}

// EvaluateInContext runs JavaScript in a specific execution context (frame).
// contextID == 0 falls back to the default/main context (plain Evaluate), so
// callers can pass a frame's context id unconditionally.
func EvaluateInContext(ctx context.Context, c *Conn, contextID int, expression string) (interface{}, error) {
	if contextID == 0 {
		return Evaluate(ctx, c, expression)
	}
	res, err := c.Call(ctx, "Runtime.evaluate", map[string]interface{}{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
		"contextId":     contextID,
	})
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Result struct {
			Value interface{} `json:"value"`
		} `json:"result"`
		ExceptionDetails interface{} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(res, &wrap); err != nil {
		return nil, fmt.Errorf("unmarshal evaluate result: %w", err)
	}
	if wrap.ExceptionDetails != nil {
		return nil, fmt.Errorf("js exception: %v", wrap.ExceptionDetails)
	}
	return wrap.Result.Value, nil
}

// CallFunctionOn calls a JS function string with a JSON-serialized argument.
func CallFunctionOn(ctx context.Context, c *Conn, objectId string, arg interface{}) (interface{}, error) {
	// objectId is the function source; it is evaluated (and, with an arg,
	// invoked) in the default execution context.
	var expr string
	if arg == nil {
		expr = objectId
	} else {
		expr = fmt.Sprintf("(%s)(%s)", objectId, MustMarshalString(arg))
	}
	res, err := c.Call(ctx, "Runtime.evaluate", map[string]interface{}{
		"expression":    expr,
		"returnByValue": true,
		"awaitPromise":  true,
	})
	if err == nil {
		// Just parse Evaluate wrapper
		var wrap struct {
			Result struct {
				Value interface{} `json:"value"`
			} `json:"result"`
			ExceptionDetails interface{} `json:"exceptionDetails"`
		}
		if json.Unmarshal(res, &wrap) == nil {
			if wrap.ExceptionDetails != nil {
				return nil, fmt.Errorf("js exception: %v", wrap.ExceptionDetails)
			}
			return wrap.Result.Value, nil
		}
	}

	return nil, err
}

func MustMarshalString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return "undefined"
	}
	return string(b)
}

func evaluateObjectID(ctx context.Context, c *Conn, expression string) (string, error) {
	res, err := c.Call(ctx, "Runtime.evaluate", map[string]interface{}{
		"expression":    expression,
		"returnByValue": false,
		"awaitPromise":  true,
	})
	if err != nil {
		return "", err
	}

	var wrap struct {
		Result struct {
			ObjectID string `json:"objectId"`
		} `json:"result"`
		ExceptionDetails interface{} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(res, &wrap); err != nil {
		return "", fmt.Errorf("unmarshal evaluate handle result: %w", err)
	}
	if wrap.ExceptionDetails != nil {
		return "", fmt.Errorf("js exception: %v", wrap.ExceptionDetails)
	}
	if wrap.Result.ObjectID == "" {
		return "", fmt.Errorf("expression did not resolve to a remote object")
	}
	return wrap.Result.ObjectID, nil
}

// Click dispatches a mouse click at the given page coordinates.
func Click(ctx context.Context, c *Conn, x, y float64) error {
	// MousePressed
	_, err := c.Call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type":       "mousePressed",
		"button":     "left",
		"x":          x,
		"y":          y,
		"clickCount": 1,
	})
	if err != nil {
		return err
	}
	// MouseReleased
	_, err = c.Call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type":       "mouseReleased",
		"button":     "left",
		"x":          x,
		"y":          y,
		"clickCount": 1,
	})
	return err
}

// DoubleClick dispatches a double-click at the given page coordinates.
func DoubleClick(ctx context.Context, c *Conn, x, y float64) error {
	// Press 1
	_, err := c.Call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type": "mousePressed", "button": "left", "x": x, "y": y, "clickCount": 1,
	})
	if err != nil {
		return err
	}
	_, err = c.Call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type": "mouseReleased", "button": "left", "x": x, "y": y, "clickCount": 1,
	})
	if err != nil {
		return err
	}

	// Press 2
	_, err = c.Call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type": "mousePressed", "button": "left", "x": x, "y": y, "clickCount": 2,
	})
	if err != nil {
		return err
	}
	_, err = c.Call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type": "mouseReleased", "button": "left", "x": x, "y": y, "clickCount": 2,
	})
	return err
}

// RightClick dispatches a right-click (contextmenu) at the given page coordinates.
func RightClick(ctx context.Context, c *Conn, x, y float64) error {
	_, err := c.Call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type":       "mousePressed",
		"button":     "right",
		"x":          x,
		"y":          y,
		"clickCount": 1,
	})
	if err != nil {
		return err
	}
	_, err = c.Call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type":       "mouseReleased",
		"button":     "right",
		"x":          x,
		"y":          y,
		"clickCount": 1,
	})
	return err
}

// Hover dispatches a mousemove event at the given page coordinates.
func Hover(ctx context.Context, c *Conn, x, y float64) error {
	_, err := c.Call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type": "mouseMoved",
		"x":    x,
		"y":    y,
	})
	return err
}

// dragSteps is how many intermediate moves a drag is broken into.
//
// One jump from source to target is not a drag as far as the page is
// concerned. jQuery UI and every HTML5 drop library start their drag on the
// first mousemove *after* mousedown, track the pointer across subsequent ones,
// and decide what is under it when the button is released — a lone move gives
// them nothing to track and frequently never enters the drag state at all.
const dragSteps = 10

// DragAndDrop dispatches a drag-and-drop sequence.
//
// Two details make the difference between this working and silently doing
// nothing. Every move between press and release carries `buttons: 1`: that
// field is the pointer state the page reads as `event.buttons`, and a page that
// sees 0 concludes the button was let go and cancels the drag it had started.
// And the pointer travels in steps rather than teleporting, for the reason
// above. Both are invisible from the outside — the CDP calls all succeed, the
// step passes, and only the verification afterwards notices nothing moved.
func DragAndDrop(ctx context.Context, c *Conn, fromX, fromY, toX, toY float64) error {
	move := func(x, y float64, buttons int) error {
		_, err := c.Call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
			"type": "mouseMoved", "button": "left", "buttons": buttons, "x": x, "y": y,
		})
		return err
	}

	// Settle the pointer on the source before pressing, so anything listening
	// for hover has already reacted.
	if err := move(fromX, fromY, 0); err != nil {
		return err
	}

	if _, err := c.Call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type": "mousePressed", "button": "left", "buttons": 1,
		"x": fromX, "y": fromY, "clickCount": 1,
	}); err != nil {
		return err
	}

	// Interpolate. The first step is deliberately a small one off the source:
	// drag implementations have a distance threshold before they engage, and a
	// single large jump can satisfy the threshold and the drop in the same
	// event, which some of them treat as neither.
	for i := 1; i <= dragSteps; i++ {
		p := float64(i) / float64(dragSteps)
		if err := move(fromX+(toX-fromX)*p, fromY+(toY-fromY)*p, 1); err != nil {
			return err
		}
	}

	// One more move at rest on the target: a droppable decides what it is over
	// on a move, not on the release.
	if err := move(toX, toY, 1); err != nil {
		return err
	}

	_, err := c.Call(ctx, "Input.dispatchMouseEvent", map[string]interface{}{
		"type": "mouseReleased", "button": "left", "buttons": 0,
		"x": toX, "y": toY, "clickCount": 1,
	})
	return err
}

// Focus focuses the element resolved by ID or XPath.
func (c *Conn) Focus(ctx context.Context, id int, xpath string) error {
	_, err := Evaluate(ctx, c, pagejs.Focus(id, xpath))
	return err
}

// SetInputValue sets the value of an input element resolved by ID or XPath.
// Uses the native HTMLInputElement/HTMLTextAreaElement value setter to
// bypass framework-level overrides (React, Vue, etc.) that intercept
// the value property on individual elements.
func (c *Conn) SetInputValue(ctx context.Context, id int, xpath, value string) error {
	_, err := Evaluate(ctx, c, pagejs.SetInputValue(id, xpath, value))
	return err
}

func (c *Conn) SetChecked(ctx context.Context, id int, xpath string, checked bool) error {
	_, err := Evaluate(ctx, c, pagejs.SetChecked(id, xpath, checked))
	return err
}

// GetDragCenters returns the viewport centres of two elements, measured after
// one scroll rather than two.
//
// GetElementCenter scrolls its element into view before measuring, which makes
// it wrong to call twice for a drag: the second call scrolls to the target and
// moves the source out from under the coordinates the first call just returned.
// The press then lands wherever the source used to be. Nothing errors — the
// coordinates are real, they are simply stale — so the step passes and only a
// later verification notices nothing was dragged.
//
// This scrolls once, to the midpoint of the two, and measures both afterwards.
// Elements far enough apart that both cannot share a viewport are still beyond
// what a coordinate drag can express, but those are rare; a drag source and its
// drop target are near each other by construction.
func (c *Conn) GetDragCenters(ctx context.Context, srcID int, srcXPath string, dstID int, dstXPath string) (x1, y1, x2, y2 float64, err error) {
	val, err := Evaluate(ctx, c, pagejs.DragCenters(srcID, srcXPath, dstID, dstXPath))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	var coords struct {
		X1 float64 `json:"x1"`
		Y1 float64 `json:"y1"`
		X2 float64 `json:"x2"`
		Y2 float64 `json:"y2"`
	}
	// JSON.stringify means the result arrives as a string, as it does for
	// GetElementCenter.
	str, ok := val.(string)
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("unexpected evaluate result format: %T", val)
	}
	if err := json.Unmarshal([]byte(str), &coords); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("drag centres: %w", err)
	}
	return coords.X1, coords.Y1, coords.X2, coords.Y2, nil
}

// ScrollIntoView scrolls the element resolved by ID or XPath into the viewport.
func (c *Conn) ScrollIntoView(ctx context.Context, id int, xpath string) error {
	_, err := Evaluate(ctx, c, pagejs.ScrollIntoView(id, xpath))
	return err
}

// ScrollPage scrolls the page or a container element in the given direction.
// Currently only direction="up" or "down" are well supported.
func ScrollPage(ctx context.Context, c *Conn, direction, container string) error {
	_, err := Evaluate(ctx, c, pagejs.ScrollPage(direction, container))
	return err
}

// SetFileInput sets the file paths on a file input element resolved by ID or XPath.
func (c *Conn) SetFileInput(ctx context.Context, id int, xpath string, filePaths []string) error {
	objectID, err := evaluateObjectID(ctx, c, pagejs.FileInput(id, xpath))
	if err != nil {
		return fmt.Errorf("SetFileInput: resolve file input: %w", err)
	}

	// Get the backend node ID
	rawRes, err := c.Call(ctx, "DOM.requestNode", map[string]interface{}{
		"objectId": objectID,
	})
	if err != nil {
		return err
	}

	var res struct {
		NodeId int `json:"nodeId"`
	}
	if err := json.Unmarshal(rawRes, &res); err != nil {
		return fmt.Errorf("SetFileInput: unmarshal requestNode: %w", err)
	}

	_, err = c.Call(ctx, "DOM.setFileInputFiles", map[string]interface{}{
		"nodeId": res.NodeId,
		"files":  filePaths,
	})
	return err
}

// Screenshot captures a PNG screenshot of the current viewport.
func Screenshot(ctx context.Context, c *Conn) ([]byte, error) {
	res, err := c.Call(ctx, "Page.captureScreenshot", map[string]interface{}{
		"format": "png",
	})
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Data []byte `json:"data"` // base64 encoded by chrome, auto-decoded by Go's []byte unmarshal!
	}
	if err := json.Unmarshal(res, &wrap); err != nil {
		return nil, fmt.Errorf("unmarshal screenshot: %w", err)
	}
	return wrap.Data, nil
}

// WaitForResponse waits for a network response whose URL matches the given pattern.
func WaitForResponse(ctx context.Context, c *Conn, urlPattern string, timeout time.Duration) error {
	// Enable network tracking first
	_, err := c.Call(ctx, "Network.enable", nil)
	if err != nil {
		return fmt.Errorf("Network.enable: %w", err)
	}

	sub := c.Subscribe()
	defer sub.Close()
	defer func() {
		// Bound the cleanup call so a dead/stuck socket cannot hang the worker forever.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = c.Call(ctx, "Network.disable", nil)
	}()

	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctxTimeout.Done():
			return fmt.Errorf("timeout waiting for response pattern %q", urlPattern)
		case event, ok := <-sub.C():
			if !ok {
				return fmt.Errorf("cdp connection closed while waiting for response pattern %q", urlPattern)
			}
			if event.Method == "Network.responseReceived" {
				var received struct {
					Response struct {
						URL string `json:"url"`
					} `json:"response"`
				}
				if err := json.Unmarshal(event.Params, &received); err == nil {
					// Extremely simple suffix/substring match
					if len(received.Response.URL) >= len(urlPattern) &&
						received.Response.URL[len(received.Response.URL)-len(urlPattern):] == urlPattern {
						return nil
					}
				}
			}
		}
	}
}

// HighlightElement injects a temporary border highlight for debugging.
// 4px solid red border + #ffeb3b background.
func (c *Conn) HighlightElement(ctx context.Context, id int, xpath string, durationMS int) error {
	_, err := Evaluate(ctx, c, pagejs.Highlight(id, xpath, durationMS))
	return err
}

// ClearHighlight immediately removes any active highlight from the page.
// It restores original inline styles for flash highlights and removes debug
// highlight attributes. Errors are swallowed because the page may have
// navigated and destroyed the execution context.
func (c *Conn) ClearHighlight(ctx context.Context) error {
	_, err := Evaluate(ctx, c, pagejs.ClearHighlight())
	return err
}

// GetElementCenter returns the centre coordinates of an element resolved by ID or XPath.
func (c *Conn) GetElementCenter(ctx context.Context, id int, xpath string) (x, y float64, err error) {
	val, err := Evaluate(ctx, c, pagejs.ElementCenter(id, xpath))
	if err != nil {
		return 0, 0, err
	}

	var coords struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}

	// val should be a string containing JSON due to JSON.stringify
	str, ok := val.(string)
	if !ok {
		return 0, 0, fmt.Errorf("unexpected evaluate result format: %T", val)
	}
	if err := json.Unmarshal([]byte(str), &coords); err != nil {
		return 0, 0, err
	}
	return coords.X, coords.Y, nil
}

// DispatchKeyEvent sends a keyboard event to the currently focused element.
// eventType is one of "keyDown" / "keyUp" / "rawKeyDown" / "char" — it
// overrides params.Type so callers don't have to set it on both ends of
// a key-press pair.
func DispatchKeyEvent(ctx context.Context, c *Conn, eventType string, params KeyEventParams) error {
	params.Type = eventType
	_, err := c.Call(ctx, "Input.dispatchKeyEvent", params)
	return err
}

// GetCurrentURL returns the current URL of the page.
func GetCurrentURL(ctx context.Context, c *Conn) (string, error) {
	val, err := Evaluate(ctx, c, pagejs.CurrentURL)
	if err != nil {
		return "", err
	}
	if s, ok := val.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("unexpected evaluation result for URL: %v", val)
}

// WaitForLoad is available but Manul Browser prefers JS-polling WaitForLoad
// in cdp_backend.go to avoid race conditions on cached pages.
func WaitForLoad(ctx context.Context, c *Conn) error {
	return nil // Handled in cdp_backend.go
}

// ── JSON helpers ───────────────────────────────────────────────────────────────

// MustMarshal marshals v to JSON, panicking on error (used in tests only).
func MustMarshal(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
