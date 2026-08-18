"""Custom controls and CALL handlers.

Covers custom-control registration and dispatch, and
test_31_call_python_args.py. Those tested a registry that lived in the same
process as the engine; the engine is now a separate process, so the same
guarantees are re-tested across the wire — registration, lookup, dispatch,
argument passing, and result capture.
"""

from __future__ import annotations

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import manul  # noqa: E402
from manul import controls  # noqa: E402

FAKE = [sys.executable, str(Path(__file__).resolve().parent / "fake_engine.py")]


@pytest.fixture(autouse=True)
def clean_registry():
    controls.reset_registry()
    yield
    controls.reset_registry()


# ── registry ─────────────────────────────────────────────────────────────────

def test_decorator_registers_and_lookup_is_case_insensitive():
    @manul.custom_control(page="Login Page", target="Username")
    def handler(ctx):
        return None

    assert controls.get_custom_control("Login Page", "Username") is handler
    assert controls.get_custom_control("login page", "username") is handler
    assert controls.get_custom_control("LOGIN PAGE", "USERNAME") is handler


def test_whitespace_is_normalised_in_keys():
    @manul.custom_control(page="  Login   Page ", target=" Username ")
    def handler(ctx):
        return None

    assert controls.get_custom_control("Login Page", "Username") is handler


def test_wildcard_page_matches_anything():
    @manul.custom_control(target="Signature Pad")
    def handler(ctx):
        return None

    assert controls.get_custom_control("Checkout", "Signature Pad") is handler
    assert controls.get_custom_control("Anything At All", "Signature Pad") is handler


def test_page_specific_wins_over_wildcard():
    @manul.custom_control(target="Widget")
    def anywhere(ctx):
        return None

    @manul.custom_control(page="Checkout", target="Widget")
    def on_checkout(ctx):
        return None

    assert controls.get_custom_control("Checkout", "Widget") is on_checkout
    assert controls.get_custom_control("Login", "Widget") is anywhere


def test_unregistered_lookup_returns_none():
    assert controls.get_custom_control("Login", "Nothing") is None


def test_list_custom_controls_reports_labels_as_written():
    @manul.custom_control(page="Login Page", target="Username")
    def a(ctx):
        return None

    @manul.custom_control(page="Checkout", target="Card Number")
    def b(ctx):
        return None

    assert manul.list_custom_controls() == [
        {"page": "Checkout", "target": "Card Number"},
        {"page": "Login Page", "target": "Username"},
    ]


def test_target_is_required():
    with pytest.raises(ValueError):
        manul.custom_control(page="Login Page", target="  ")


# ── signature enforcement ────────────────────────────────────────────────────
#
# The standalone engine changed handler signatures once, and a stale handler
# failed deep inside a step. Registration is where that must be caught.

def test_multi_argument_handler_is_rejected():
    with pytest.raises(TypeError, match="single context argument"):
        @manul.custom_control(page="Login Page", target="Username")
        def legacy(page, target, value):  # the old three-arg shape
            return None


def test_zero_argument_handler_is_rejected():
    with pytest.raises(TypeError, match="accept a context argument"):
        @manul.custom_control(page="Login Page", target="Username")
        def takes_nothing():
            return None


def test_defaulted_extra_arguments_are_allowed():
    @manul.custom_control(page="Login Page", target="Username")
    def handler(ctx, retries=3):
        return None

    assert controls.get_custom_control("Login Page", "Username") is handler


def test_varargs_handler_is_allowed():
    @manul.custom_control(page="Login Page", target="Username")
    def handler(*args):
        return None

    assert controls.get_custom_control("Login Page", "Username") is handler


def test_non_callable_is_rejected():
    with pytest.raises(TypeError, match="callable"):
        manul.custom_control(page="P", target="T")("not a function")


# ── miss diagnostics ─────────────────────────────────────────────────────────

def test_sibling_page_miss_names_the_page_that_has_it():
    @manul.custom_control(page="Login Page", target="Username")
    def handler(ctx):
        return None

    hint = manul.diagnose_custom_control_miss("Signup Page", "Username")
    assert hint is not None
    assert "Login Page" in hint


def test_no_diagnosis_when_the_target_is_unknown_everywhere():
    assert manul.diagnose_custom_control_miss("Login Page", "Nothing") is None


# ── dispatch across the wire ─────────────────────────────────────────────────

def test_custom_control_is_invoked_instead_of_dom_resolution():
    seen = {}

    @manul.custom_control(page="Login Page", target="Username")
    def handler(ctx):
        seen["target"] = ctx.target
        seen["action"] = ctx.action
        seen["value"] = ctx.value
        seen["page"] = ctx.page
        seen["vars"] = ctx.vars

    with manul.Session(binary=FAKE) as s:
        out = s.step("FILL 'Username' field with 'ada'")
        assert out.ok
        # The fake engine only fires a control for a target it was told about,
        # so reaching the handler proves registration crossed the wire.
        assert seen["target"] == "Username"
        assert seen["action"] == "input"
        assert seen["value"] == "ada"
        assert seen["page"] == "Login Page"


