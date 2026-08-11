// Package serve implements `manul serve --stdio`: one long-lived process that
// owns a browser session and speaks the NDJSON protocol in spec/protocol.md.
//
// The one-shot agent commands already externalise browser state — each attaches
// to a running Chrome — but three things do not survive a process boundary: DSL
// variables written by EXTRACT, the scorer's cache channel, and the cost of
// attaching. A session keeps all three alive for its lifetime.
package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/alexbeatnik/manul-browser/core/pkg/agent"
	"github.com/alexbeatnik/manul-browser/core/pkg/config"
	"github.com/alexbeatnik/manul-browser/core/pkg/lifecycle"
	"github.com/alexbeatnik/manul-browser/core/pkg/runtime"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

// ProtocolVersion is the major.minor of the wire contract. Minor bumps add
// commands, args or result fields; clients must ignore unknown fields. Major
// bumps change existing shapes.
const ProtocolVersion = "1.0"

// maxLineBytes caps one request line, matching the extension protocol's 1 MB
// safety cap so a long hunt source cannot trip ErrTooLong.
const maxLineBytes = 1024 * 1024

// Error codes carried in a response's error.code.
const (
	CodeBadRequest  = "bad_request"
	CodeNotOpen     = "not_open"
	CodeAlreadyOpen = "already_open"
	CodeStepFailed  = "step_failed"
	CodeInternal    = "internal"
)

// errNoSession is the single wording for "you have not opened a session yet",
// so every command reports it identically.
var errNoSession = errors.New("no session; call open first")

// Request is one line of input.
type Request struct {
	// ID correlates the response. Null or absent is legal but leaves the
	// caller unable to match the reply, so bindings always send one.
	ID   *json.RawMessage `json:"id"`
	Cmd  string           `json:"cmd"`
	Args json.RawMessage  `json:"args,omitempty"`
}

type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	ID     *json.RawMessage `json:"id"`
	OK     bool             `json:"ok"`
	Result any              `json:"result,omitempty"`
	Error  *wireError       `json:"error,omitempty"`
}

// Options configures a server.
type Options struct {
	// EngineVersion is reported in the ready event.
	EngineVersion string
	// Schema supplies the payload for the `schema` command. Injected rather
	// than imported so this package stays independent of the CLI.
	Schema func() map[string]any
	// Config is the resolved engine configuration. It decides launch-vs-attach
	// unless a single `open` overrides it.
	Config config.Config
	// Logger sinks engine logs. These must never reach stdout, which carries
	// the protocol and nothing else.
	Logger *utils.Logger
}

// Server holds the session for the life of one stdio conversation.
type Server struct {
	opts Options
	out  *json.Encoder
	sess *agent.Session

	// sc is shared with the reverse-call path: a custom control handler runs
	// inside a request, so it reads its reply from the same stream the main
	// loop is otherwise blocked on. Dispatch is serial, so the two never
	// compete — while a handler waits, the client is waiting too, and the only
	// line it can legitimately send is that handler's reply.
	sc        *bufio.Scanner
	invokeSeq int

	// invokeMu serialises reverse calls. A stdio session never contends for it —
	// dispatch is serial there — but a hook peer is driven by `manul run`, where
	// group hooks fire from every goroutine in the worker pool at once. One pipe
	// cannot carry two overlapping exchanges, so they queue.
	invokeMu sync.Mutex
}

// NewPeer builds a Server that talks *down* to a child process instead of up to
// the client that spawned it.
//
// Same wire, opposite plumbing: in and out are the child's stdout and stdin, so
// a `register` line the child writes is read here exactly as a binding's would
// be. That is the whole reason `--hooks` costs so little — the reverse-call
// machinery already existed, only the direction of the pipes is new.
//
// Unlike Serve, this does not reset the extension registries when it finishes.
// The caller owns that, because in `manul run` the registrations must outlive
// the handshake and survive for the whole suite.
func NewPeer(in io.Reader, out io.Writer, opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = utils.NewLoggerTo(io.Discard, nil)
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 4096), maxLineBytes)
	return &Server{opts: opts, out: json.NewEncoder(out), sc: sc}
}

