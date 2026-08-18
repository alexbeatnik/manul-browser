"""Suite-level lifecycle hooks.

Covers the `@before_all` / `@after_all` /
`@before_group` / `@after_group`. The suite now lives in the engine — it is what
knows which files a run contains and what `@tags:` each carries — so the hooks
are declared here and called back over the protocol.
"""

from __future__ import annotations

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import manul  # noqa: E402
from manul import controls  # noqa: E402

FAKE = [sys.executable, str(Path(__file__).resolve().parent / "fake_engine.py")]

# The fake engine reads tags from the file name: "name@tag,tag.hunt".
SMOKE = "checkout@smoke.hunt"
SLOW = "report@slow.hunt"
PLAIN = "plain.hunt"
FAILING = "fail@smoke.hunt"


@pytest.fixture(autouse=True)
def clean_registry():
    controls.reset_registry()
    yield
    controls.reset_registry()


# ── registration ─────────────────────────────────────────────────────────────

def test_hooks_are_declared_one_slot_per_kind_and_tag():
    @manul.before_all
    def a(ctx):
        pass

    # Two handlers in one slot must still be a single declaration: the engine
    # registers one bridge per slot and this side runs both when it fires.
    @manul.before_all
    def b(ctx):
        pass

    @manul.before_group("smoke")
    def c(ctx):
        pass

    assert manul.list_hooks() == [
        {"kind": "before_all", "tag": ""},
        {"kind": "before_group", "tag": "smoke"},
    ]


def test_group_decorators_require_a_tag():
    with pytest.raises(ValueError):
        manul.before_group("  ")
    with pytest.raises(ValueError):
        manul.after_group("")


def test_hook_signature_is_enforced():
    with pytest.raises(TypeError):
        @manul.before_all
        def bad(a, b):
            pass


def test_hooks_are_published_on_open():
    @manul.before_all
    def a(ctx):
        pass

    @manul.after_group("smoke")
    def b(ctx):
        pass

    with manul.Session(binary=FAKE) as s:
        assert s.published["hooks"] == 2


# ── before_all / after_all ───────────────────────────────────────────────────

def test_before_all_runs_once_before_any_hunt():
    order = []

    @manul.before_all
    def setup(ctx):
        order.append("before_all")

    with manul.Session(binary=FAKE) as s:
        res = s.run_suite([PLAIN, SLOW])
        assert res.ok
        assert order == ["before_all"], "before_all must run exactly once"
        assert res.total == 2 and res.passed == 2


def test_after_all_runs_once_at_the_end():
    order = []

    @manul.before_all
    def setup(ctx):
        order.append("before")

    @manul.after_all
    def teardown(ctx):
        order.append("after")

    with manul.Session(binary=FAKE) as s:
        s.run_suite([PLAIN])
    assert order == ["before", "after"]


def test_variables_published_by_before_all_reach_the_suite():
    @manul.before_all
    def login(ctx):
        ctx.set("token", "abc123")

    with manul.Session(binary=FAKE) as s:
        res = s.run_suite([PLAIN])
        assert res.vars["token"] == "abc123"


def test_all_handlers_in_a_slot_run_and_share_one_context():
    @manul.before_all
    def first(ctx):
        ctx.set("stage", "one")

    @manul.before_all
    def second(ctx):
        # The earlier hook's value must already be visible.
        assert ctx.variables["stage"] == "one"
        ctx.set("stage", "two")

    with manul.Session(binary=FAKE) as s:
        res = s.run_suite([PLAIN])
        assert res.vars["stage"] == "two"


# A broken precondition must stop the suite, not merely be noted.
def test_before_all_failure_aborts_the_suite():
    ran = []

    @manul.before_all
    def setup(ctx):
        raise RuntimeError("no credentials")

    @manul.after_all
    def teardown(ctx):
        ran.append("after_all")

    with manul.Session(binary=FAKE) as s:
        with pytest.raises(manul.EngineError) as exc:
            s.run_suite([PLAIN, SLOW])
        assert "no credentials" in str(exc.value)

    # Teardown still runs: before_all may have got halfway.
    assert ran == ["after_all"]


# Metadata is scratch space between hooks and must not leak into a hunt.
def test_metadata_does_not_reach_the_suite_variables():
    @manul.before_all
    def setup(ctx):
        ctx.metadata["connection"] = object()
        ctx.set("token", "abc")

    with manul.Session(binary=FAKE) as s:
        res = s.run_suite([PLAIN])
        assert res.vars == {"token": "abc"}


# ── group hooks ──────────────────────────────────────────────────────────────

def test_group_hooks_fire_only_for_matching_tags():
    fired = []

    @manul.before_group("smoke")
    def smoke(ctx):
        fired.append("smoke")

    @manul.before_group("slow")
    def slow(ctx):
        fired.append("slow")

    with manul.Session(binary=FAKE) as s:
        s.run_suite([SMOKE, PLAIN])

    assert fired == ["smoke"], "an untagged hunt fires nothing"


def test_group_hooks_fire_once_per_matching_hunt():
    fired = []

    @manul.before_group("smoke")
    def smoke(ctx):
        fired.append("x")

    with manul.Session(binary=FAKE) as s:
        s.run_suite([SMOKE, SMOKE, PLAIN])

    assert len(fired) == 2


def test_after_group_runs_after_each_matching_hunt():
    order = []

    @manul.before_group("smoke")
    def before(ctx):
        order.append("before")

    @manul.after_group("smoke")
    def after(ctx):
        order.append("after")

    with manul.Session(binary=FAKE) as s:
        s.run_suite([SMOKE])

    assert order == ["before", "after"]


# A failed group precondition skips its hunts and only its hunts.
def test_before_group_failure_skips_only_that_hunt():
    @manul.before_group("smoke")
    def gate(ctx):
        raise RuntimeError("fixture missing")

    with manul.Session(binary=FAKE) as s:
        res = s.run_suite([SMOKE, PLAIN])

    assert res.skipped == 1
    assert res.passed == 1, "the untagged hunt still runs"
    assert res.ok is False

    skipped = [h for h in res.hunts if h.skipped]
    assert len(skipped) == 1
    assert "fixture missing" in skipped[0].error


# Teardown failures are reported but change no result.
def test_after_group_failure_does_not_fail_the_hunt():
    @manul.after_group("smoke")
    def cleanup(ctx):
        raise RuntimeError("cleanup broke")

    with manul.Session(binary=FAKE) as s:
        res = s.run_suite([SMOKE])

    assert res.ok and res.passed == 1


def test_group_tag_matching_ignores_case():
    fired = []

    @manul.before_group("  SMOKE ")
    def smoke(ctx):
        fired.append("x")

    with manul.Session(binary=FAKE) as s:
        s.run_suite([SMOKE])

    assert fired == ["x"]


# ── suite results ────────────────────────────────────────────────────────────

def test_suite_result_reports_each_hunt():
    with manul.Session(binary=FAKE) as s:
        res = s.run_suite([PLAIN, FAILING])

    assert res.total == 2
    assert res.passed == 1 and res.failed == 1
    assert res.ok is False
    assert bool(res) is False
    assert [h.path for h in res] == [PLAIN, FAILING]
    assert res.hunts[0].steps == 2


def test_suite_with_no_hooks_still_runs():
    with manul.Session(binary=FAKE) as s:
        res = s.run_suite([PLAIN])
    assert res.ok and res.passed == 1
