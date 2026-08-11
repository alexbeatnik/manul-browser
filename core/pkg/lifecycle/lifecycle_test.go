package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func reset(t *testing.T) {
	t.Helper()
	Reset()
	t.Cleanup(Reset)
}

func noop(context.Context, *GlobalContext) error { return nil }

// ── registration ─────────────────────────────────────────────────────────────

func TestIsEmpty(t *testing.T) {
	reset(t)
	if !IsEmpty() {
		t.Fatal("a fresh registry should be empty")
	}
	_ = RegisterBeforeAll(noop)
	if IsEmpty() {
		t.Error("registry should not be empty after registering")
	}
}

func TestRegistrationRejectsNilAndBlanks(t *testing.T) {
	reset(t)
	if RegisterBeforeAll(nil) == nil {
		t.Error("a nil before-all handler should be rejected")
	}
	if RegisterAfterAll(nil) == nil {
		t.Error("a nil after-all handler should be rejected")
	}
	if RegisterBeforeGroup("smoke", nil) == nil {
		t.Error("a nil before-group handler should be rejected")
	}
	if RegisterBeforeGroup("   ", noop) == nil {
		t.Error("a blank tag should be rejected")
	}
	if RegisterAfterGroup("", noop) == nil {
		t.Error("an empty tag should be rejected")
	}
}

func TestTagsAreReported(t *testing.T) {
	reset(t)
	_ = RegisterBeforeGroup("smoke", noop)
	_ = RegisterAfterGroup("Regression", noop)
	_ = RegisterBeforeGroup("smoke", noop) // duplicate tag, listed once

	got := Tags()
	if strings.Join(got, ",") != "regression,smoke" {
		t.Errorf("Tags() = %v, want [regression smoke]", got)
	}
}

// ── before-all ───────────────────────────────────────────────────────────────

func TestBeforeAllRunsInRegistrationOrder(t *testing.T) {
	reset(t)
	var order []string
	_ = RegisterBeforeAll(func(context.Context, *GlobalContext) error {
		order = append(order, "first")
		return nil
	})
	_ = RegisterBeforeAll(func(context.Context, *GlobalContext) error {
		order = append(order, "second")
		return nil
	})

	if err := RunBeforeAll(context.Background(), NewGlobalContext()); err != nil {
		t.Fatalf("RunBeforeAll: %v", err)
	}
	if strings.Join(order, ",") != "first,second" {
		t.Errorf("order = %v", order)
	}
}

// A failed precondition must stop the suite, not merely be noted.
func TestBeforeAllStopsAtTheFirstFailure(t *testing.T) {
	reset(t)
	ran := 0
	_ = RegisterBeforeAll(func(context.Context, *GlobalContext) error {
		ran++
		return errors.New("no credentials")
	})
	_ = RegisterBeforeAll(func(context.Context, *GlobalContext) error {
		ran++
		return nil
	})

	err := RunBeforeAll(context.Background(), NewGlobalContext())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "no credentials") {
		t.Errorf("error should name the cause: %v", err)
	}
	if ran != 1 {
		t.Errorf("ran %d hooks, want 1 — later hooks must be skipped", ran)
	}
}

// ── after-all ────────────────────────────────────────────────────────────────

// Cleanup that stops halfway leaves more behind than it saves, so every
// after-all hook runs even when an earlier one fails.
func TestAfterAllRunsEveryHookDespiteFailures(t *testing.T) {
	reset(t)
	ran := 0
	_ = RegisterAfterAll(func(context.Context, *GlobalContext) error {
		ran++
		return errors.New("first teardown broke")
	})
	_ = RegisterAfterAll(func(context.Context, *GlobalContext) error {
		ran++
		return nil
	})
	_ = RegisterAfterAll(func(context.Context, *GlobalContext) error {
		ran++
		return errors.New("third teardown broke")
	})

	errs := RunAfterAll(context.Background(), NewGlobalContext())
	if ran != 3 {
		t.Errorf("ran %d hooks, want all 3", ran)
	}
	if len(errs) != 2 {
		t.Fatalf("got %d errors, want 2: %v", len(errs), errs)
	}
}

// ── group hooks ──────────────────────────────────────────────────────────────

