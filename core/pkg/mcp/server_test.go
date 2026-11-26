package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/agent"
)

// fakeBackend answers the tools without a browser.
type fakeBackend struct {
	mu      sync.Mutex
	steps   []string
	closed  bool
	stepErr error              // returned by every Step while set
	block   chan struct{}      // Step waits on it, when set
	outcome *agent.StepOutcome // returned by Step, when set
}

func (f *fakeBackend) Step(ctx context.Context, instruction string) (agent.StepOutcome, error) {
	f.mu.Lock()
	f.steps = append(f.steps, instruction)
	block, err, out := f.block, f.stepErr, f.outcome
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return agent.StepOutcome{Step: instruction, Reason: agent.ReasonTimeout}, ctx.Err()
		}
	}
	if err != nil {
		return agent.StepOutcome{Step: instruction, Reason: agent.ReasonActionFailed, Error: err.Error()}, err
	}
	if out != nil {
		return *out, nil
	}
	return agent.StepOutcome{OK: true, Step: instruction, Action: "click", URL: "https://shop.test/", Reason: agent.ReasonOK, Score: 1}, nil
}

func (f *fakeBackend) Run(ctx context.Context, script string) (agent.RunOutcome, error) {
	return agent.RunOutcome{OK: true, TotalSteps: 2, Passed: 2, URL: "https://shop.test/"}, nil
}

func (f *fakeBackend) Map(ctx context.Context, b agent.MapBudget) (agent.PageMap, error) {
	return agent.PageMap{URL: "https://shop.test/", Groups: []agent.MapGroup{
		{Name: "Main", Elements: []agent.MapElement{{Label: "Add to cart", Role: "button"}, {Label: "Email", Role: "textbox", Editable: true}}},
	}}, nil
}

func (f *fakeBackend) Read(ctx context.Context, target string) (agent.Value, error) {
	if target == "Order total" {
		return agent.Value{Text: "$61.20", Found: true, Reason: agent.ReasonOK}, nil
	}
	return agent.Value{Reason: agent.ReasonNotFound}, nil
}

func (f *fakeBackend) ReadText(ctx context.Context, selector string) (string, error) {
	switch selector {
	case "#bad[":
		return "", agent.ErrBadSelector
	case "#none":
		return "", nil
	}
	return strings.Repeat("word ", 10), nil
}

func (f *fakeBackend) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

// client drives Serve over pipes, as an MCP client would.
type client struct {
	t     *testing.T
	in    *io.PipeWriter
	lines chan string
	done  chan error
	opens int
	mu    sync.Mutex
	next  int
}

func start(t *testing.T, open Opener) *client {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &client{t: t, in: inW, lines: make(chan string, 64), done: make(chan error, 1)}
	counted := func(ctx context.Context) (Backend, error) {
		c.mu.Lock()
		c.opens++
		c.mu.Unlock()
		return open(ctx)
	}
	go func() {
		c.done <- Serve(context.Background(), inR, outW, Options{EngineVersion: "9.9.9", Open: counted})
		_ = outW.Close()
	}()
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			c.lines <- sc.Text()
		}
		close(c.lines)
	}()
	t.Cleanup(func() { _ = inW.Close() })
	return c
}

func (c *client) send(v any) {
	c.t.Helper()
	b, _ := json.Marshal(v)
	if _, err := c.in.Write(append(b, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) request(method string, params any) int {
	c.next++
	c.send(map[string]any{"jsonrpc": "2.0", "id": c.next, "method": method, "params": params})
	return c.next
}

type reply struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *client) read() reply {
	c.t.Helper()
	select {
	case line, ok := <-c.lines:
		if !ok {
			c.t.Fatal("server closed its output")
		}
		var r reply
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			c.t.Fatalf("not JSON on stdout: %q", line)
		}
		return r
	case <-time.After(5 * time.Second):
		c.t.Fatal("no reply")
	}
	return reply{}
}

func (c *client) call(name string, args any) (text string, isError bool) {
	c.t.Helper()
	id := c.request("tools/call", map[string]any{"name": name, "arguments": args})
	r := c.read()
	if string(r.ID) != jsonID(id) {
		c.t.Fatalf("reply to %s, want %d", r.ID, id)
	}
	if r.Error != nil {
		c.t.Fatalf("%s: protocol error %d %s", name, r.Error.Code, r.Error.Message)
	}
	var res struct {
		Content []content `json:"content"`
		IsError bool      `json:"isError"`
	}
	if err := json.Unmarshal(r.Result, &res); err != nil {
		c.t.Fatal(err)
	}
	var parts []string
	for _, ct := range res.Content {
		parts = append(parts, ct.Text)
	}
	return strings.Join(parts, "\n"), res.IsError
}

func jsonID(n int) string { b, _ := json.Marshal(n); return string(b) }

func openFake(f *fakeBackend) Opener {
	return func(context.Context) (Backend, error) { return f, nil }
}