// Handshake reads a peer's declarations until it announces `ready`.
//
// Only `register` and `ready` are legal here. Everything else would need a
// browser that does not exist yet — the handshake runs before the first hunt,
// which is the point: registration has to be complete before the worker pool
// starts, exactly as it must be for a Go caller registering at process init.
func (s *Server) Handshake(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, ok := s.readLine()
		if !ok {
			if err := s.sc.Err(); err != nil {
				return fmt.Errorf("reading hook declarations: %w", err)
			}
			return errors.New("the hook script exited before it announced ready")
		}
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			return fmt.Errorf("the hook script wrote a line that is not a request: %s", summarise(line))
		}

		switch req.Cmd {
		case "register":
			result, code, err := s.cmdRegister(ctx, req.Args)
			if err != nil {
				// Answered as well as returned: the child is blocked on this
				// reply, and a peer that learns why it failed can say so on its
				// own stderr before it exits.
				_ = s.fail(req.ID, code, err.Error())
				return fmt.Errorf("hook registration rejected: %w", err)
			}
			if err := s.reply(req.ID, result); err != nil {
				return err
			}

		case "ready":
			return s.reply(req.ID, struct{}{})

		default:
			msg := fmt.Sprintf("%q is not available during the hook handshake; only register and ready are", req.Cmd)
			_ = s.fail(req.ID, CodeBadRequest, msg)
			return errors.New(msg)
		}
	}
}

// Serve runs the protocol loop until in is exhausted, `close` is acknowledged,
// or ctx is cancelled.
//
// Requests are handled serially and replies are written in the order received,
// so a caller may pipeline. Correlate by id regardless.
func Serve(ctx context.Context, in io.Reader, out io.Writer, opts Options) error {
	if opts.Logger == nil {
		opts.Logger = utils.NewLoggerTo(io.Discard, nil)
	}

	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 4096), maxLineBytes)

	s := &Server{opts: opts, out: json.NewEncoder(out), sc: sc}
	defer func() {
		if s.sess != nil {
			_ = s.sess.Close()
		}
		// Handlers bridged to this client must not outlive it: the registries
		// are process-global, and a stale bridge would try to talk down a
		// stream nobody is reading.
		runtime.ResetRuntimeRegistries()
		lifecycle.Reset()
	}()

	if err := s.emitReady(); err != nil {
		return err
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, ok := s.readLine()
		if !ok {
			return sc.Err()
		}
		if line == "" {
			continue
		}

		done, err := s.handleLine(ctx, []byte(line))
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

// readLine returns the next trimmed input line, or ok=false at end of input.
//
// A UTF-8 BOM ahead of the first request is common enough from shell pipelines
// and Windows editors that rejecting it would only ever look like a bug in the
// client.
func (s *Server) readLine() (string, bool) {
	if !s.sc.Scan() {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(s.sc.Text(), "\ufeff")), true
}

func (s *Server) emitReady() error {
	return s.out.Encode(map[string]any{
		"event":    "ready",
		"protocol": ProtocolVersion,
		"engine":   s.opts.EngineVersion,
	})
}

// handleLine parses and dispatches one request. It returns done=true after a
// successful `close`. A malformed line is answered, not fatal — the session
// stays usable.
func (s *Server) handleLine(ctx context.Context, line []byte) (bool, error) {
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return false, s.fail(nil, CodeBadRequest, fmt.Sprintf("malformed request: %v", err))
	}
	if req.Cmd == "" {
		return false, s.fail(req.ID, CodeBadRequest, "missing cmd")
	}

	if req.Cmd == "close" {
		if s.sess != nil {
			_ = s.sess.Close()
			s.sess = nil
		}
		return true, s.reply(req.ID, struct{}{})
	}

	result, code, err := s.dispatch(ctx, req)
	if err != nil {
		return false, s.fail(req.ID, code, err.Error())
	}
	return false, s.reply(req.ID, result)
}

func (s *Server) dispatch(ctx context.Context, req Request) (any, string, error) {
	switch req.Cmd {
	case "schema":
		if s.opts.Schema == nil {
			return nil, CodeInternal, errors.New("schema unavailable")
		}
		return s.opts.Schema(), "", nil

	case "open":
		return s.cmdOpen(ctx, req.Args)

	case "register":
		return s.cmdRegister(ctx, req.Args)

	case "map":
		return s.cmdMap(ctx, req.Args)

	case "read":
		return s.cmdRead(ctx, req.Args)

	case "run-step":
		return s.cmdRunStep(ctx, req.Args)

	case "run":
		return s.cmdRun(ctx, req.Args)

	case "run-suite":
		return s.cmdRunSuite(ctx, req.Args)

	case "vars":
		return s.cmdVars(ctx, req.Args)

	case "state":
		if s.sess == nil {
			return nil, CodeNotOpen, errNoSession
		}
		st, err := s.sess.PageState(ctx)
		if err != nil {
			return nil, CodeInternal, err
		}
		return st, "", nil
	}
	return nil, CodeBadRequest, fmt.Errorf("unknown cmd %q", req.Cmd)
}

