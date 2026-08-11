package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alexbeatnik/Manul/core/pkg/browser"
	"github.com/alexbeatnik/Manul/core/pkg/lifecycle"
	"github.com/alexbeatnik/Manul/core/pkg/runtime"
)

// Reverse calls: the engine asking the client to run something.
//
// Custom controls and CALL HOST handlers live in the client, not the engine —
// that is the whole point of them. When the engine is embedded in Go the
// handler is a Go func; when it is driven through a binding the handler is on
// the far side of the pipe, so the engine has to call back.
//
// The exchange is strictly nested inside a request the client is already
// waiting on. The client sends `run-step`, the engine reaches a registered
// control, writes an invoke line, and blocks; the client — which is sitting in
// its own read loop waiting for the run-step reply — sees the invoke, runs the
// handler, writes the result, and goes back to waiting. Neither side needs a
// second connection or a background thread.

// invokeRequest is written by the engine. The `invoke` key (rather than `id`)
// marks the direction, so neither side can mistake one for the other.
type invokeRequest struct {
	Invoke int    `json:"invoke"`
	Kind   string `json:"kind"`

	// custom_control
	Page   string `json:"page,omitempty"`
	Target string `json:"target,omitempty"`
	Action string `json:"action,omitempty"`
	Value  string `json:"value,omitempty"`
	Step   string `json:"step,omitempty"`

	// call
	Name string   `json:"name,omitempty"`
	Args []string `json:"args,omitempty"`

	// hook
	Hook string `json:"hook,omitempty"`
	Tag  string `json:"tag,omitempty"`

	Vars map[string]string `json:"vars,omitempty"`
}

