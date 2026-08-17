package bidi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

// ── Types ─────────────────────────────────────────────────────────────────────

// Context is one browsing context: a tab, or an iframe inside one. ID is what
// every page-scoped command is addressed with — BiDi's equivalent of a CDP
// target id, except that iframes have one too, which is what makes per-frame
// evaluation a matter of swapping the id rather than tracking realms.
type Context struct {
	ID       string    `json:"context"`
	URL      string    `json:"url"`
	Children []Context `json:"children"`
}

// ── Connection setup ──────────────────────────────────────────────────────────

// SessionPath is where a BiDi endpoint accepts the WebSocket upgrade.
const SessionPath = "/session"

// WebSocketURL resolves endpoint to a BiDi WebSocket URL.
//
// A ws:// or wss:// endpoint is taken as given, bar the session path. Anything
// else is treated as an HTTP endpoint (or a bare host:port) and probed for
// /json/version — Chromium-style discovery that a Firefox with the remote
// agent enabled no longer answers — before falling back to ws://host/session,
// which is where Firefox listens.
func WebSocketURL(ctx context.Context, endpoint string) (string, error) {
	e := strings.TrimSpace(endpoint)
	if e == "" {
		return "", fmt.Errorf("bidi: empty endpoint")
	}
	if strings.HasPrefix(e, "ws://") || strings.HasPrefix(e, "wss://") {
		return NormalizeWebSocketURL(e), nil
	}
	base := e
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	u, err := neturl.Parse(base)
	if err != nil {
		return "", fmt.Errorf("bidi: parse endpoint %q: %w", endpoint, err)
	}
	if ws := probeWebSocketURL(ctx, strings.TrimSuffix(base, "/")+"/json/version"); ws != "" {
		return NormalizeWebSocketURL(ws), nil
	}
	return "ws://" + u.Host + SessionPath, nil
}

// NormalizeWebSocketURL appends the session path to a socket URL that has
// none.
//
// Firefox announces its endpoint as a bare origin — "WebDriver BiDi listening
// on ws://127.0.0.1:9222" — but the upgrade only succeeds at /session. A URL
// taken from that banner, or pasted from it by a user attaching to a browser
// they started themselves, would otherwise fail the handshake with nothing to
// explain why.
func NormalizeWebSocketURL(raw string) string {
	u, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = SessionPath
	}
	return u.String()
}

// probeWebSocketURL returns the webSocketDebuggerUrl advertised at url, or ""
// when the endpoint does not answer or does not advertise one.
func probeWebSocketURL(ctx context.Context, url string) string {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ""
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var v struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return ""
	}
	return v.WebSocketDebuggerURL
}

