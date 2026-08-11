package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alexbeatnik/Manul/core/pkg/dsl"
	"github.com/alexbeatnik/Manul/core/pkg/lifecycle"
)

// run-suite runs several hunts in one session with the suite lifecycle applied.
//
// A client could loop over `run` itself, but then before-all/after-all would be
// its own invention and `@tags:` — which only the engine parses — could not
// select group hooks at all. The suite belongs to the engine for the same
// reason the hooks do.

type suiteArgs struct {
	// Paths are .hunt files to run, in order.
	Paths []string `json:"paths"`
}

// SuiteHunt is one hunt's outcome within a suite.
type SuiteHunt struct {
	Path string `json:"path"`
	// Skipped is set when a before-group hook refused this hunt. The rest of
	// the suite still runs.
	Skipped bool     `json:"skipped,omitempty"`
	OK      bool     `json:"ok"`
	Tags    []string `json:"tags,omitempty"`
	Steps   int      `json:"steps,omitempty"`
	Passed  int      `json:"passed,omitempty"`
	Failed  int      `json:"failed,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// SuiteResult is what `run-suite` answers with.
type SuiteResult struct {
	OK      bool              `json:"ok"`
	Total   int               `json:"total"`
	Passed  int               `json:"passed"`
	Failed  int               `json:"failed"`
	Skipped int               `json:"skipped"`
	Hunts   []SuiteHunt       `json:"hunts"`
	Vars    map[string]string `json:"vars,omitempty"`
}

func (s *Server) cmdRunSuite(ctx context.Context, raw json.RawMessage) (any, string, error) {
	var a suiteArgs
	if err := decodeArgs(raw, &a); err != nil {
		return nil, CodeBadRequest, err
	}
	if len(a.Paths) == 0 {
		return nil, CodeBadRequest, fmt.Errorf("run-suite needs at least one path")
	}
	if s.sess == nil {
		return nil, CodeNotOpen, errNoSession
	}

	// Parse everything up front. A suite that cannot be read should say so
	// before before-all does anything with side effects.
	type parsed struct {
		path   string
		source string
		tags   []string
	}
	hunts := make([]parsed, 0, len(a.Paths))
	for _, p := range a.Paths {
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, CodeBadRequest, fmt.Errorf("read %s: %w", p, err)
		}
		hunt, err := dsl.Parse(strings.NewReader(string(src)))
		if err != nil {
			return nil, CodeBadRequest, fmt.Errorf("parse %s: %w", p, err)
		}
		hunts = append(hunts, parsed{path: p, source: string(src), tags: hunt.Tags})
	}

	gctx := lifecycle.NewGlobalContext()

	if err := lifecycle.RunBeforeAll(ctx, gctx); err != nil {
		// Preconditions failed, so nothing runs. after-all still does: a
		// before-all that got halfway may have left something to clean up.
		s.reportHookErrors(lifecycle.RunAfterAll(ctx, gctx))
		return nil, CodeStepFailed, fmt.Errorf("suite aborted: %w", err)
	}
	defer func() {
		s.reportHookErrors(lifecycle.RunAfterAll(ctx, gctx))
	}()

	res := SuiteResult{OK: true, Total: len(hunts)}

	for _, h := range hunts {
		entry := SuiteHunt{Path: h.path, Tags: h.tags}

		if err := lifecycle.RunBeforeGroup(ctx, h.tags, gctx); err != nil {
			entry.Skipped = true
			entry.Error = err.Error()
			res.Skipped++
			res.OK = false
			res.Hunts = append(res.Hunts, entry)
			continue
		}

		// Whatever before-all and the group hooks published is visible to this
		// hunt at global scope, so its own values still win.
		if err := s.sess.SetGlobalVars(gctx.Vars()); err != nil {
			return nil, CodeInternal, err
		}

		outcome, err := s.sess.Run(ctx, h.source)
		entry.OK = err == nil && outcome.OK
		entry.Steps = outcome.TotalSteps
		entry.Passed = outcome.Passed
		entry.Failed = outcome.Failed
		if err != nil {
			entry.Error = err.Error()
		}
		if entry.OK {
			res.Passed++
		} else {
			res.Failed++
			res.OK = false
		}

		s.reportHookErrors(lifecycle.RunAfterGroup(ctx, h.tags, gctx))
		res.Hunts = append(res.Hunts, entry)
	}

	res.Vars = gctx.Vars()
	return res, "", nil
}

// reportHookErrors surfaces teardown failures without letting them change a
// result. Cleanup that reports a problem is still better than cleanup skipped.
func (s *Server) reportHookErrors(errs []error) {
	for _, err := range errs {
		s.opts.Logger.Warn("%v", err)
	}
}