// invokeReply is written by the client.
type invokeReply struct {
	Invoke *int            `json:"invoke"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  *wireError      `json:"error"`
}

const (
	kindCustomControl = "custom_control"
	kindCall          = "call"
	kindHook          = "hook"
)

// Suite-level hook names, as sent in invokeRequest.Hook.
const (
	HookBeforeAll   = "before_all"
	HookAfterAll    = "after_all"
	HookBeforeGroup = "before_group"
	HookAfterGroup  = "after_group"
)

// invoke writes one reverse call and waits for the client's answer.
//
// While waiting it also serves `page.*` requests against page. A handler in the
// embedded Go API is handed a live browser.Page and can evaluate against it;
// without this, the same handler written in Python could only decide things,
// never look at the page — the engine is mid-step and blocked, so the ordinary
// commands are not safe to re-enter, but these page primitives are.
func (s *Server) invoke(ctx context.Context, page browser.Page, req invokeRequest) (json.RawMessage, error) {
	s.invokeSeq++
	req.Invoke = s.invokeSeq

	if err := s.out.Encode(req); err != nil {
		return nil, fmt.Errorf("invoke %s: write: %w", req.Kind, err)
	}

	for {
		line, ok := s.readLine()
		if !ok {
			return nil, fmt.Errorf("invoke %s: client closed the stream while a handler was pending", req.Kind)
		}
		if line == "" {
			continue
		}

		var probe map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			return nil, fmt.Errorf("invoke %s: unreadable line: %w", req.Kind, err)
		}

		// A nested page request: serve it and keep waiting.
		if _, isRequest := probe["cmd"]; isRequest {
			if err := s.serveNested(ctx, page, []byte(line)); err != nil {
				return nil, err
			}
			continue
		}

		var rep invokeReply
		if err := json.Unmarshal([]byte(line), &rep); err != nil {
			return nil, fmt.Errorf("invoke %s: unreadable reply: %w", req.Kind, err)
		}
		if rep.Invoke == nil {
			return nil, fmt.Errorf(
				"invoke %s: expected a reply to invoke %d, got %s",
				req.Kind, req.Invoke, summarise(line))
		}
		if *rep.Invoke != req.Invoke {
			return nil, fmt.Errorf("invoke %s: reply id %d, want %d", req.Kind, *rep.Invoke, req.Invoke)
		}

		if !rep.OK {
			msg := "handler failed"
			if rep.Error != nil && rep.Error.Message != "" {
				msg = rep.Error.Message
			}
			return nil, fmt.Errorf("%s", msg)
		}
		return rep.Result, nil
	}
}

// serveNested answers the small set of commands a handler may issue while the
// engine is paused inside its step.
//
// Only `page.*` is allowed. Anything else — run-step, map, run — would re-enter
// the runtime that is currently executing, so it is refused with an error that
// says as much rather than deadlocking.
func (s *Server) serveNested(ctx context.Context, page browser.Page, line []byte) error {
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return s.fail(nil, CodeBadRequest, fmt.Sprintf("malformed request: %v", err))
	}

	if page == nil {
		return s.fail(req.ID, CodeInternal, "no page available to this handler")
	}

	switch req.Cmd {
	case "page.eval":
		var a struct {
			JS string `json:"js"`
		}
		if err := decodeArgs(req.Args, &a); err != nil {
			return s.fail(req.ID, CodeBadRequest, err.Error())
		}
		if strings.TrimSpace(a.JS) == "" {
			return s.fail(req.ID, CodeBadRequest, "page.eval needs js")
		}
		raw, err := page.EvalJS(ctx, a.JS)
		if err != nil {
			return s.fail(req.ID, CodeInternal, err.Error())
		}
		return s.reply(req.ID, decodeEvalResult(raw))

	case "page.url":
		url, err := page.CurrentURL(ctx)
		if err != nil {
			return s.fail(req.ID, CodeInternal, err.Error())
		}
		return s.reply(req.ID, url)
	}

	return s.fail(req.ID, CodeBadRequest, fmt.Sprintf(
		"%q is not available while a handler is running; only page.eval and page.url are", req.Cmd))
}

// decodeEvalResult turns the bytes EvalJS produced into a value the client can
// receive.
//
// EvalJS is not uniform: numbers, objects and booleans come back as JSON, but a
// string result arrives as its bare, unquoted text. Parsing strictly would turn
// every string the page produced into null — so unparseable bytes are taken as
// the text they are, which is the only thing they can be.
func decodeEvalResult(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw)
	}
	return decoded
}

func summarise(line string) string {
	const max = 120
	if len(line) > max {
		return line[:max] + "…"
	}
	return line
}

// ── registration ─────────────────────────────────────────────────────────────

type controlSpec struct {
	// Page is the page label the control belongs to. "*" matches any page.
	Page   string `json:"page"`
	Target string `json:"target"`
}

// hookSpec declares one suite-level hook the client owns.
type hookSpec struct {
	// Kind is before_all | after_all | before_group | after_group.
	Kind string `json:"kind"`
	// Tag is required for the group kinds and ignored for the others.
	Tag string `json:"tag"`
}

type registerArgs struct {
	Controls []controlSpec `json:"controls"`
	Calls    []string      `json:"calls"`
	Hooks    []hookSpec    `json:"hooks"`
}

type registerResult struct {
	Controls int `json:"controls"`
	Calls    int `json:"calls"`
	Hooks    int `json:"hooks"`
}

// cmdRegister tells the engine which custom controls and CALL handlers the
// client owns. Each one becomes a bridge that turns an engine-side invocation
// into a reverse call.
//
// Registration is deliberately allowed before `open`: handlers describe the
// client, not the browser, and a caller naturally declares them at import time.
func (s *Server) cmdRegister(_ context.Context, raw json.RawMessage) (any, string, error) {
	var a registerArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, CodeBadRequest, err
	}

	for _, spec := range a.Controls {
		page, target := strings.TrimSpace(spec.Page), strings.TrimSpace(spec.Target)
		if target == "" {
			return nil, CodeBadRequest, fmt.Errorf("custom control needs a target")
		}
		if page == "" {
			// An unqualified control applies everywhere, which is what a caller
			// who left the page out meant.
			page = "*"
		}
		if err := runtime.RegisterCustomControl(page, target, s.bridgeControl()); err != nil {
			return nil, CodeBadRequest, err
		}
	}

	for _, name := range a.Calls {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, CodeBadRequest, fmt.Errorf("CALL handler needs a name")
		}
		if err := runtime.RegisterGoCall(name, s.bridgeCall()); err != nil {
			return nil, CodeBadRequest, err
		}
	}

	for _, h := range a.Hooks {
		if err := s.registerHook(h); err != nil {
			return nil, CodeBadRequest, err
		}
	}

	return registerResult{
		Controls: len(a.Controls),
		Calls:    len(a.Calls),
		Hooks:    len(a.Hooks),
	}, "", nil
}

func (s *Server) registerHook(h hookSpec) error {
	kind := strings.ToLower(strings.TrimSpace(h.Kind))
	tag := strings.TrimSpace(h.Tag)

	switch kind {
	case HookBeforeAll:
		return lifecycle.RegisterBeforeAll(s.bridgeHook(kind, ""))
	case HookAfterAll:
		return lifecycle.RegisterAfterAll(s.bridgeHook(kind, ""))
	case HookBeforeGroup:
		return lifecycle.RegisterBeforeGroup(tag, s.bridgeHook(kind, tag))
	case HookAfterGroup:
		return lifecycle.RegisterAfterGroup(tag, s.bridgeHook(kind, tag))
	}
	return fmt.Errorf("unknown hook kind %q: want %s, %s, %s or %s",
		h.Kind, HookBeforeAll, HookAfterAll, HookBeforeGroup, HookAfterGroup)
}

// bridgeHook turns a suite-level hook into a reverse call.
//
// The variables a hook publishes travel back in its result: a returned object
// is merged into the suite's global context, which is how a Python
// `@before_all` seeds `{token}` for every hunt that follows.
func (s *Server) bridgeHook(kind, tag string) lifecycle.Handler {
	return func(ctx context.Context, g *lifecycle.GlobalContext) error {
		raw, err := s.invoke(ctx, s.currentPage(), invokeRequest{
			Kind: kindHook,
			Hook: kind,
			Tag:  tag,
			Vars: g.Vars(),
		})
		if err != nil {
			return err
		}
		if len(raw) == 0 || string(raw) == "null" {
			return nil
		}
		var published map[string]any
		if err := json.Unmarshal(raw, &published); err != nil {
			// A hook that returned something other than an object simply
			// published nothing; that is not an error.
			return nil
		}
		for k, v := range published {
			if strings.TrimSpace(k) != "" {
				g.SetVar(k, fmt.Sprint(v))
			}
		}
		return nil
	}
}

// currentPage is the page a hook may reach through page.eval, when a session is
// open. before-all runs before any browser exists, so it is legitimately nil.
func (s *Server) currentPage() browser.Page {
	if s.sess == nil {
		return nil
	}
	return s.sess.Page()
}

// bridgeControl turns an engine-side custom control invocation into a reverse
// call. The handler's job is the side effect, so only failure travels back.
func (s *Server) bridgeControl() runtime.CustomControlHandler {
	return func(ctx context.Context, page browser.Page, inv runtime.CustomControlInvocation) error {
		_, err := s.invoke(ctx, page, invokeRequest{
			Kind:   kindCustomControl,
			Page:   inv.Page,
			Target: inv.Target,
			Action: inv.ActionType,
			Value:  inv.Value,
			Step:   inv.Command.Raw,
			Vars:   inv.Variables,
		})
		return err
	}
}

// bridgeCall turns a CALL HOST invocation into a reverse call and hands the
// decoded result back to the runtime.
//
// A returned object becomes runtime variables and a scalar goes into `into
// {var}` — both are the engine's existing behaviour for a handler result, so
// decoding to `any` is what keeps the Python side equivalent to a Go one.
func (s *Server) bridgeCall() runtime.GoCallHandler {
	return func(ctx context.Context, inv runtime.GoCallInvocation) (any, error) {
		raw, err := s.invoke(ctx, inv.Page, invokeRequest{
			Kind: kindCall,
			Name: inv.Name,
			Args: inv.Args,
			Vars: inv.Variables,
		})
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 || string(raw) == "null" {
			return nil, nil
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, fmt.Errorf("CALL %s: undecodable result: %w", inv.Name, err)
		}
		return decoded, nil
	}
}
