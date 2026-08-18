// Package bidi provides a WebDriver BiDi client for Manul Browser.
//
// It is the Firefox counterpart of pkg/cdp: the same job (drive a browser over
// a WebSocket), a different wire protocol. Firefox removed its experimental CDP
// support in version 141, and `remote.active-protocols` went with it, so BiDi
// is the only protocol a current Firefox speaks. Chrome keeps using pkg/cdp.
//
// Three differences from CDP are worth knowing before reading further:
//
//   - One socket, many pages. CDP dials a WebSocket per target; BiDi has a
//     single browser-wide connection and addresses pages by browsing-context
//     id. A Page therefore carries (conn, contextID), not a private socket.
//   - A session must be created. Commands other than session.status and
//     session.new are refused until session.new succeeds — Connect does that.
//   - Events must be subscribed to server-side. Conn.Subscribe only opens a
//     local channel; the browser sends nothing until session.subscribe asks
//     for the event by name.
package bidi

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// cmd is an outgoing BiDi command. Unlike CDP, params is mandatory — a missing
// object is a protocol error, so callers that have no arguments send `{}`.
type cmd struct {
	ID     int64       `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

// message is an incoming BiDi message: a command result, a command error, or
// an event. Type says which.
type message struct {
	Type    string          `json:"type"` // "success" | "error" | "event"
	ID      int64           `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
	Message string          `json:"message,omitempty"`
}

// Method returns the event method name, or "" for command replies.
func (m *message) EventMethod() string { return m.Method }

// EventParams returns the raw event payload.
func (m *message) EventParams() json.RawMessage { return m.Params }

// eventChanCap is the buffer size of every event subscription channel.
// A slow subscriber that fails to drain will lose events past this depth
// (publisher uses non-blocking send), but will never block the read loop.
const eventChanCap = 64

// Conn is a live WebSocket connection to a browser's WebDriver BiDi endpoint.
// It is safe for concurrent use from multiple goroutines.
type Conn struct {
	wsURL string
	ws    *websocket.Conn

	// sessionID is set by NewSession. Firefox does not require it on the wire
	// (the socket is the session), but it is kept for diagnostics.
	sessionID string

	idSeq   atomic.Int64
	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[int64]chan *message

	subsMu    sync.Mutex
	subsSeq   uint64
	eventSubs map[uint64]chan *message

	closeOnce sync.Once

	ctx    context.Context
	cancel context.CancelFunc

	// parentCtx is the caller's Dial-time context. connCtx is deliberately
	// detached from it (so a short dial deadline doesn't kill a live conn),
	// but Call still honors parentCtx cancellation directly — see the same
	// reasoning in pkg/cdp/conn.go.
	parentCtx context.Context
}

// Subscription is a handle to a local event subscription. The caller must
// invoke Close() when done — typically via `defer sub.Close()`.
//
// Opening one does not make the browser send anything: BiDi events are
// off by default. Pair it with SubscribeEvents for the methods you want.
type Subscription struct {
	id   uint64
	ch   chan *message
	conn *Conn
	once sync.Once
}

// C returns the receive channel for this subscription. The channel is closed
// by the publisher when the connection terminates or Close() is called.
func (s *Subscription) C() <-chan *message {
	if s == nil {
		return nil
	}
	return s.ch
}

// Close releases the subscription. Safe to call multiple times.
func (s *Subscription) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.conn.unsubscribe(s.id)
	})
}

// Dial establishes a WebSocket connection to a BiDi endpoint. It performs no
// handshake; call NewSession (or use Connect) before issuing other commands.
func Dial(ctx context.Context, wsURL string) (*Conn, error) {
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("websocket dial %s: %w", wsURL, err)
	}

	connCtx, cancel := context.WithCancel(context.Background())
	c := &Conn{
		wsURL:     wsURL,
		ws:        ws,
		pending:   make(map[int64]chan *message),
		eventSubs: make(map[uint64]chan *message),
		ctx:       connCtx,
		cancel:    cancel,
		parentCtx: ctx,
	}

	// Watch for parent ctx cancellation: tear down the connection.
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Close()
		case <-connCtx.Done():
		}
	}()

	go c.readLoop()
	return c, nil
}