func TestInitialize_NegotiatesTheProtocolVersion(t *testing.T) {
	for asked, want := range map[string]string{
		"2025-06-18": "2025-06-18",
		"2024-11-05": "2024-11-05",
		"1999-01-01": protocolVersions[0],
	} {
		c := start(t, openFake(&fakeBackend{}))
		c.request("initialize", map[string]any{"protocolVersion": asked, "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}})
		var res struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name, Version string
			} `json:"serverInfo"`
			Capabilities map[string]any `json:"capabilities"`
			Instructions string         `json:"instructions"`
		}
		if err := json.Unmarshal(c.read().Result, &res); err != nil {
			t.Fatal(err)
		}
		if res.ProtocolVersion != want {
			t.Errorf("asked %s, got %s, want %s", asked, res.ProtocolVersion, want)
		}
		if res.ServerInfo.Name != "manul" || res.ServerInfo.Version != "9.9.9" || res.Capabilities["tools"] == nil || res.Instructions == "" {
			t.Errorf("initialize result: %+v", res)
		}
	}
}

func TestToolsList_IsTheContractsTools(t *testing.T) {
	c := start(t, openFake(&fakeBackend{}))
	c.request("tools/list", nil)
	var res struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
			Annotations annotations    `json:"annotations"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(c.read().Result, &res); err != nil {
		t.Fatal(err)
	}
	readOnlyTools := map[string]bool{"map": true, "read": true, "read_text": true}
	var names []string
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
		if tl.InputSchema["type"] != "object" || tl.Description == "" || tl.Annotations.Title == "" {
			t.Errorf("%s: incomplete definition", tl.Name)
		}
		if tl.Annotations.ReadOnlyHint != readOnlyTools[tl.Name] || tl.Annotations.DestructiveHint == readOnlyTools[tl.Name] {
			t.Errorf("%s: readOnly=%v destructive=%v", tl.Name, tl.Annotations.ReadOnlyHint, tl.Annotations.DestructiveHint)
		}
	}
	if strings.Join(names, ",") != "step,map,read,read_text,run_hunt" {
		t.Errorf("tools = %v", names)
	}
	// Listing the tools starts no browser.
	if c.opens != 0 {
		t.Errorf("tools/list opened the browser %d times", c.opens)
	}
}

func TestToolsCall_TheBrowserOpensOnceOnFirstUse(t *testing.T) {
	f := &fakeBackend{}
	c := start(t, openFake(f))
	out, isErr := c.call("step", map[string]any{"step": "CLICK the 'Add to cart' button"})
	if isErr || !strings.HasPrefix(out, "OK: CLICK the 'Add to cart' button") {
		t.Errorf("step: %v %q", isErr, out)
	}
	if out, isErr := c.call("map", map[string]any{}); isErr || !strings.Contains(out, "'Add to cart' [button — CLICK]") || !strings.Contains(out, "'Email' [textbox — FILL]") {
		t.Errorf("map: %v %q", isErr, out)
	}
	if out, isErr := c.call("read", map[string]any{"target": "Order total"}); isErr || out != "$61.20" {
		t.Errorf("read: %v %q", isErr, out)
	}
	if out, isErr := c.call("read", map[string]any{"target": "Nope"}); !isErr || !strings.Contains(out, "Nothing on the page is labelled") {
		t.Errorf("read of a missing label: %v %q", isErr, out)
	}
	if out, isErr := c.call("read_text", map[string]any{"max_chars": 9}); isErr || !strings.Contains(out, "chars truncated") {
		t.Errorf("read_text: %v %q", isErr, out)
	}
	if out, isErr := c.call("read_text", map[string]any{"selector": "#bad["}); !isErr || !strings.Contains(out, "not a valid CSS selector") {
		t.Errorf("read_text, bad selector: %v %q", isErr, out)
	}
	if out, isErr := c.call("read_text", map[string]any{"selector": "#none"}); !isErr || !strings.Contains(out, "Nothing on the page matches") {
		t.Errorf("read_text, no match: %v %q", isErr, out)
	}
	if out, isErr := c.call("run_hunt", map[string]any{"script": "CLICK the 'A' button\nCLICK the 'B' button"}); isErr || !strings.HasPrefix(out, "SUCCESS: ran 2 step(s)") {
		t.Errorf("run_hunt: %v %q", isErr, out)
	}
	if c.opens != 1 {
		t.Errorf("the browser was opened %d times, want 1", c.opens)
	}
}

// A step that fails is the tool's answer, with what the model needs to correct
// it — not a protocol error the model never sees.
func TestToolsCall_AFailedStepIsAToolError(t *testing.T) {
	f := &fakeBackend{outcome: &agent.StepOutcome{
		Step: "CLICK the 'Buy' button", Action: "click", Reason: agent.ReasonNotFound,
		Near: []agent.Cand{{Text: "Buy now", Score: 0.4}},
	}}
	c := start(t, openFake(f))
	out, isErr := c.call("step", map[string]any{"step": "CLICK the 'Buy' button"})
	if !isErr || !strings.HasPrefix(out, "FAILED: CLICK the 'Buy' button") || !strings.Contains(out, "'Buy now'") {
		t.Errorf("got %v %q", isErr, out)
	}
}

func TestToolsCall_BadArgumentsAreToolErrors(t *testing.T) {
	c := start(t, openFake(&fakeBackend{}))
	for _, tc := range []struct {
		tool string
		args any
		want string
	}{
		{"step", map[string]any{}, "step is empty"},
		{"step", map[string]any{"step": "CLICK the 'A' button\nCLICK the 'B' button"}, "use run_hunt"},
		{"step", map[string]any{"instruction": "x"}, "bad arguments"},
		{"read", map[string]any{"target": " "}, "target is empty"},
		{"map", map[string]any{"max_per_group": -1}, "must not be negative"},
		{"run_hunt", map[string]any{"script": ""}, "script is empty"},
	} {
		if out, isErr := c.call(tc.tool, tc.args); !isErr || !strings.Contains(out, tc.want) {
			t.Errorf("%s %v: %v %q, want an error containing %q", tc.tool, tc.args, isErr, out, tc.want)
		}
	}
}

func TestProtocolErrors(t *testing.T) {
	c := start(t, openFake(&fakeBackend{}))

	_, _ = c.in.Write([]byte("{not json\n"))
	if r := c.read(); r.Error == nil || r.Error.Code != codeParseError || string(r.ID) != "null" {
		t.Errorf("parse error: %+v", r)
	}
	c.request("resources/list", nil)
	if r := c.read(); r.Error == nil || r.Error.Code != codeMethodNotFound {
		t.Errorf("unknown method: %+v", r)
	}
	c.request("tools/call", map[string]any{"name": "teleport", "arguments": map[string]any{}})
	if r := c.read(); r.Error == nil || r.Error.Code != codeInvalidParams || !strings.Contains(r.Error.Message, "unknown tool") {
		t.Errorf("unknown tool: %+v", r)
	}
	// A notification gets no answer, known or not; the ping after it does,
	// and is the next thing on stdout.
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/whatever"})
	id := c.request("ping", nil)
	if r := c.read(); string(r.ID) != jsonID(id) || r.Error != nil {
		t.Errorf("ping: %+v", r)
	}
}

func TestToolsCall_ABrowserThatCannotStartIsTriedAgain(t *testing.T) {
	f := &fakeBackend{}
	fail := true
	c := start(t, func(context.Context) (Backend, error) {
		if fail {
			fail = false
			return nil, errors.New("chrome not found")
		}
		return f, nil
	})
	if out, isErr := c.call("step", map[string]any{"step": "NAVIGATE to https://shop.test/"}); !isErr || !strings.Contains(out, "Could not start the browser: chrome not found") {
		t.Errorf("first call: %v %q", isErr, out)
	}
	if _, isErr := c.call("step", map[string]any{"step": "NAVIGATE to https://shop.test/"}); isErr {
		t.Error("second call failed; the browser was not tried again")
	}
}

// The user closed the browser's window: the next call reopens it once, and
// says so, rather than failing every call from then on.
func TestToolsCall_ABrowserThatWentAwayIsReopened(t *testing.T) {
	gone := &fakeBackend{stepErr: errors.New("cdp connection closed")}
	fresh := &fakeBackend{}
	n := 0
	c := start(t, func(context.Context) (Backend, error) {
		n++
		if n == 1 {
			return gone, nil
		}
		return fresh, nil
	})
	out, isErr := c.call("step", map[string]any{"step": "NAVIGATE to https://shop.test/"})
	if isErr || !strings.Contains(out, "a new one was opened") || !strings.Contains(out, "OK: NAVIGATE") {
		t.Errorf("got %v %q", isErr, out)
	}
	if !gone.closed || len(fresh.steps) != 1 {
		t.Errorf("old closed=%v, steps on the new one=%d", gone.closed, len(fresh.steps))
	}
}

func TestToolsCall_ACancelledCallIsNotAnswered(t *testing.T) {
	f := &fakeBackend{block: make(chan struct{})}
	c := start(t, openFake(f))
	id := c.request("tools/call", map[string]any{"name": "step", "arguments": map[string]any{"step": "WAIT 60"}})
	time.Sleep(50 * time.Millisecond)
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled", "params": map[string]any{"requestId": id}})
	// The next reply is the ping's, not the cancelled call's.
	f.mu.Lock()
	f.block = nil
	f.mu.Unlock()
	pid := c.request("ping", nil)
	if r := c.read(); string(r.ID) != jsonID(pid) {
		t.Errorf("got a reply to %s, want only the ping's (%d)", r.ID, pid)
	}
	if _, isErr := c.call("step", map[string]any{"step": "CLICK the 'A' button"}); isErr {
		t.Error("the call after a cancelled one failed")
	}
}

func TestServe_EndOfInputClosesTheBrowser(t *testing.T) {
	f := &fakeBackend{}
	c := start(t, openFake(f))
	c.call("step", map[string]any{"step": "NAVIGATE to https://shop.test/"})
	_ = c.in.Close()
	select {
	case err := <-c.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return at end of input")
	}
	if !f.closed {
		t.Error("the browser was left open")
	}
}