// Connect resolves endpoint, dials it and creates a WebDriver session, leaving
// the connection ready for browsing-context commands.
func Connect(ctx context.Context, endpoint string) (*Conn, error) {
	wsURL, err := WebSocketURL(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	c, err := Dial(ctx, wsURL)
	if err != nil {
		return nil, err
	}
	if err := NewSession(ctx, c); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// NewSession performs the session.new handshake. Every command other than
// session.status is refused until it has run.
func NewSession(ctx context.Context, c *Conn) error {
	raw, err := c.Call(ctx, "session.new", map[string]interface{}{
		"capabilities": map[string]interface{}{},
	})
	if err != nil {
		return fmt.Errorf("bidi: session.new: %w", err)
	}
	var res struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return fmt.Errorf("bidi: decode session.new result: %w", err)
	}
	c.sessionID = res.SessionID
	return nil
}

// Ready reports whether the remote end answers session.status. It is the
// cheapest liveness probe in the protocol and needs no session.
func Ready(ctx context.Context, c *Conn) error {
	_, err := c.Call(ctx, "session.status", map[string]interface{}{})
	return err
}

// SubscribeEvents asks the browser to start sending the named events. BiDi
// sends no events until this is called — a local Conn.Subscribe alone stays
// silent. Passing contexts narrows the subscription to those pages.
func SubscribeEvents(ctx context.Context, c *Conn, events []string, contexts []string) error {
	params := map[string]interface{}{"events": events}
	if len(contexts) > 0 {
		params["contexts"] = contexts
	}
	_, err := c.Call(ctx, "session.subscribe", params)
	return err
}

// UnsubscribeEvents stops the browser sending the named events.
func UnsubscribeEvents(ctx context.Context, c *Conn, events []string) error {
	_, err := c.Call(ctx, "session.unsubscribe", map[string]interface{}{"events": events})
	return err
}

// ── Browsing contexts ─────────────────────────────────────────────────────────

// GetTree returns the browsing-context tree. An empty root returns every
// top-level context (one per tab); a context id returns that page's own tree,
// whose Children are its iframes.
func GetTree(ctx context.Context, c *Conn, root string) ([]Context, error) {
	params := map[string]interface{}{}
	if root != "" {
		params["root"] = root
	}
	raw, err := c.Call(ctx, "browsingContext.getTree", params)
	if err != nil {
		return nil, err
	}
	var res struct {
		Contexts []Context `json:"contexts"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("bidi: decode getTree result: %w", err)
	}
	return res.Contexts, nil
}

// CreateContext opens a new tab and returns its context id.
func CreateContext(ctx context.Context, c *Conn) (string, error) {
	raw, err := c.Call(ctx, "browsingContext.create", map[string]interface{}{"type": "tab"})
	if err != nil {
		return "", err
	}
	var res struct {
		Context string `json:"context"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("bidi: decode create result: %w", err)
	}
	if res.Context == "" {
		return "", fmt.Errorf("bidi: browsingContext.create returned no context")
	}
	return res.Context, nil
}

// CloseContext closes the tab identified by contextID.
func CloseContext(ctx context.Context, c *Conn, contextID string) error {
	_, err := c.Call(ctx, "browsingContext.close", map[string]interface{}{"context": contextID})
	return err
}

// Navigate loads url in the given context and waits for the load to complete.
func Navigate(ctx context.Context, c *Conn, contextID, url string) error {
	_, err := c.Call(ctx, "browsingContext.navigate", map[string]interface{}{
		"context": contextID,
		"url":     url,
		"wait":    "complete",
	})
	return err
}

// CaptureScreenshot returns a PNG of the context's viewport.
func CaptureScreenshot(ctx context.Context, c *Conn, contextID string) ([]byte, error) {
	raw, err := c.Call(ctx, "browsingContext.captureScreenshot", map[string]interface{}{
		"context": contextID,
		"origin":  "viewport",
		"format":  map[string]interface{}{"type": "image/png"},
	})
	if err != nil {
		return nil, err
	}
	var res struct {
		Data []byte `json:"data"` // base64 on the wire, decoded by encoding/json
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("bidi: decode screenshot: %w", err)
	}
	return res.Data, nil
}

// ── Script evaluation ─────────────────────────────────────────────────────────

// Evaluate runs a script in the given context and returns its completion
// value, decoded to the same Go shapes pkg/cdp returns: string, float64, bool,
// []interface{}, map[string]interface{} or nil.
//
// Statements are allowed — the value is the script's completion value, exactly
// as with CDP's Runtime.evaluate — which is what lets pkg/pagejs serve both
// backends unchanged.
func Evaluate(ctx context.Context, c *Conn, contextID, expression string) (interface{}, error) {
	raw, err := c.Call(ctx, "script.evaluate", evalParams(contextID, expression))
	if err != nil {
		return nil, err
	}
	return decodeScriptResult(raw)
}

// CallFunction calls a serialized JS function with a JSON argument. It builds
// the same `(fn)(arg)` expression pkg/cdp does, so a probe behaves identically
// on both backends.
func CallFunction(ctx context.Context, c *Conn, contextID, fn string, arg interface{}) (interface{}, error) {
	expr := fn
	if arg != nil {
		expr = fmt.Sprintf("(%s)(%s)", fn, MustMarshalString(arg))
	}
	return Evaluate(ctx, c, contextID, expr)
}

// EvaluateNode runs expression and returns the shared reference of the node it
// produced. Node references are how input.setFiles addresses an element.
func EvaluateNode(ctx context.Context, c *Conn, contextID, expression string) (string, error) {
	raw, err := c.Call(ctx, "script.evaluate", evalParams(contextID, expression))
	if err != nil {
		return "", err
	}
	var res struct {
		Type             string          `json:"type"`
		Result           remoteValue     `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("bidi: decode evaluate result: %w", err)
	}
	if res.Type == "exception" {
		return "", fmt.Errorf("js exception: %s", exceptionText(res.ExceptionDetails))
	}
	if res.Result.SharedID == "" {
		return "", fmt.Errorf("bidi: expression did not resolve to a node")
	}
	return res.Result.SharedID, nil
}

// SetFiles sets the file list of a file input identified by a shared node
// reference.
func SetFiles(ctx context.Context, c *Conn, contextID, sharedID string, files []string) error {
	_, err := c.Call(ctx, "input.setFiles", map[string]interface{}{
		"context": contextID,
		"element": map[string]interface{}{"sharedId": sharedID},
		"files":   files,
	})
	return err
}

func evalParams(contextID, expression string) map[string]interface{} {
	return map[string]interface{}{
		"expression":   expression,
		"target":       map[string]interface{}{"context": contextID},
		"awaitPromise": true,
		// The engine's probes read the DOM and return plain data; serializing
		// node trees on top of that would be pure weight, so DOM depth is 0.
		"serializationOptions": map[string]interface{}{"maxDomDepth": 0},
		// Treat evaluation as a user gesture, so page code gated behind one
		// (media playback, clipboard) behaves as it does under a real click.
		"userActivation": true,
	}
}

func decodeScriptResult(raw json.RawMessage) (interface{}, error) {
	var res struct {
		Type             string          `json:"type"`
		Result           json.RawMessage `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("bidi: decode evaluate result: %w", err)
	}
	if res.Type == "exception" {
		return nil, fmt.Errorf("js exception: %s", exceptionText(res.ExceptionDetails))
	}
	return decodeRemoteValue(res.Result)
}

func exceptionText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "unknown"
	}
	var d struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &d); err == nil && d.Text != "" {
		return d.Text
	}
	return string(raw)
}

