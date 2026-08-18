package bidi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// evalResult wraps a serialized value the way script.evaluate returns it.
func evalResult(remote string) string {
	return `{"type":"success","realm":"r1","result":` + remote + `}`
}

// Evaluate must hand back the same Go shapes pkg/cdp's returnByValue does:
// pkg/browser marshals whatever it gets, and a decode that produced different
// types would silently change what every probe sees on Firefox.
func TestEvaluate_DecodesRemoteValuesLikeCDP(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		want   interface{}
	}{
		{"string", `{"type":"string","value":"complete"}`, "complete"},
		{"number", `{"type":"number","value":42}`, float64(42)},
		{"boolean", `{"type":"boolean","value":true}`, true},
		{"null", `{"type":"null"}`, nil},
		{"undefined", `{"type":"undefined"}`, nil},
		{
			"array of primitives",
			`{"type":"array","value":[{"type":"number","value":1},{"type":"string","value":"a"}]}`,
			[]interface{}{float64(1), "a"},
		},
		{
			"object",
			`{"type":"object","value":[["id",{"type":"number","value":7}],["tag",{"type":"string","value":"button"}]]}`,
			map[string]interface{}{"id": float64(7), "tag": "button"},
		},
		{
			"nested object inside array — the shape every DOM probe returns",
			`{"type":"array","value":[{"type":"object","value":[["visible",{"type":"boolean","value":true}]]}]}`,
			[]interface{}{map[string]interface{}{"visible": true}},
		},
		{
			// NaN cannot survive a round trip back to JSON, and CDP drops it
			// too (unserializableValue with no value).
			"unserializable number",
			`{"type":"number","value":"NaN"}`,
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := startMockBrowser(t, map[string]string{"script.evaluate": evalResult(tc.remote)})
			c, ctx := dialMock(t, m)

			got, err := Evaluate(ctx, c, "ctx-1", "probe()")
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("decoded %#v, want %#v", got, tc.want)
			}
		})
	}
}

// A JS exception is a failed step, not an empty result. Swallowing it would
// make a broken probe look like a page with nothing on it.
func TestEvaluate_ExceptionBecomesError(t *testing.T) {
	m := startMockBrowser(t, map[string]string{
		"script.evaluate": `{"type":"exception","exceptionDetails":{"text":"ReferenceError: nope is not defined"}}`,
	})
	c, ctx := dialMock(t, m)

	_, err := Evaluate(ctx, c, "ctx-1", "nope()")
	if err == nil {
		t.Fatal("expected a JS exception to fail the call")
	}
	if got := err.Error(); got != "js exception: ReferenceError: nope is not defined" {
		t.Fatalf("unexpected error text: %s", got)
	}
}

func TestEvaluate_TargetsTheGivenContext(t *testing.T) {
	m := startMockBrowser(t, map[string]string{
		"script.evaluate": evalResult(`{"type":"string","value":"ok"}`),
	})
	c, ctx := dialMock(t, m)

	if _, err := Evaluate(ctx, c, "frame-7", "document.title"); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	p := m.paramsFor("script.evaluate")
	target, _ := p["target"].(map[string]any)
	if target == nil || target["context"] != "frame-7" {
		t.Fatalf("evaluate was not addressed to frame-7: %v", p)
	}
	if p["awaitPromise"] != true {
		t.Fatalf("awaitPromise must be set so async probes resolve: %v", p)
	}
}

// CallFunction composes the same `(fn)(arg)` expression pkg/cdp does, so a
// probe behaves identically on both backends.
func TestCallFunction_ComposesInvocation(t *testing.T) {
	m := startMockBrowser(t, map[string]string{
		"script.evaluate": evalResult(`{"type":"number","value":1}`),
	})
	c, ctx := dialMock(t, m)

	if _, err := CallFunction(ctx, c, "ctx-1", "(a) => a.n", map[string]int{"n": 3}); err != nil {
		t.Fatalf("call function: %v", err)
	}
	got := m.paramsFor("script.evaluate")["expression"]
	if got != `((a) => a.n)({"n":3})` {
		t.Fatalf("expression = %v", got)
	}

	// A nil argument evaluates the function source itself, matching
	// cdp.CallFunctionOn.
	m2 := startMockBrowser(t, map[string]string{
		"script.evaluate": evalResult(`{"type":"number","value":1}`),
	})
	c2, ctx2 := dialMock(t, m2)
	if _, err := CallFunction(ctx2, c2, "ctx-1", "probe()", nil); err != nil {
		t.Fatalf("call function: %v", err)
	}
	if got := m2.paramsFor("script.evaluate")["expression"]; got != "probe()" {
		t.Fatalf("expression = %v", got)
	}
}