def test_handler_failure_fails_the_step_without_killing_the_session():
    @manul.custom_control(page="Login Page", target="Username")
    def handler(ctx):
        raise RuntimeError("widget exploded")

    with manul.Session(binary=FAKE) as s:
        out = s.step("FILL 'Username' field with 'ada'")
        assert out.ok is False
        assert "widget exploded" in out.error

        # The session is still usable afterwards.
        assert s.step("CLICK the 'Sign in' button").ok


# A control registered for another page must not claim the step: it falls
# through to ordinary DOM resolution, exactly as if nothing were registered.
def test_control_on_another_page_does_not_claim_the_step():
    called = []

    @manul.custom_control(page="Somewhere Else", target="Username")
    def handler(ctx):
        called.append(ctx)

    with manul.Session(binary=FAKE) as s:
        out = s.step("FILL 'Username' field with 'ada'")
        assert out.ok
        assert out.action == "click"      # the fake engine's DOM path
        assert called == []


# If the engine ever asks for a handler this process does not have — a stale
# registration, a page label that moved — the answer must name the page that
# does have it rather than a bare "not found".
def test_unknown_callback_is_answered_with_the_sibling_hint():
    @manul.custom_control(page="Login Page", target="Username")
    def handler(ctx):
        return None

    with manul.Session(binary=FAKE) as s:
        with pytest.raises(LookupError) as exc:
            s._dispatch_invoke({
                "kind": "custom_control",
                "page": "Signup Page",
                "target": "Username",
                "action": "input",
            })
        assert "Login Page" in str(exc.value)


def test_unknown_callback_kind_is_rejected():
    with manul.Session(binary=FAKE) as s:
        with pytest.raises(LookupError, match="unknown callback kind"):
            s._dispatch_invoke({"kind": "telepathy"})


# ── CALL handlers ────────────────────────────────────────────────────────────

def test_call_receives_positional_args():
    seen = {}

    @manul.call("compute_total")
    def compute_total(ctx):
        seen["name"] = ctx.name
        seen["args"] = ctx.args
        return str(sum(float(a) for a in ctx.args))

    with manul.Session(binary=FAKE) as s:
        out = s.step('CALL PYTHON compute_total with args: "12" "30" into {total}')
        assert out.ok
        assert seen["name"] == "compute_total"
        assert seen["args"] == ["12", "30"]
        assert s.vars()["total"] == "42.0"


def test_call_result_is_captured_into_the_variable():
    @manul.call("greeting")
    def greeting(ctx):
        return "hello"

    with manul.Session(binary=FAKE) as s:
        s.step("CALL HOST greeting into {msg}")
        assert s.vars()["msg"] == "hello"


def test_call_returning_a_mapping_becomes_variables():
    @manul.call("credentials")
    def credentials(ctx):
        return {"user": "ada", "token": "abc123"}

    with manul.Session(binary=FAKE) as s:
        s.step("CALL HOST credentials")
        got = s.vars()
        assert got["user"] == "ada"
        assert got["token"] == "abc123"


def test_call_name_lookup_is_case_insensitive():
    @manul.call("Compute_Total")
    def compute(ctx):
        return "ok"

    assert controls.get_call("compute_total") is compute
    assert manul.list_calls() == ["compute_total"]


def test_unregistered_call_fails_the_step():
    with manul.Session(binary=FAKE) as s:
        out = s.step("CALL HOST nobody_home")
        assert out.ok is False


def test_call_handler_signature_is_enforced():
    with pytest.raises(TypeError):
        @manul.call("bad")
        def bad(a, b):
            return None


# ── page primitives inside a handler ─────────────────────────────────────────

# A handler must be able to look at the page, the way an embedded Go handler
# does with its browser.Page. The engine is paused mid-step, so this travels a
# nested request rather than an ordinary one.
def test_handler_can_evaluate_javascript():
    seen = {}

    @manul.custom_control(page="Login Page", target="Username")
    def handler(ctx):
        seen["title"] = ctx.eval("document.title")
        seen["url"] = ctx.current_url()

    with manul.Session(binary=FAKE) as s:
        assert s.step("FILL 'Username' field with 'ada'").ok
        assert seen["title"] == "Example"
        assert seen["url"] == "https://example.com"


def test_ordinary_commands_are_refused_inside_a_handler():
    failure = {}

    @manul.custom_control(page="Login Page", target="Username")
    def handler(ctx):
        try:
            # Re-entering the runtime mid-step must be refused, not deadlock.
            ctx._session._t.nested_call("map")
        except manul.EngineError as exc:
            failure["code"] = exc.code
            raise

    with manul.Session(binary=FAKE) as s:
        out = s.step("FILL 'Username' field with 'ada'")
        assert out.ok is False
        assert failure["code"] == "bad_request"


# ── registration payload ─────────────────────────────────────────────────────

def test_handlers_registered_before_any_session_are_published_on_open():
    @manul.custom_control(page="Login Page", target="Username")
    def handler(ctx):
        return None

    @manul.call("compute_total")
    def compute(ctx):
        return "1"

    with manul.Session(binary=FAKE) as s:
        assert s.published == {"controls": 1, "calls": 1, "hooks": 0}


def test_nothing_is_published_when_nothing_is_registered():
    with manul.Session(binary=FAKE) as s:
        assert s.published == {"controls": 0, "calls": 0, "hooks": 0}