// URL returns the WebSocket URL this connection was dialed on.
func (c *Conn) URL() string { return c.wsURL }

// Closed reports whether the connection has been torn down — by Close, by the
// parent context, or by the browser going away underneath it.
func (c *Conn) Closed() bool {
	if c == nil {
		return true
	}
	select {
	case <-c.ctx.Done():
		return true
	default:
		return false
	}
}

// SessionID returns the WebDriver session id, once NewSession has run.
func (c *Conn) SessionID() string { return c.sessionID }

// Close terminates the WebSocket connection. Safe to call multiple times.
func (c *Conn) Close() error {
	if c == nil {
		return nil
	}
	var closeErr error
	c.closeOnce.Do(func() {
		c.cancel()
		if c.ws != nil {
			closeErr = c.ws.Close()
		}
		// Pending waiters are released by Call's `<-c.ctx.Done()` branch;
		// closing their channels here would race readLoop's send.
		c.subsMu.Lock()
		for id, ch := range c.eventSubs {
			close(ch)
			delete(c.eventSubs, id)
		}
		c.subsMu.Unlock()
	})
	return closeErr
}

func (c *Conn) readLoop() {
	defer c.Close()
	for {
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		select {
		case <-c.ctx.Done():
			return
		default:
		}
		var msg message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		// Command replies carry an id; events do not. A BiDi error reply
		// carries both an id and type "error", so route on the id first.
		if msg.ID != 0 && msg.Type != "event" {
			c.pendingMu.Lock()
			ch, ok := c.pending[msg.ID]
			if ok {
				delete(c.pending, msg.ID)
			}
			c.pendingMu.Unlock()
			if ok {
				ch <- &msg
			}
			continue
		}
		if msg.Method != "" {
			c.subsMu.Lock()
			for _, sub := range c.eventSubs {
				select {
				case sub <- &msg:
				default:
				}
			}
			c.subsMu.Unlock()
		}
	}
}

// Subscribe returns a handle to a local event subscription. The caller MUST
// invoke Close() on it when done. See SubscribeEvents for the server side.
func (c *Conn) Subscribe() *Subscription {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	select {
	case <-c.ctx.Done():
		ch := make(chan *message)
		close(ch)
		return &Subscription{ch: ch, conn: c}
	default:
	}
	c.subsSeq++
	id := c.subsSeq
	ch := make(chan *message, eventChanCap)
	c.eventSubs[id] = ch
	return &Subscription{id: id, ch: ch, conn: c}
}

func (c *Conn) unsubscribe(id uint64) {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	if ch, ok := c.eventSubs[id]; ok {
		delete(c.eventSubs, id)
		close(ch)
	}
}

// Call sends a BiDi command and waits for its reply.
func (c *Conn) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	if c.parentCtx != nil {
		if err := c.parentCtx.Err(); err != nil {
			return nil, fmt.Errorf("bidi connection closed: %w", err)
		}
	}
	if params == nil {
		params = struct{}{}
	}

	id := c.idSeq.Add(1)
	ch := make(chan *message, 1)

	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	c.writeMu.Lock()
	err := c.ws.WriteJSON(cmd{ID: id, Method: method, Params: params})
	c.writeMu.Unlock()
	if err != nil {
		c.deletePending(id)
		return nil, fmt.Errorf("bidi write: %w", err)
	}

	select {
	case <-ctx.Done():
		c.deletePending(id)
		return nil, ctx.Err()
	case <-c.parentCtx.Done():
		c.deletePending(id)
		return nil, fmt.Errorf("bidi connection closed")
	case <-c.ctx.Done():
		c.deletePending(id)
		return nil, fmt.Errorf("bidi connection closed")
	case msg := <-ch:
		if msg.Type == "error" || msg.Error != "" {
			return nil, fmt.Errorf("bidi error: %s: %s (%s)", msg.Error, msg.Message, method)
		}
		return msg.Result, nil
	}
}

func (c *Conn) deletePending(id int64) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}