// MustMarshalString marshals v for embedding in a JS expression, mirroring
// cdp.MustMarshalString: an unmarshalable or null value becomes `undefined`.
func MustMarshalString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return "undefined"
	}
	return string(b)
}

// ── Input ─────────────────────────────────────────────────────────────────────

const (
	mouseSourceID = "manul-mouse"
	keySourceID   = "manul-keys"
)

// PerformActions dispatches a list of BiDi input source actions.
func PerformActions(ctx context.Context, c *Conn, contextID string, actions []interface{}) error {
	_, err := c.Call(ctx, "input.performActions", map[string]interface{}{
		"context": contextID,
		"actions": actions,
	})
	return err
}

// ReleaseActions resets input state (held buttons and modifier keys) for the
// context. BiDi keeps that state between performActions calls, so a sequence
// that pressed a modifier must let it go or every later key carries it.
func ReleaseActions(ctx context.Context, c *Conn, contextID string) error {
	_, err := c.Call(ctx, "input.releaseActions", map[string]interface{}{"context": contextID})
	return err
}

// pointerSource wraps pointer actions in the source envelope performActions expects.
func pointerSource(actions ...interface{}) []interface{} {
	return []interface{}{map[string]interface{}{
		"type":       "pointer",
		"id":         mouseSourceID,
		"parameters": map[string]interface{}{"pointerType": "mouse"},
		"actions":    actions,
	}}
}

// pointerMove is a move to viewport coordinates. BiDi takes integer CSS
// pixels, so fractional centres are rounded on the way out.
func pointerMove(x, y float64) map[string]interface{} {
	return map[string]interface{}{
		"type":   "pointerMove",
		"x":      int(x + 0.5),
		"y":      int(y + 0.5),
		"origin": "viewport",
	}
}

func pointerDown(button int) map[string]interface{} {
	return map[string]interface{}{"type": "pointerDown", "button": button}
}

func pointerUp(button int) map[string]interface{} {
	return map[string]interface{}{"type": "pointerUp", "button": button}
}

// Click dispatches a left click at the given viewport coordinates.
func Click(ctx context.Context, c *Conn, contextID string, x, y float64) error {
	return PerformActions(ctx, c, contextID, pointerSource(
		pointerMove(x, y), pointerDown(0), pointerUp(0),
	))
}

// DoubleClick dispatches two left clicks in one action sequence, which is what
// makes the page see a dblclick rather than two unrelated clicks.
func DoubleClick(ctx context.Context, c *Conn, contextID string, x, y float64) error {
	return PerformActions(ctx, c, contextID, pointerSource(
		pointerMove(x, y), pointerDown(0), pointerUp(0), pointerDown(0), pointerUp(0),
	))
}

// RightClick dispatches a context-menu click.
func RightClick(ctx context.Context, c *Conn, contextID string, x, y float64) error {
	return PerformActions(ctx, c, contextID, pointerSource(
		pointerMove(x, y), pointerDown(2), pointerUp(2),
	))
}

// Hover moves the pointer without pressing.
func Hover(ctx context.Context, c *Conn, contextID string, x, y float64) error {
	return PerformActions(ctx, c, contextID, pointerSource(pointerMove(x, y)))
}

