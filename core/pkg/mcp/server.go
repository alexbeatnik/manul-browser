// Package mcp serves the engine to MCP clients — Claude Desktop, Claude Code
// and anything else that speaks the Model Context Protocol — over stdio.
//
// It is a protocol front, like pkg/serve: every tool is a thin call into
// agent.Session, and nothing about scoring, the DSL or the browser is decided
// here. Messages are JSON-RPC 2.0, one per line; stdout carries them and
// nothing else, and every log goes to stderr.
//
// The browser is opened by the first tool call, not at start-up, so a client
// can list the tools without a browser being started, and closed when the
// client goes away (end of input) — or reopened when its window was closed in
// the middle of a session.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/alexbeatnik/manul-browser/core/pkg/agent"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

// Protocol versions this server speaks, newest first. A client asking for one
// of them gets it; any other request is answered with the newest, as the
// specification asks, and the client decides whether it can go on.
var protocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// maxLineBytes bounds one message. A run_hunt script is the largest thing a
// client sends; ten megabytes of it is not a script.
const maxLineBytes = 10 << 20

// JSON-RPC error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// Backend is what the tools drive: an agent.Session, or a fake in tests.
type Backend interface {
	Step(ctx context.Context, instruction string) (agent.StepOutcome, error)
	Run(ctx context.Context, huntScript string) (agent.RunOutcome, error)
	Map(ctx context.Context, budget agent.MapBudget) (agent.PageMap, error)
	Read(ctx context.Context, target string) (agent.Value, error)
	ReadText(ctx context.Context, selector string) (string, error)
	Close() error
}

// Opener opens the browser session the tools drive. It is called by the first
// tool call, and again after the browser has gone away.
type Opener func(ctx context.Context) (Backend, error)

// Options configures Serve.
type Options struct {
	// EngineVersion is reported as serverInfo.version.
	EngineVersion string
	// Open starts or attaches to the browser.
	Open Opener
	// Logger sinks diagnostics; it must not write to stdout.
	Logger *utils.Logger
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type server struct {
	opts Options
	log  *utils.Logger

	outMu sync.Mutex
	out   io.Writer

	// calls run one at a time, in the order they arrived: one page, one
	// runtime, one goroutine.
	calls chan call

	mu      sync.Mutex
	cancels map[string]context.CancelFunc // in-flight or queued, by request id
	backend Backend
}

type call struct {
	id     json.RawMessage
	params json.RawMessage
	ctx    context.Context
}

// Serve speaks MCP on in/out until in ends or ctx is cancelled, then closes
// the browser session if one was opened.
func Serve(ctx context.Context, in io.Reader, out io.Writer, opts Options) error {
	log := opts.Logger
	if log == nil {
		log = utils.NewLoggerTo(io.Discard, nil)
	}
	s := &server{
		opts:    opts,
		log:     log,
		out:     out,
		calls:   make(chan call, 64),
		cancels: map[string]context.CancelFunc{},
	}

	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for c := range s.calls {
			s.runCall(c)
		}
	}()

	readErr := s.readLoop(ctx, in)

	// The client is gone: nothing still running or queued has anyone to
	// answer, so it is cancelled rather than finished.
	s.mu.Lock()
	for _, cancel := range s.cancels {
		cancel()
	}
	s.mu.Unlock()
	close(s.calls)
	<-workerDone

	s.mu.Lock()
	b := s.backend
	s.backend = nil
	s.mu.Unlock()
	if b != nil {
		if err := b.Close(); err != nil {
			s.log.Warn("mcp: close browser: %v", err)
		}
	}
	return readErr
}

