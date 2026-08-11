package agent

import (
	"context"
	"fmt"

	"github.com/alexbeatnik/Manul/core/pkg/browser"
)

// Page exposes the session's live page for callers that need the browser
// primitives directly — a suite-level hook or a custom control handler that has
// to reach the DOM while the runtime is mid-step, when the ordinary session
// methods would re-enter it. Nil once the session is closed.
func (s *Session) Page() browser.Page {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	return s.page
}

// SetGlobalVars seeds variables at global scope — where suite-level hooks
// publish. A hunt's own values still shadow them.
func (s *Session) SetGlobalVars(kv map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("agent: session closed")
	}
	s.rt.SetGlobalVars(kv)
	return nil
}

// Vars returns every variable currently visible to the session, flattened by
// DSL precedence.
//
// This is what makes a multi-step agent flow possible in one session: a value
// captured by `EXTRACT the 'Order total' into {total}` stays readable across
// later Step calls, which is exactly what a one-shot CLI invocation cannot do.
func (s *Session) Vars(ctx context.Context) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("agent: session closed")
	}
	return s.rt.Vars(), nil
}

// SetVars seeds variables before or between steps and returns the resulting
// snapshot. Values are set at mission scope, so per-row data still shadows them.
func (s *Session) SetVars(ctx context.Context, kv map[string]string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("agent: session closed")
	}
	for k, v := range kv {
		s.rt.SetVar(k, v)
	}
	return s.rt.Vars(), nil
}