// dragSteps is how many intermediate moves a drag is broken into, for the same
// reason the CDP backend interpolates: a drag library starts on the first move
// after the press and tracks the pointer across the ones that follow, so a
// single jump frequently never enters the drag state at all.
const dragSteps = 10

// DragAndDrop presses at the source, walks the pointer to the target in steps
// and releases there.
func DragAndDrop(ctx context.Context, c *Conn, contextID string, fromX, fromY, toX, toY float64) error {
	actions := []interface{}{pointerMove(fromX, fromY), pointerDown(0)}
	for i := 1; i <= dragSteps; i++ {
		p := float64(i) / float64(dragSteps)
		actions = append(actions, pointerMove(fromX+(toX-fromX)*p, fromY+(toY-fromY)*p))
	}
	// One more move at rest on the target: a droppable decides what it is over
	// on a move, not on the release.
	actions = append(actions, pointerMove(toX, toY), pointerUp(0))
	return PerformActions(ctx, c, contextID, pointerSource(actions...))
}

// WebDriver modifier key values, indexed by the engine's modifier bitmask.
const (
	keyAlt   = "\ue00a"
	keyCtrl  = "\ue009"
	keyMeta  = "\ue03d"
	keyShift = "\ue008"
)

// webDriverKeys maps the canonical DOM key names the engine uses to the
// WebDriver private-use code points BiDi expects. Printable single characters
// are sent as themselves and are not listed here.
var webDriverKeys = map[string]string{
	"Enter":      "\ue007",
	"Tab":        "\ue004",
	"Escape":     "\ue00c",
	"Backspace":  "\ue003",
	"Delete":     "\ue017",
	"Insert":     "\ue016",
	"ArrowLeft":  "\ue012",
	"ArrowUp":    "\ue013",
	"ArrowRight": "\ue014",
	"ArrowDown":  "\ue015",
	"Home":       "\ue011",
	"End":        "\ue010",
	"PageUp":     "\ue00e",
	"PageDown":   "\ue00f",
	"F1":         "\ue031",
	"F2":         "\ue032",
	"F3":         "\ue033",
	"F4":         "\ue034",
	"F5":         "\ue035",
	"F6":         "\ue036",
	"F7":         "\ue037",
	"F8":         "\ue038",
	"F9":         "\ue039",
	"F10":        "\ue03a",
	"F11":        "\ue03b",
	"F12":        "\ue03c",
}

// KeyValue returns the WebDriver key value for a canonical DOM key name.
// Unknown names are sent verbatim, which is correct for printable characters.
func KeyValue(key string) string {
	if v, ok := webDriverKeys[key]; ok {
		return v
	}
	return key
}

// modifierKeys returns the WebDriver key values for a modifier bitmask
// (1=Alt, 2=Ctrl, 4=Meta, 8=Shift), in a stable order.
func modifierKeys(modifiers int) []string {
	var keys []string
	if modifiers&2 != 0 {
		keys = append(keys, keyCtrl)
	}
	if modifiers&1 != 0 {
		keys = append(keys, keyAlt)
	}
	if modifiers&8 != 0 {
		keys = append(keys, keyShift)
	}
	if modifiers&4 != 0 {
		keys = append(keys, keyMeta)
	}
	return keys
}

// DispatchKey presses and releases key with the given modifier bitmask
// (1=Alt, 2=Ctrl, 4=Meta, 8=Shift). Modifiers are pressed around the key and
// released in reverse order, so nothing stays held afterwards.
func DispatchKey(ctx context.Context, c *Conn, contextID, key string, modifiers int) error {
	mods := modifierKeys(modifiers)
	actions := make([]interface{}, 0, len(mods)*2+2)
	for _, m := range mods {
		actions = append(actions, map[string]interface{}{"type": "keyDown", "value": m})
	}
	value := KeyValue(key)
	actions = append(actions,
		map[string]interface{}{"type": "keyDown", "value": value},
		map[string]interface{}{"type": "keyUp", "value": value},
	)
	for i := len(mods) - 1; i >= 0; i-- {
		actions = append(actions, map[string]interface{}{"type": "keyUp", "value": mods[i]})
	}
	err := PerformActions(ctx, c, contextID, []interface{}{map[string]interface{}{
		"type":    "key",
		"id":      keySourceID,
		"actions": actions,
	}})
	if err != nil {
		return err
	}
	// Drop any input state the sequence left behind; a modifier that stays
	// held would silently ride along on every later key.
	return ReleaseActions(ctx, c, contextID)
}

// ── Network ───────────────────────────────────────────────────────────────────