func (s *server) readLoop(ctx context.Context, in io.Reader) error {
	r := bufio.NewReaderSize(in, 64*1024)
	for {
		if ctx.Err() != nil {
			return nil
		}
		line, err := readLine(r)
		if len(bytes.TrimSpace(line)) > 0 {
			s.handle(ctx, line)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// readLine reads one message. A line longer than maxLineBytes is answered as
// a parse error by handle, having been read to its end and dropped.
func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	tooLong := false
	for {
		chunk, err := r.ReadSlice('\n')
		if !tooLong {
			buf = append(buf, chunk...)
			if len(buf) > maxLineBytes {
				tooLong, buf = true, nil
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if tooLong {
			return []byte("\x00too long"), err
		}
		return bytes.TrimRight(buf, "\r\n"), err
	}
}

func (s *server) handle(ctx context.Context, line []byte) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		// A batch, which this protocol no longer has, is not an object either.
		s.send(response{ID: json.RawMessage("null"), Error: &rpcError{codeParseError, "parse error: not a JSON-RPC object"}})
		return
	}
	isNotification := len(req.ID) == 0
	if req.JSONRPC != "2.0" || req.Method == "" {
		if !isNotification {
			s.send(response{ID: req.ID, Error: &rpcError{codeInvalidRequest, "invalid request: want jsonrpc \"2.0\" and a method"}})
		}
		return
	}

	switch req.Method {
	case "initialize":
		s.reply(req.ID, s.initialize(req.Params))
	case "ping":
		s.reply(req.ID, struct{}{})
	case "tools/list":
		s.reply(req.ID, map[string]any{"tools": toolList()})
	case "tools/call":
		if isNotification {
			return
		}
		callCtx, cancel := context.WithCancel(ctx)
		s.mu.Lock()
		s.cancels[string(req.ID)] = cancel
		s.mu.Unlock()
		s.calls <- call{id: req.ID, params: req.Params, ctx: callCtx}
	case "notifications/cancelled":
		var p struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		if json.Unmarshal(req.Params, &p) == nil && len(p.RequestID) > 0 {
			s.mu.Lock()
			if cancel, ok := s.cancels[string(p.RequestID)]; ok {
				cancel()
			}
			s.mu.Unlock()
		}
	default:
		// notifications/initialized and any other notification need no
		// answer; an unknown request does.
		if !isNotification {
			s.send(response{ID: req.ID, Error: &rpcError{codeMethodNotFound, "method not found: " + req.Method}})
		}
	}
}

func (s *server) initialize(raw json.RawMessage) map[string]any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
		ClientInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}
	_ = json.Unmarshal(raw, &p)
	version := protocolVersions[0]
	for _, v := range protocolVersions {
		if v == p.ProtocolVersion {
			version = v
		}
	}
	s.log.Info("mcp: %s %s connected, protocol %s", p.ClientInfo.Name, p.ClientInfo.Version, version)
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo": map[string]any{
			"name":       "manul",
			"title":      "Manul Browser",
			"version":    s.opts.EngineVersion,
			"websiteUrl": "https://github.com/alexbeatnik/manul-browser",
		},
		"instructions": instructions,
	}
}

func (s *server) runCall(c call) {
	defer func() {
		s.mu.Lock()
		delete(s.cancels, string(c.id))
		s.mu.Unlock()
	}()
	// Cancelled while queued: the client asked for no answer.
	if c.ctx.Err() != nil {
		return
	}
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(c.params, &p); err != nil || p.Name == "" {
		s.send(response{ID: c.id, Error: &rpcError{codeInvalidParams, "tools/call needs a tool name"}})
		return
	}
	t, ok := toolByName(p.Name)
	if !ok {
		s.send(response{ID: c.id, Error: &rpcError{codeInvalidParams, "unknown tool: " + p.Name}})
		return
	}
	res := s.callTool(c.ctx, t, p.Arguments)
	if c.ctx.Err() != nil {
		return
	}
	s.reply(c.id, res)
}

// callTool runs a tool against the session, opening it first when there is
// none. A session whose browser went away — its window closed by the user,
// the process killed — is opened afresh once, and the tool run again.
func (s *server) callTool(ctx context.Context, t tool, args json.RawMessage) toolResult {
	for attempt := 0; ; attempt++ {
		b, err := s.session(ctx)
		if err != nil {
			return errorResult("Could not start the browser: " + err.Error())
		}
		res, err := t.run(ctx, b, args)
		if err != nil && browserGone(err) && attempt == 0 {
			s.log.Warn("mcp: the browser went away (%v); opening a new one", err)
			s.drop(b)
			continue
		}
		if err != nil && res.Content == nil {
			return errorResult(err.Error())
		}
		if attempt > 0 {
			res.Content = append([]content{text("Note: the browser had been closed, so a new one was opened; pages that were open are gone.")}, res.Content...)
		}
		return res
	}
}

func (s *server) session(ctx context.Context) (Backend, error) {
	s.mu.Lock()
	b := s.backend
	s.mu.Unlock()
	if b != nil {
		return b, nil
	}
	if s.opts.Open == nil {
		return nil, errors.New("no browser configured")
	}
	b, err := s.opts.Open(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.backend = b
	s.mu.Unlock()
	return b, nil
}

func (s *server) drop(b Backend) {
	s.mu.Lock()
	if s.backend == b {
		s.backend = nil
	}
	s.mu.Unlock()
	_ = b.Close()
}

// browserGone reports whether err is the transport telling that the browser
// is no longer there, rather than a step that failed on a live page.
func browserGone(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "cdp connection closed") || strings.Contains(msg, "bidi connection closed")
}

func (s *server) reply(id json.RawMessage, result any) {
	s.send(response{ID: id, Result: result})
}

func (s *server) send(r response) {
	r.JSONRPC = "2.0"
	b, err := json.Marshal(r)
	if err != nil {
		b, _ = json.Marshal(response{JSONRPC: "2.0", ID: r.ID, Error: &rpcError{codeInternalError, fmt.Sprintf("encode: %v", err)}})
	}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	_, _ = s.out.Write(append(b, '\n'))
}