func TestClick_SendsPointerSequence(t *testing.T) {
	m := startMockBrowser(t, nil)
	c, ctx := dialMock(t, m)

	if err := Click(ctx, c, "ctx-1", 12.4, 30.6); err != nil {
		t.Fatalf("click: %v", err)
	}
	p := m.paramsFor("input.performActions")
	if p["context"] != "ctx-1" {
		t.Fatalf("wrong context: %v", p)
	}
	sources, _ := p["actions"].([]any)
	if len(sources) != 1 {
		t.Fatalf("want one input source, got %v", p["actions"])
	}
	src, _ := sources[0].(map[string]any)
	if src["type"] != "pointer" {
		t.Fatalf("want a pointer source: %v", src)
	}
	actions, _ := src["actions"].([]any)
	if len(actions) != 3 {
		t.Fatalf("want move/down/up, got %v", actions)
	}
	move, _ := actions[0].(map[string]any)
	// Coordinates are integers on the wire: a fractional element centre that
	// was passed through verbatim would be rejected by the browser.
	if move["x"] != float64(12) || move["y"] != float64(31) {
		t.Fatalf("coordinates not rounded to integers: %v", move)
	}
	if move["origin"] != "viewport" {
		t.Fatalf("move must be viewport-relative: %v", move)
	}
}

func TestDragAndDrop_InterpolatesAndHoldsTheButton(t *testing.T) {
	m := startMockBrowser(t, nil)
	c, ctx := dialMock(t, m)

	if err := DragAndDrop(ctx, c, "ctx-1", 0, 0, 100, 50); err != nil {
		t.Fatalf("drag: %v", err)
	}
	src, _ := m.paramsFor("input.performActions")["actions"].([]any)
	actions, _ := src[0].(map[string]any)["actions"].([]any)

	// move, down, dragSteps moves, a settling move, up — all in ONE sequence,
	// which is what keeps the button held across the moves.
	if len(actions) != dragSteps+4 {
		t.Fatalf("want %d actions, got %d", dragSteps+4, len(actions))
	}
	first, _ := actions[1].(map[string]any)
	last, _ := actions[len(actions)-1].(map[string]any)
	if first["type"] != "pointerDown" || last["type"] != "pointerUp" {
		t.Fatalf("press/release missing: %v then %v", first, last)
	}
	settle, _ := actions[len(actions)-2].(map[string]any)
	if settle["x"] != float64(100) || settle["y"] != float64(50) {
		t.Fatalf("drag must settle on the target before release: %v", settle)
	}
}

func TestDispatchKey_MapsNamesAndWrapsModifiers(t *testing.T) {
	m := startMockBrowser(t, nil)
	c, ctx := dialMock(t, m)

	// Ctrl+Shift+Enter: modifiers press before the key and release after it.
	if err := DispatchKey(ctx, c, "ctx-1", "Enter", 2|8); err != nil {
		t.Fatalf("dispatch key: %v", err)
	}
	src, _ := m.paramsFor("input.performActions")["actions"].([]any)
	source, _ := src[0].(map[string]any)
	if source["type"] != "key" {
		t.Fatalf("want a key source: %v", source)
	}
	actions, _ := source["actions"].([]any)
	var got []string
	for _, a := range actions {
		e, _ := a.(map[string]any)
		got = append(got, fmt.Sprintf("%s:%s", e["type"], strconv.Quote(fmt.Sprint(e["value"]))))
	}

	// The WebDriver code points, written as runes: the escaped spelling is
	// invisible in a diff, and these are the values the browser matches on.
	var (
		enter = string(rune(0xE007))
		shift = string(rune(0xE008))
		ctrl  = string(rune(0xE009))
	)
	want := []string{
		"keyDown:" + strconv.Quote(ctrl),
		"keyDown:" + strconv.Quote(shift),
		"keyDown:" + strconv.Quote(enter),
		"keyUp:" + strconv.Quote(enter),
		"keyUp:" + strconv.Quote(shift),
		"keyUp:" + strconv.Quote(ctrl),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("key sequence = %v, want %v", got, want)
	}

	// Held modifiers survive between performActions calls, so the sequence
	// has to hand input state back.
	methods := m.methods()
	if methods[len(methods)-1] != "input.releaseActions" {
		t.Fatalf("releaseActions must follow a key press, got %v", methods)
	}
}