// WaitForResponse waits for a network response whose URL ends with urlPattern,
// matching the CDP backend's suffix semantics.
func WaitForResponse(ctx context.Context, c *Conn, contextID, urlPattern string, timeout time.Duration) error {
	const event = "network.responseCompleted"
	// Open the local channel before asking the browser to send anything: a
	// response that completes between the two would otherwise arrive with
	// nowhere to go, and the wait would sit through a request it was watching
	// for.
	sub := c.Subscribe()
	defer sub.Close()
	if err := SubscribeEvents(ctx, c, []string{event}, []string{contextID}); err != nil {
		return fmt.Errorf("bidi: subscribe %s: %w", event, err)
	}
	defer func() {
		// Bound the cleanup call so a dead socket cannot hang the caller.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = UnsubscribeEvents(ctx, c, []string{event})
	}()

	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctxTimeout.Done():
			return fmt.Errorf("timeout waiting for response pattern %q", urlPattern)
		case ev, ok := <-sub.C():
			if !ok {
				return fmt.Errorf("bidi connection closed while waiting for response pattern %q", urlPattern)
			}
			if ev.Method != event {
				continue
			}
			var received struct {
				Response struct {
					URL string `json:"url"`
				} `json:"response"`
			}
			if err := json.Unmarshal(ev.Params, &received); err != nil {
				continue
			}
			if strings.HasSuffix(received.Response.URL, urlPattern) {
				return nil
			}
		}
	}
}

// ── Remote value decoding ─────────────────────────────────────────────────────

// remoteValue is BiDi's serialized JS value. Unlike CDP's returnByValue, which
// hands back plain JSON, every value is tagged with its type and containers
// carry their contents as nested remote values.
type remoteValue struct {
	Type     string          `json:"type"`
	Value    json.RawMessage `json:"value"`
	SharedID string          `json:"sharedId"`
	Handle   string          `json:"handle"`
}

func decodeRemoteValue(raw json.RawMessage) (interface{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var rv remoteValue
	if err := json.Unmarshal(raw, &rv); err != nil {
		return nil, fmt.Errorf("bidi: decode remote value: %w", err)
	}
	return rv.decode()
}

func (rv remoteValue) decode() (interface{}, error) {
	switch rv.Type {
	case "undefined", "null":
		return nil, nil

	case "string", "bigint", "date":
		var s string
		if err := json.Unmarshal(rv.Value, &s); err != nil {
			return nil, nil
		}
		return s, nil

	case "boolean":
		var b bool
		if err := json.Unmarshal(rv.Value, &b); err != nil {
			return nil, nil
		}
		return b, nil

	case "number":
		// NaN, Infinity, -Infinity and -0 arrive as strings, since JSON cannot
		// carry them. None survives a round trip back to JSON either, and CDP
		// drops them the same way (unserializableValue, no value), so nil keeps
		// the two backends in step.
		var f float64
		if err := json.Unmarshal(rv.Value, &f); err != nil {
			return nil, nil
		}
		return f, nil

	case "array", "set", "nodelist", "htmlcollection":
		var items []json.RawMessage
		if err := json.Unmarshal(rv.Value, &items); err != nil {
			// A container serialized past the depth limit has no value at all.
			return []interface{}{}, nil
		}
		out := make([]interface{}, 0, len(items))
		for _, item := range items {
			v, err := decodeRemoteValue(item)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil

	case "object", "map":
		var pairs [][2]json.RawMessage
		if err := json.Unmarshal(rv.Value, &pairs); err != nil {
			return map[string]interface{}{}, nil
		}
		out := make(map[string]interface{}, len(pairs))
		for _, pair := range pairs {
			key, err := decodeKey(pair[0])
			if err != nil {
				continue
			}
			v, err := decodeRemoteValue(pair[1])
			if err != nil {
				return nil, err
			}
			out[key] = v
		}
		return out, nil

	case "node":
		// Nodes are not data. The engine reads the DOM through probes and only
		// ever needs a node itself to hand to input.setFiles, which takes the
		// shared reference — so that is what survives the decode.
		return map[string]interface{}{"sharedId": rv.SharedID}, nil

	default:
		// function, symbol, error, promise, proxy, window, typedarray… none of
		// which the engine's probes return.
		return nil, nil
	}
}

// decodeKey reads an object key, which is a bare string for plain objects and
// a remote value for Map entries.
func decodeKey(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	v, err := decodeRemoteValue(raw)
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", fmt.Errorf("bidi: unusable object key %s", string(raw))
	}
	return fmt.Sprint(v), nil
}