func TestGroupHooksOnlyFireForMatchingTags(t *testing.T) {
	reset(t)
	var fired []string
	_ = RegisterBeforeGroup("smoke", func(context.Context, *GlobalContext) error {
		fired = append(fired, "smoke")
		return nil
	})
	_ = RegisterBeforeGroup("slow", func(context.Context, *GlobalContext) error {
		fired = append(fired, "slow")
		return nil
	})

	if err := RunBeforeGroup(context.Background(), []string{"smoke"}, NewGlobalContext()); err != nil {
		t.Fatalf("RunBeforeGroup: %v", err)
	}
	if strings.Join(fired, ",") != "smoke" {
		t.Errorf("fired = %v, want only smoke", fired)
	}
}

func TestGroupTagMatchingIgnoresCaseAndSpace(t *testing.T) {
	reset(t)
	fired := false
	_ = RegisterBeforeGroup("  Smoke ", func(context.Context, *GlobalContext) error {
		fired = true
		return nil
	})

	if err := RunBeforeGroup(context.Background(), []string{"SMOKE"}, NewGlobalContext()); err != nil {
		t.Fatalf("RunBeforeGroup: %v", err)
	}
	if !fired {
		t.Error("tag matching should ignore case and surrounding space")
	}
}

func TestUntaggedHuntFiresNoGroupHooks(t *testing.T) {
	reset(t)
	fired := false
	_ = RegisterBeforeGroup("smoke", func(context.Context, *GlobalContext) error {
		fired = true
		return nil
	})

	if err := RunBeforeGroup(context.Background(), nil, NewGlobalContext()); err != nil {
		t.Fatalf("RunBeforeGroup: %v", err)
	}
	if fired {
		t.Error("a hunt with no tags must not fire group hooks")
	}
}

func TestBeforeGroupStopsAtFirstFailure(t *testing.T) {
	reset(t)
	ran := 0
	_ = RegisterBeforeGroup("smoke", func(context.Context, *GlobalContext) error {
		ran++
		return errors.New("fixture missing")
	})
	_ = RegisterBeforeGroup("smoke", func(context.Context, *GlobalContext) error {
		ran++
		return nil
	})

	err := RunBeforeGroup(context.Background(), []string{"smoke"}, NewGlobalContext())
	if err == nil || ran != 1 {
		t.Errorf("err=%v ran=%d; want an error after exactly 1 hook", err, ran)
	}
}

func TestAfterGroupRunsEveryMatchingHook(t *testing.T) {
	reset(t)
	ran := 0
	for i := 0; i < 3; i++ {
		_ = RegisterAfterGroup("smoke", func(context.Context, *GlobalContext) error {
			ran++
			return errors.New("boom")
		})
	}

	errs := RunAfterGroup(context.Background(), []string{"smoke"}, NewGlobalContext())
	if ran != 3 || len(errs) != 3 {
		t.Errorf("ran=%d errs=%d; want 3 and 3", ran, len(errs))
	}
}

// ── global context ───────────────────────────────────────────────────────────

func TestGlobalContextVarsAreCopied(t *testing.T) {
	g := NewGlobalContext()
	g.SetVar("token", "abc")

	snapshot := g.Vars()
	snapshot["token"] = "tampered"

	if g.Vars()["token"] != "abc" {
		t.Error("Vars() must hand back a copy, not the live map")
	}
}

func TestMetadataIsSeparateFromVars(t *testing.T) {
	g := NewGlobalContext()
	g.SetMeta("conn", 42)
	g.SetVar("token", "abc")

	if _, present := g.Vars()["conn"]; present {
		t.Error("metadata must not leak into the variables a hunt sees")
	}
	v, ok := g.Meta("conn")
	if !ok || v != 42 {
		t.Errorf("Meta = %v, %v", v, ok)
	}
}

func TestHooksSeeEachOthersState(t *testing.T) {
	reset(t)
	_ = RegisterBeforeAll(func(_ context.Context, g *GlobalContext) error {
		g.SetVar("token", "abc123")
		return nil
	})
	_ = RegisterBeforeAll(func(_ context.Context, g *GlobalContext) error {
		if g.Vars()["token"] != "abc123" {
			return errors.New("earlier hook's value not visible")
		}
		return nil
	})

	if err := RunBeforeAll(context.Background(), NewGlobalContext()); err != nil {
		t.Fatalf("hooks should share one context: %v", err)
	}
}