func TestKeyValue_PassesPrintableCharactersThrough(t *testing.T) {
	if got := KeyValue("a"); got != "a" {
		t.Fatalf("printable key mangled: %q", got)
	}
	if got := KeyValue(" "); got != " " {
		t.Fatalf("space mangled: %q", got)
	}
	if got := KeyValue("Escape"); got != string(rune(0xE00C)) {
		t.Fatalf("Escape = %q", got)
	}
}

func TestGetTree_ReturnsNestedContexts(t *testing.T) {
	m := startMockBrowser(t, map[string]string{
		"browsingContext.getTree": `{"contexts":[{"context":"top","url":"https://example.com","children":[{"context":"frame","url":"https://example.com/f","children":[]}]}]}`,
	})
	c, ctx := dialMock(t, m)

	contexts, err := GetTree(ctx, c, "top")
	if err != nil {
		t.Fatalf("get tree: %v", err)
	}
	if len(contexts) != 1 || contexts[0].ID != "top" {
		t.Fatalf("unexpected tree: %+v", contexts)
	}
	if len(contexts[0].Children) != 1 || contexts[0].Children[0].ID != "frame" {
		t.Fatalf("iframe context missing: %+v", contexts[0])
	}
	if m.paramsFor("browsingContext.getTree")["root"] != "top" {
		t.Fatal("root was not passed through")
	}
}

func TestWaitForResponse_MatchesSuffixAndUnsubscribes(t *testing.T) {
	m := startMockBrowser(t, nil)
	c, ctx := dialMock(t, m)

	done := make(chan error, 1)
	go func() {
		done <- WaitForResponse(ctx, c, "ctx-1", "/api/cart", 5*time.Second)
	}()
	waitForMethod(t, m, "session.subscribe")

	// A non-matching response must not end the wait.
	m.events <- json.RawMessage(`{"type":"event","method":"network.responseCompleted","params":{"response":{"url":"https://x.test/other"}}}`)
	m.events <- json.RawMessage(`{"type":"event","method":"network.responseCompleted","params":{"response":{"url":"https://x.test/api/cart"}}}`)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("matching response never ended the wait")
	}

	methods := m.methods()
	var subscribed, unsubscribed bool
	for _, mth := range methods {
		switch mth {
		case "session.subscribe":
			subscribed = true
		case "session.unsubscribe":
			unsubscribed = true
		}
	}
	// BiDi sends no events until asked, and keeps sending them until told to
	// stop — both halves matter.
	if !subscribed || !unsubscribed {
		t.Fatalf("subscribe/unsubscribe missing: %v", methods)
	}
}

func TestWaitForResponse_TimesOut(t *testing.T) {
	m := startMockBrowser(t, nil)
	c, ctx := dialMock(t, m)

	err := WaitForResponse(ctx, c, "ctx-1", "/never", 300*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout")
	}
}

func TestWebSocketURL(t *testing.T) {
	ctx := context.Background()

	// An explicit socket URL is taken as given.
	got, err := WebSocketURL(ctx, "ws://127.0.0.1:9222/session")
	if err != nil || got != "ws://127.0.0.1:9222/session" {
		t.Fatalf("ws URL = %q, %v", got, err)
	}

	// An HTTP endpoint that advertises one is believed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"webSocketDebuggerUrl": "ws://127.0.0.1:4444/session",
		})
	}))
	defer srv.Close()
	got, err = WebSocketURL(ctx, srv.URL)
	if err != nil || got != "ws://127.0.0.1:4444/session" {
		t.Fatalf("advertised URL ignored: %q, %v", got, err)
	}

	// An endpoint that advertises nothing falls back to the conventional path,
	// which is where Firefox listens.
	quiet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer quiet.Close()
	got, err = WebSocketURL(ctx, quiet.URL)
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}
	want := "ws://" + quiet.Listener.Addr().String() + "/session"
	if got != want {
		t.Fatalf("fallback URL = %q, want %q", got, want)
	}
}
