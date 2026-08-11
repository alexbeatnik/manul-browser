// Package lifecycle implements suite-level hooks: code that runs once around a
// whole run, or around every hunt carrying a given tag.
//
// A hunt file's own [SETUP] / [TEARDOWN] blocks cover one file. These cover the
// run — logging in once for twenty hunts, seeding a fixture, tearing down an
// environment afterwards. They live here rather than in a client because the
// engine is what knows when a suite begins, which files it contains, and what
// tags each one carries.
//
// Failure behaviour differs per hook, deliberately:
//
//   - before-all failing aborts the run. Its whole job is to establish
//     preconditions; continuing without them wastes the suite.
//   - before-group failing skips the hunts in that group, and only those.
//   - after-all and after-group are cleanup. They always run, every one of them
//     even if an earlier one failed, and their errors are reported but change
//     no result — cleanup that reports failure is still better than cleanup
//     that was skipped.
package lifecycle

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// GlobalContext is the state shared across a whole suite.
type GlobalContext struct {
	mu sync.Mutex
	// variables are published to every hunt as {placeholder} values at global
	// scope, so anything a hunt sets for itself still wins.
	variables map[string]string
	// metadata is scratch space passed between hooks. It never reaches a hunt.
	metadata map[string]any
}

func NewGlobalContext() *GlobalContext {
	return &GlobalContext{
		variables: map[string]string{},
		metadata:  map[string]any{},
	}
}

// SetVar publishes a variable to every hunt in the suite.
func (g *GlobalContext) SetVar(name, value string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.variables[name] = value
}

// Vars returns a copy of the published variables.
func (g *GlobalContext) Vars() map[string]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]string, len(g.variables))
	for k, v := range g.variables {
		out[k] = v
	}
	return out
}

// SetMeta stores hook-to-hook scratch state, never seen by a hunt.
func (g *GlobalContext) SetMeta(key string, value any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.metadata[key] = value
}

func (g *GlobalContext) Meta(key string) (any, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	v, ok := g.metadata[key]
	return v, ok
}

// Handler is one registered hook.
type Handler func(context.Context, *GlobalContext) error

type groupHook struct {
	tag     string
	handler Handler
}

// REGISTRY POLICY: as with the custom-control registries, registration is
// expected at process init and lookup during the run. Registering while a suite
// is executing is race-free but makes visibility timing-dependent.
var (
	mu          sync.RWMutex
	beforeAll   []Handler
	afterAll    []Handler
	beforeGroup []groupHook
	afterGroup  []groupHook
)

func RegisterBeforeAll(h Handler) error {
	if h == nil {
		return fmt.Errorf("lifecycle: before-all needs a handler")
	}
	mu.Lock()
	defer mu.Unlock()
	beforeAll = append(beforeAll, h)
	return nil
}

func RegisterAfterAll(h Handler) error {
	if h == nil {
		return fmt.Errorf("lifecycle: after-all needs a handler")
	}
	mu.Lock()
	defer mu.Unlock()
	afterAll = append(afterAll, h)
	return nil
}

func RegisterBeforeGroup(tag string, h Handler) error {
	key := normaliseTag(tag)
	if key == "" {
		return fmt.Errorf("lifecycle: before-group needs a tag")
	}
	if h == nil {
		return fmt.Errorf("lifecycle: before-group needs a handler")
	}
	mu.Lock()
	defer mu.Unlock()
	beforeGroup = append(beforeGroup, groupHook{tag: key, handler: h})
	return nil
}

func RegisterAfterGroup(tag string, h Handler) error {
	key := normaliseTag(tag)
	if key == "" {
		return fmt.Errorf("lifecycle: after-group needs a tag")
	}
	if h == nil {
		return fmt.Errorf("lifecycle: after-group needs a handler")
	}
	mu.Lock()
	defer mu.Unlock()
	afterGroup = append(afterGroup, groupHook{tag: key, handler: h})
	return nil
}

// IsEmpty reports whether anything is registered, so a plain run can skip the
// suite machinery entirely.
func IsEmpty() bool {
	mu.RLock()
	defer mu.RUnlock()
	return len(beforeAll)+len(afterAll)+len(beforeGroup)+len(afterGroup) == 0
}

// Reset clears every registration. For test fixtures; must not be called while
// a suite is running.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	beforeAll, afterAll, beforeGroup, afterGroup = nil, nil, nil, nil
}

// Tags lists every tag any group hook is registered for, sorted. Callers use it
// to warn about hooks that will never fire.
func Tags() []string {
	mu.RLock()
	defer mu.RUnlock()
	seen := map[string]bool{}
	for _, h := range append(append([]groupHook{}, beforeGroup...), afterGroup...) {
		seen[h.tag] = true
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// RunBeforeAll executes every before-all hook in registration order, stopping
// at the first failure — the suite must not proceed on a broken precondition.
func RunBeforeAll(ctx context.Context, g *GlobalContext) error {
	mu.RLock()
	hooks := append([]Handler{}, beforeAll...)
	mu.RUnlock()

	for i, h := range hooks {
		if err := h(ctx, g); err != nil {
			return fmt.Errorf("before-all hook %d: %w", i+1, err)
		}
	}
	return nil
}

// RunAfterAll executes every after-all hook, and does not stop for a failure:
// skipping the rest of teardown because one part of it failed leaves more
// behind than it saves. Errors are collected and returned together.
func RunAfterAll(ctx context.Context, g *GlobalContext) []error {
	mu.RLock()
	hooks := append([]Handler{}, afterAll...)
	mu.RUnlock()

	var errs []error
	for i, h := range hooks {
		if err := h(ctx, g); err != nil {
			errs = append(errs, fmt.Errorf("after-all hook %d: %w", i+1, err))
		}
	}
	return errs
}

// RunBeforeGroup executes the before-group hooks whose tag appears in tags,
// stopping at the first failure so the caller can skip the hunt.
func RunBeforeGroup(ctx context.Context, tags []string, g *GlobalContext) error {
	for _, h := range matching(beforeGroupHooks(), tags) {
		if err := h.handler(ctx, g); err != nil {
			return fmt.Errorf("before-group %q: %w", h.tag, err)
		}
	}
	return nil
}

// RunAfterGroup executes every matching after-group hook, failures and all.
func RunAfterGroup(ctx context.Context, tags []string, g *GlobalContext) []error {
	var errs []error
	for _, h := range matching(afterGroupHooks(), tags) {
		if err := h.handler(ctx, g); err != nil {
			errs = append(errs, fmt.Errorf("after-group %q: %w", h.tag, err))
		}
	}
	return errs
}

func beforeGroupHooks() []groupHook {
	mu.RLock()
	defer mu.RUnlock()
	return append([]groupHook{}, beforeGroup...)
}

func afterGroupHooks() []groupHook {
	mu.RLock()
	defer mu.RUnlock()
	return append([]groupHook{}, afterGroup...)
}

// matching keeps the hooks whose tag the hunt carries, in registration order.
func matching(hooks []groupHook, tags []string) []groupHook {
	if len(hooks) == 0 || len(tags) == 0 {
		return nil
	}
	have := make(map[string]bool, len(tags))
	for _, t := range tags {
		if key := normaliseTag(t); key != "" {
			have[key] = true
		}
	}
	var out []groupHook
	for _, h := range hooks {
		if have[h.tag] {
			out = append(out, h)
		}
	}
	return out
}

// normaliseTag matches how tags are compared elsewhere: case- and
// whitespace-insensitive, so `@tags: Smoke` and `tag="smoke"` are one group.
func normaliseTag(tag string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(tag))), " ")
}
