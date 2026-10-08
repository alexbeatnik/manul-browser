package cdp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// startMockCDP spins up a local WebSocket endpoint that echoes JSON-RPC
// requests back as responses and lets tests inject CDP events.
func startMockCDP(t *testing.T) (wsURL string, eventsIn chan<- json.RawMessage, stop func()) {
	t.Helper()
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	events := make(chan json.RawMessage, 16)
	var (
		mu       sync.Mutex
		liveConn *websocket.Conn
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		mu.Lock()
		liveConn = conn
		mu.Unlock()

		go func() {
			for ev := range events {
				mu.Lock()
				c := liveConn
				mu.Unlock()
				if c == nil {
					return
				}
				_ = c.WriteMessage(websocket.TextMessage, ev)
			}
		}()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				mu.Lock()
				liveConn = nil
				mu.Unlock()
				return
			}
			var req msgReq
			if err := json.Unmarshal(msg, &req); err != nil {
				continue
			}
			resp := msgResp{ID: req.ID, Result: json.RawMessage(`{"ok":true}`)}
			b, _ := json.Marshal(resp)
			_ = conn.WriteMessage(websocket.TextMessage, b)
		}
	}))

	wsURL = "ws" + strings.TrimPrefix(srv.URL, "http")
	stop = func() {
		close(events)
		srv.Close()
	}
	return wsURL, events, stop
}