// ── open ─────────────────────────────────────────────────────────────────────

type openArgs struct {
	// Mode overrides config for this session only: "launch" or "attach".
	Mode string `json:"mode"`
	// CDP is the endpoint to dial when attaching.
	CDP string `json:"cdp"`
	// Tab selects the first tab whose URL contains this substring (attach).
	Tab string `json:"tab"`
	// Headless applies to launch only. Pointer so "absent" differs from false.
	Headless       *bool  `json:"headless"`
	Port           int    `json:"port"`
	ExecutablePath string `json:"executablePath"`
}

type openResult struct {
	Mode string `json:"mode"`
	CDP  string `json:"cdp,omitempty"`
	URL  string `json:"url,omitempty"`
}

func (s *Server) cmdOpen(ctx context.Context, raw json.RawMessage) (any, string, error) {
	if s.sess != nil {
		return nil, CodeAlreadyOpen, errors.New("session already open")
	}

	var a openArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, CodeBadRequest, err
	}

	cfg := s.opts.Config
	// Per-session overrides sit above env and the JSON file, matching the
	// documented precedence: open args › env › config file › defaults.
	if a.CDP != "" {
		cfg.CDPEndpoint = a.CDP
	}
	if a.Mode != "" {
		cfg.BrowserMode = a.Mode
	}
	if a.Headless != nil {
		cfg.Headless = *a.Headless
	}
	if a.ExecutablePath != "" {
		cfg.ExecutablePath = &a.ExecutablePath
	}

	mode := cfg.ResolveBrowserMode()
	if a.Mode != "" && mode != strings.ToLower(a.Mode) {
		return nil, CodeBadRequest, fmt.Errorf("unknown mode %q: want %q or %q",
			a.Mode, config.ModeLaunch, config.ModeAttach)
	}
	if cfg.BrowserModeIsDeprecatedSpelling() {
		s.opts.Logger.Warn(`config: browser "electron" is a deprecated spelling of browser_mode "attach"`)
	}

	opts := agent.Options{
		Headless:       cfg.Headless,
		Port:           a.Port,
		ExecutablePath: derefOr(cfg.ExecutablePath, ""),
		Config:         &cfg,
		Logger:         s.opts.Logger,
	}

	var (
		sess *agent.Session
		err  error
		res  openResult
	)
	switch mode {
	case config.ModeAttach:
		endpoint := cfg.AttachEndpoint()
		sess, err = agent.Attach(ctx, endpoint, a.Tab, opts)
		res = openResult{Mode: config.ModeAttach, CDP: endpoint}
	default:
		sess, err = agent.Launch(ctx, opts)
		res = openResult{Mode: config.ModeLaunch}
	}
	if err != nil {
		return nil, CodeInternal, err
	}
	s.sess = sess

	if st, err := sess.PageState(ctx); err == nil {
		res.URL = st.URL
	}
	return res, "", nil
}

// ── page commands ────────────────────────────────────────────────────────────

type mapArgs struct {
	MaxPerGroup      int  `json:"maxPerGroup"`
	IncludeUnlabeled bool `json:"includeUnlabeled"`
}

func (s *Server) cmdMap(ctx context.Context, raw json.RawMessage) (any, string, error) {
	// Args are validated before the session is checked: a malformed request is
	// the caller's bug either way, and telling them to open a session first
	// would only send them back here for the same error.
	var a mapArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, CodeBadRequest, err
	}
	if s.sess == nil {
		return nil, CodeNotOpen, errNoSession
	}
	pm, err := s.sess.Map(ctx, agent.MapBudget{
		MaxPerGroup:      a.MaxPerGroup,
		IncludeUnlabeled: a.IncludeUnlabeled,
	})
	if err != nil {
		return nil, CodeInternal, err
	}
	return pm, "", nil
}

type readArgs struct {
	Label    string `json:"label"`
	Selector string `json:"selector"`
	MaxChars int    `json:"maxChars"`
}

