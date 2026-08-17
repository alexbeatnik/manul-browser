package bidi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// sentCmd is one command the mock browser received.
type sentCmd struct {
	Method string
	Params json.RawMessage
}

// mockBrowser is a WebSocket endpoint that speaks enough BiDi to drive the
// client: it replies to commands from a scripted table and can push events.
//
// A result string starting with "!" is replied as a protocol error, which is
// how a test asks for the failure path.
type mockBrowser struct {
	srv     *httptest.Server
	events  chan json.RawMessage
	mu      sync.Mutex
	sent    []sentCmd
	results map[string]string
}

func startMockBrowser(t *testing.T, results map[string]string) *mockBrowser {
	t.Helper()
	m := &mockBrowser{
		events:  make(chan json.RawMessage, 16),
		results: results,
	}
	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
	var writeMu sync.Mutex

	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		go func() {
			for ev := range m.events {
				writeMu.Lock()
				_ = conn.WriteMessage(websocket.TextMessage, ev)
				writeMu.Unlock()
			}
		}()
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var in cmd
			if err := json.Unmarshal(raw, &in); err != nil {
				continue
			}
			params, _ := json.Marshal(in.Params)
			m.mu.Lock()
			m.sent = append(m.sent, sentCmd{Method: in.Method, Params: params})
			result, ok := m.results[in.Method]
			m.mu.Unlock()
			if !ok {
				result = "{}"
			}

			var reply []byte
			if strings.HasPrefix(result, "!") {
				reply, _ = json.Marshal(map[string]any{
					"type": "error", "id": in.ID,
					"error": "unknown command", "message": strings.TrimPrefix(result, "!"),
				})
			} else {
				reply = []byte(`{"type":"success","id":` + strconv.FormatInt(in.ID, 10) + `,"result":` + result + `}`)
			}
			writeMu.Lock()
			_ = conn.WriteMessage(websocket.TextMessage, reply)
			writeMu.Unlock()
		}
	}))
	t.Cleanup(func() {
		close(m.events)
		m.srv.Close()
	})
	return m
}

func (m *mockBrowser) wsURL() string {
	return "ws" + strings.TrimPrefix(m.srv.URL, "http")
}

// paramsFor returns the params of the first command with the given method.
func (m *mockBrowser) paramsFor(method string) map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.sent {
		if c.Method == method {
			var out map[string]any
			_ = json.Unmarshal(c.Params, &out)
			return out
		}
	}
	return nil
}

func (m *mockBrowser) methods() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.sent))
	for _, c := range m.sent {
		out = append(out, c.Method)
	}
	return out
}

// waitForMethod blocks until the mock has received the named command, so a
// test can push events only once the client is listening for them.
func waitForMethod(t *testing.T, m *mockBrowser, method string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, got := range m.methods() {
			if got == method {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s was never sent", method)
}

func dialMock(t *testing.T, m *mockBrowser) (*Conn, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	c, err := Dial(ctx, m.wsURL())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, ctx
}

func TestConn_CallReturnsResult(t *testing.T) {
	m := startMockBrowser(t, map[string]string{"session.status": `{"ready":true}`})
	c, ctx := dialMock(t, m)

	res, err := c.Call(ctx, "session.status", nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(string(res), `"ready":true`) {
		t.Fatalf("unexpected result: %s", res)
	}
}

// An error reply carries the same id as the command it answers. It must
// surface as an error rather than be routed to event subscribers or, worse,
// leave the caller waiting for a success that never comes.
func TestConn_CallSurfacesProtocolError(t *testing.T) {
	m := startMockBrowser(t, map[string]string{"browsingContext.navigate": "!no such frame"})
	c, ctx := dialMock(t, m)

	_, err := c.Call(ctx, "browsingContext.navigate", map[string]any{"context": "x"})
	if err == nil {
		t.Fatal("expected an error reply to fail the call")
	}
	if !strings.Contains(err.Error(), "no such frame") {
		t.Fatalf("error lost the message: %v", err)
	}
}

func TestConn_SubscribeReceivesEvents(t *testing.T) {
	m := startMockBrowser(t, nil)
	c, _ := dialMock(t, m)

	sub := c.Subscribe()
	defer sub.Close()

	m.events <- json.RawMessage(`{"type":"event","method":"network.responseCompleted","params":{"response":{"url":"https://example.com/api"}}}`)

	select {
	case ev := <-sub.C():
		if ev.Method != "network.responseCompleted" {
			t.Fatalf("unexpected event method %q", ev.Method)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("event never arrived")
	}
}

func TestConn_ClosedReportsTeardown(t *testing.T) {
	m := startMockBrowser(t, nil)
	c, _ := dialMock(t, m)

	if c.Closed() {
		t.Fatal("a live connection reported itself closed")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !c.Closed() {
		t.Fatal("a closed connection reported itself live")
	}
	// Closed connections are what the browser backend keys its reconnect on,
	// so a second Close must stay harmless.
	if err := c.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestNewSession_StoresSessionID(t *testing.T) {
	m := startMockBrowser(t, map[string]string{"session.new": `{"sessionId":"abc-123","capabilities":{}}`})
	c, ctx := dialMock(t, m)

	if err := NewSession(ctx, c); err != nil {
		t.Fatalf("session.new: %v", err)
	}
	if c.SessionID() != "abc-123" {
		t.Fatalf("session id = %q, want abc-123", c.SessionID())
	}
	// Commands must always carry a params object; Firefox rejects a bare
	// command outright.
	if p := m.paramsFor("session.new"); p == nil {
		t.Fatal("session.new was sent without params")
	} else if _, ok := p["capabilities"]; !ok {
		t.Fatalf("session.new params missing capabilities: %v", p)
	}
}