func TestConn_CallReturnsResult(t *testing.T) {
	wsURL, _, stop := startMockCDP(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := DialTarget(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	res, err := c.Call(ctx, "Test.method", nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if string(res) != `{"ok":true}` {
		t.Fatalf("unexpected result: %s", string(res))
	}
}

func TestConn_ParallelCallsUniqueIDs(t *testing.T) {
	wsURL, _, stop := startMockCDP(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := DialTarget(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	const N = 64
	var wg sync.WaitGroup
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Call(ctx, "Test.method", nil); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("call err: %v", err)
	}
}

func TestConn_SubscribeReceivesEvents(t *testing.T) {
	wsURL, events, stop := startMockCDP(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := DialTarget(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	sub := c.Subscribe()
	defer sub.Close()

	// Give the connection a moment to be live on the server side.
	time.Sleep(50 * time.Millisecond)
	events <- json.RawMessage(`{"method":"Test.event","params":{"k":"v"}}`)

	select {
	case ev, ok := <-sub.C():
		if !ok {
			t.Fatalf("subscription closed unexpectedly")
		}
		if ev.Method != "Test.event" {
			t.Fatalf("unexpected event method: %s", ev.Method)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not receive event")
	}
}

func TestConn_SubscriptionCloseIdempotent(t *testing.T) {
	wsURL, _, stop := startMockCDP(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := DialTarget(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	sub := c.Subscribe()
	sub.Close()
	sub.Close() // must not panic
}

func TestConn_CloseUnblocksSubscribers(t *testing.T) {
	wsURL, _, stop := startMockCDP(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := DialTarget(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	sub := c.Subscribe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range sub.C() {
		}
	}()

	_ = c.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("subscriber did not exit after Close()")
	}
}

func TestConn_CloseIsIdempotent(t *testing.T) {
	wsURL, _, stop := startMockCDP(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := DialTarget(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// Second close must not panic and must return without error.
	if err := c.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestConn_ParentCtxCancelTearsDownConn(t *testing.T) {
	wsURL, _, stop := startMockCDP(t)
	defer stop()

	parent, cancel := context.WithCancel(context.Background())
	c, err := DialTarget(parent, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	cancel()

	// Pending Call must observe connection closure.
	_, err = c.Call(context.Background(), "Test.method", nil)
	if err == nil {
		t.Fatalf("expected error after parent cancel")
	}
}

// scriptedCDP is a CDP endpoint that records every call, answers the methods
// it was given results for (anything else gets `{}`), and can push events.
type scriptedCDP struct {
	t       *testing.T
	results map[string]string

	mu    sync.Mutex
	calls []scriptedCall
	conn  *websocket.Conn
}

type scriptedCall struct {
	Method string
	Params json.RawMessage
}

func startScriptedCDP(t *testing.T, results map[string]string) (*scriptedCDP, *Conn, context.Context) {
	t.Helper()
	s := &scriptedCDP{t: t, results: results}
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		s.mu.Lock()
		s.conn = conn
		s.mu.Unlock()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req struct {
				ID     int64           `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.Unmarshal(msg, &req); err != nil {
				continue
			}
			result := json.RawMessage(`{}`)
			if r, ok := s.results[req.Method]; ok {
				result = json.RawMessage(r)
			}
			b, _ := json.Marshal(msgResp{ID: req.ID, Result: result})
			s.mu.Lock()
			s.calls = append(s.calls, scriptedCall{req.Method, req.Params})
			_ = conn.WriteMessage(websocket.TextMessage, b)
			s.mu.Unlock()
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	c, err := DialTarget(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return s, c, ctx
}

// push sends an event to the client.
func (s *scriptedCDP) push(event string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.conn.WriteMessage(websocket.TextMessage, []byte(event)); err != nil {
		s.t.Errorf("push: %v", err)
	}
}

func (s *scriptedCDP) methods() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.calls))
	for _, c := range s.calls {
		names = append(names, c.Method)
	}
	return strings.Join(names, ",")
}

// await waits for a call to method and decodes its params into out.
func (s *scriptedCDP) await(method string, out any) {
	s.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for _, c := range s.calls {
			if c.Method == method {
				s.mu.Unlock()
				if err := json.Unmarshal(c.Params, out); err != nil {
					s.t.Fatalf("%s params: %v", method, err)
				}
				return
			}
		}
		s.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	s.t.Fatalf("%s was never called; saw %s", method, s.methods())
}

// DOM.requestNode only answers once the DOM agent has been handed the
// document, which nothing in this package does — so it returned node 0 and
// every UPLOAD failed with "Could not find node with given id". The input is
// addressed by the object id the evaluate already produced.
func TestSetFileInput_AddressesTheInputByObjectID(t *testing.T) {
	s, c, ctx := startScriptedCDP(t, map[string]string{
		"Runtime.evaluate": `{"result":{"type":"object","objectId":"obj-7"}}`,
	})

	if err := c.SetFileInput(ctx, 3, "/html/body/input[1]", []string{`C:\tmp\a.txt`}); err != nil {
		t.Fatalf("SetFileInput: %v", err)
	}
	if got := s.methods(); got != "Runtime.evaluate,DOM.setFileInputFiles" {
		t.Errorf("calls = %s", got)
	}
	var sent struct {
		ObjectID string   `json:"objectId"`
		Files    []string `json:"files"`
	}
	s.await("DOM.setFileInputFiles", &sent)
	if sent.ObjectID != "obj-7" || len(sent.Files) != 1 || sent.Files[0] != `C:\tmp\a.txt` {
		t.Errorf("setFileInputFiles params = %+v", sent)
	}
}

// Nothing listened for a dialog, and a dialog blocks the page: the click that
// opened an alert never returned, and the session with it.
func TestAcceptDialogs_AnswersWhatThePageOpens(t *testing.T) {
	s, c, ctx := startScriptedCDP(t, nil)

	if err := AcceptDialogs(ctx, c); err != nil {
		t.Fatalf("AcceptDialogs: %v", err)
	}
	// Chrome only announces dialogs to a client that enabled the Page domain.
	if got := s.methods(); got != "Page.enable" {
		t.Fatalf("calls = %s", got)
	}

	s.push(`{"method":"Page.javascriptDialogOpening","params":{"type":"prompt","message":"Name?","defaultPrompt":"Ada"}}`)

	var answer struct {
		Accept     bool   `json:"accept"`
		PromptText string `json:"promptText"`
	}
	s.await("Page.handleJavaScriptDialog", &answer)
	if !answer.Accept || answer.PromptText != "Ada" {
		t.Errorf("answer = %+v, want accepted with the prompt's default", answer)
	}
}

func TestOnRequestPaused_HandsOverTheRequestAndFulfilsIt(t *testing.T) {
	s, c, ctx := startScriptedCDP(t, nil)

	paused := make(chan PausedRequest, 1)
	OnRequestPaused(c, func(req PausedRequest) { paused <- req })
	if err := InterceptRequests(ctx, c, []string{"*/api/users"}); err != nil {
		t.Fatalf("InterceptRequests: %v", err)
	}
	var enabled struct {
		Patterns []struct {
			URLPattern   string `json:"urlPattern"`
			RequestStage string `json:"requestStage"`
		} `json:"patterns"`
	}
	s.await("Fetch.enable", &enabled)
	if len(enabled.Patterns) != 1 || enabled.Patterns[0].URLPattern != "*/api/users" || enabled.Patterns[0].RequestStage != "Request" {
		t.Errorf("Fetch.enable patterns = %+v", enabled.Patterns)
	}
	// A cached response never becomes a request, so it could not be paused.
	var cache struct {
		CacheDisabled bool `json:"cacheDisabled"`
	}
	s.await("Network.setCacheDisabled", &cache)
	if !cache.CacheDisabled {
		t.Error("the cache was left on")
	}

	s.push(`{"method":"Fetch.requestPaused","params":{"requestId":"req-1","request":{"url":"https://x.test/api/users","method":"GET","headers":{"Origin":"https://x.test"}}}}`)

	var req PausedRequest
	select {
	case req = <-paused:
	case <-time.After(3 * time.Second):
		t.Fatal("the paused request was never handed over")
	}
	if req.ID != "req-1" || req.Method != "GET" || req.URL != "https://x.test/api/users" || req.Headers["Origin"] != "https://x.test" {
		t.Errorf("paused request = %+v", req)
	}

	if err := FulfillRequest(ctx, c, req.ID, 200, [][2]string{{"Content-Type", "application/json"}}, []byte(`{"ok":1}`)); err != nil {
		t.Fatalf("FulfillRequest: %v", err)
	}
	var fulfilled struct {
		RequestID       string              `json:"requestId"`
		ResponseCode    int                 `json:"responseCode"`
		ResponseHeaders []map[string]string `json:"responseHeaders"`
		Body            string              `json:"body"`
	}
	s.await("Fetch.fulfillRequest", &fulfilled)
	// The body travels base64-encoded: {"ok":1}.
	if fulfilled.RequestID != "req-1" || fulfilled.ResponseCode != 200 || fulfilled.Body != "eyJvayI6MX0=" ||
		len(fulfilled.ResponseHeaders) != 1 || fulfilled.ResponseHeaders[0]["name"] != "Content-Type" {
		t.Errorf("fulfilment = %+v", fulfilled)
	}
}

// Chrome answers Page.navigate successfully for a page it could not load and
// reports the failure in the reply. Unread, a NAVIGATE to a host that does not
// resolve passed, and the hunt carried on against chrome-error://.
func TestNavigate_ReportsWhatChromeCouldNotLoad(t *testing.T) {
	for reply, wantErr := range map[string]string{
		`{"frameId":"F1","loaderId":"L1"}`:                                "",
		`{"frameId":"F1","errorText":"net::ERR_NAME_NOT_RESOLVED"}`:       "net::ERR_NAME_NOT_RESOLVED",
		`{"frameId":"F1","errorText":"net::ERR_CONNECTION_REFUSED"}`:      "net::ERR_CONNECTION_REFUSED",
		`{"frameId":"F1","loaderId":"L1","errorText":"net::ERR_ABORTED"}`: "", // became a download; the page is as it was
	} {
		_, c, ctx := startScriptedCDP(t, map[string]string{"Page.navigate": reply})
		err := Navigate(ctx, c, "https://example.test/")
		switch {
		case wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", reply, err)
		case wantErr != "" && (err == nil || !strings.Contains(err.Error(), wantErr)):
			t.Errorf("%s: want an error naming %s, got %v", reply, wantErr, err)
		}
	}
}