type readValueResult struct {
	Value  string `json:"value"`
	Found  bool   `json:"found"`
	Reason string `json:"reason"`
}

type readTextResult struct {
	Text     string `json:"text"`
	Selector string `json:"selector"`
}

func (s *Server) cmdRead(ctx context.Context, raw json.RawMessage) (any, string, error) {
	var a readArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, CodeBadRequest, err
	}
	if s.sess == nil {
		return nil, CodeNotOpen, errNoSession
	}

	// Selector and label are two different reads with two different result
	// shapes, exactly as the CLI contract defines them.
	if a.Selector != "" {
		text, err := s.sess.ReadText(ctx, a.Selector)
		if err != nil {
			return nil, CodeInternal, err
		}
		if a.MaxChars > 0 && len(text) > a.MaxChars {
			text = text[:a.MaxChars]
		}
		return readTextResult{Text: text, Selector: a.Selector}, "", nil
	}
	if a.Label == "" {
		return nil, CodeBadRequest, errors.New("read needs either label or selector")
	}

	v, err := s.sess.Read(ctx, a.Label)
	if err != nil {
		return nil, CodeInternal, err
	}
	return readValueResult{Value: v.Text, Found: v.Found, Reason: string(v.Reason)}, "", nil
}

type runStepArgs struct {
	Step string `json:"step"`
}

func (s *Server) cmdRunStep(ctx context.Context, raw json.RawMessage) (any, string, error) {
	var a runStepArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, CodeBadRequest, err
	}
	if strings.TrimSpace(a.Step) == "" {
		return nil, CodeBadRequest, errors.New("run-step needs a step")
	}
	if s.sess == nil {
		return nil, CodeNotOpen, errNoSession
	}

	// A step that resolves nothing is a normal answer, not a transport fault:
	// the outcome carries ok=false and the caller stays in the session. Only
	// the outcome is returned, so the reply is ok=true with ok=false inside.
	outcome, _ := s.sess.Step(ctx, a.Step)
	return outcome, "", nil
}

type runArgs struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

func (s *Server) cmdRun(ctx context.Context, raw json.RawMessage) (any, string, error) {
	var a runArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, CodeBadRequest, err
	}
	if s.sess == nil {
		return nil, CodeNotOpen, errNoSession
	}

	source := a.Source
	if source == "" {
		if a.Path == "" {
			return nil, CodeBadRequest, errors.New("run needs either path or source")
		}
		b, err := os.ReadFile(a.Path)
		if err != nil {
			return nil, CodeBadRequest, err
		}
		source = string(b)
	}

	outcome, err := s.sess.Run(ctx, source)
	if err != nil {
		return nil, CodeStepFailed, err
	}
	return outcome, "", nil
}

type varsArgs struct {
	Set map[string]string `json:"set"`
	Get []string          `json:"get"`
}

func (s *Server) cmdVars(ctx context.Context, raw json.RawMessage) (any, string, error) {
	var a varsArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, CodeBadRequest, err
	}
	if s.sess == nil {
		return nil, CodeNotOpen, errNoSession
	}

	all := map[string]string{}
	var err error
	if len(a.Set) > 0 {
		all, err = s.sess.SetVars(ctx, a.Set)
	} else {
		all, err = s.sess.Vars(ctx)
	}
	if err != nil {
		return nil, CodeInternal, err
	}

	if len(a.Get) == 0 {
		return all, "", nil
	}
	// A projection still reports absent names, as empty strings, so a caller
	// never has to distinguish "missing key" from "missing variable".
	picked := make(map[string]string, len(a.Get))
	for _, k := range a.Get {
		picked[k] = all[k]
	}
	return picked, "", nil
}

// ── wire helpers ─────────────────────────────────────────────────────────────

func (s *Server) reply(id *json.RawMessage, result any) error {
	return s.out.Encode(response{ID: id, OK: true, Result: result})
}

func (s *Server) fail(id *json.RawMessage, code, msg string) error {
	return s.out.Encode(response{ID: id, OK: false, Error: &wireError{Code: code, Message: msg}})
}

// decodeArgs tolerates absent args so `{"cmd":"map"}` is as valid as
// `{"cmd":"map","args":{}}`.
func decodeArgs(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("bad args: %w", err)
	}
	return nil
}

func derefOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}
