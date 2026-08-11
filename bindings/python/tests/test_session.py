"""Binding tests against the fake engine — no Chrome, no network."""

from __future__ import annotations

import os
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import manul  # noqa: E402
from manul.errors import EngineError, EngineNotFound, ProtocolError, SessionClosed  # noqa: E402

FAKE = [sys.executable, str(Path(__file__).resolve().parent / "fake_engine.py")]


def session(**kw):
    return manul.Session(binary=FAKE, **kw)


# ── handshake ────────────────────────────────────────────────────────────────

def test_ready_handshake_populates_versions():
    with session() as s:
        assert s.protocol == "1.0"
        assert s.engine_version == "0.0.0-fake"
        assert not s.closed


def test_engine_that_never_reports_ready_is_a_startup_error():
    env = {**os.environ, "FAKE_NO_READY": "1"}
    with pytest.raises(ProtocolError, match="ready"):
        manul.Session(binary=FAKE, env=env)


def test_unreadable_first_line_is_a_protocol_error():
    env = {**os.environ, "FAKE_JUNK_READY": "1"}
    with pytest.raises(ProtocolError):
        manul.Session(binary=FAKE, env=env)


# A major protocol bump changes existing shapes, so refusing is the only safe
# response — silently carrying on would misread every reply.
def test_future_major_protocol_is_refused():
    env = {**os.environ, "FAKE_PROTOCOL": "2.0"}
    with pytest.raises(ProtocolError, match="protocol"):
        manul.Session(binary=FAKE, env=env)


def test_future_minor_protocol_is_accepted():
    env = {**os.environ, "FAKE_PROTOCOL": "1.7"}
    with manul.Session(binary=FAKE, env=env) as s:
        assert s.protocol == "1.7"


def test_missing_binary_names_what_was_searched():
    with pytest.raises(EngineNotFound) as exc:
        manul.Session(binary=os.path.join("definitely", "not", "here", "manul"))
    message = str(exc.value)
    # The message must be actionable: what was tried, and how to fix it.
    assert "explicit path" in message
    assert "MANUL_BINARY" in message


# ── open ─────────────────────────────────────────────────────────────────────

def test_open_defaults_to_launch():
    with session() as s:
        assert s.mode == "launch"


def test_attach_mode_is_reported_back():
    with session(mode="attach", cdp="http://127.0.0.1:9222") as s:
        assert s.mode == "attach"
        assert s.cdp == "http://127.0.0.1:9222"


def test_deferred_open():
    s = manul.Session(binary=FAKE, open_now=False)
    try:
        assert s.mode == ""
        s.open()
        assert s.mode == "launch"
        # Opening twice is a no-op, not an already_open error.
        s.open()
    finally:
        s.close()


# ── steps ────────────────────────────────────────────────────────────────────

def test_step_success():
    with session() as s:
        out = s.step("CLICK the 'Sign in' button")
        assert out.ok and bool(out) is True
        assert out.action == "click"
        assert out.score == pytest.approx(0.91)


# A step that resolves nothing is an outcome to react to, not an exception.
def test_failed_step_does_not_raise():
    with session() as s:
        out = s.step("CLICK the 'Missing' button")
        assert out.ok is False
        assert bool(out) is False
        assert out.reason == "not_found"
        assert out.near and out.near[0]["label"] == "Other"


def test_run_with_source():
    with session() as s:
        out = s.run("CLICK the 'Sign in' button")
        assert out.ok and out.passed == 3


def test_run_requires_exactly_one_of_source_or_path():
    with session() as s:
        with pytest.raises(ValueError):
            s.run()
        with pytest.raises(ValueError):
            s.run("x", path="y.hunt")


# ── perceiving ───────────────────────────────────────────────────────────────

def test_map_is_parsed_into_dataclasses():
    with session() as s:
        pm = s.map()
        assert pm.url == "https://example.com"
        assert pm.labels() == ["Sign in", "Email"]
        group = pm.groups[0]
        assert group.name == "Page"
        assert group.elements[1].editable is True
        # PageMap iterates its groups directly.
        assert [g.name for g in pm] == ["Page"]


def test_map_budget_is_passed_through():
    with session() as s:
        assert len(s.map(max_per_group=1).groups[0].elements) == 1


def test_read_found_and_missing():
    with session() as s:
        v = s.read("Order total")
        assert v.found and str(v) == "42.00" and bool(v) is True

        missing = s.read("Nothing")
        assert missing.found is False
        assert bool(missing) is False


def test_read_text_uses_the_selector_shape():
    with session() as s:
        assert s.read_text("#main") == "region text"


def test_state():
    with session() as s:
        assert s.state() == {"title": "Example", "url": "https://example.com"}


# ── variables ────────────────────────────────────────────────────────────────

# The whole point of a session: a value set in one call is visible in the next.
def test_variables_survive_across_calls():
    with session() as s:
        s.set_vars(total="42.00", user="ada")
        assert s.vars()["total"] == "42.00"
        assert s.vars("user") == {"user": "ada"}


def test_unset_variables_come_back_empty():
    with session() as s:
        assert s.vars("nope") == {"nope": ""}


# ── errors and lifecycle ─────────────────────────────────────────────────────

def test_engine_rejection_becomes_EngineError():
    s = manul.Session(binary=FAKE, open_now=False)
    try:
        # Any page command before open is rejected by the engine.
        with pytest.raises(EngineError) as exc:
            s.map()
        assert exc.value.code == "not_open"
    finally:
        s.close()


# An EngineError is recoverable: the session must still work afterwards.
def test_session_survives_an_engine_error():
    s = manul.Session(binary=FAKE, open_now=False)
    try:
        with pytest.raises(EngineError):
            s.map()
        s.open()
        assert s.step("CLICK the 'Sign in' button").ok
    finally:
        s.close()


def test_calls_after_close_raise():
    s = session()
    s.close()
    assert s.closed
    with pytest.raises(SessionClosed):
        s.map()


def test_close_is_idempotent():
    s = session()
    s.close()
    s.close()


def test_context_manager_closes_on_exception():
    s = None
    with pytest.raises(RuntimeError):
        with session() as sess:
            s = sess
            raise RuntimeError("boom")
    assert s is not None and s.closed


# A dead engine must surface as a clear protocol error, not a hang.
def test_engine_dying_mid_session_is_reported():
    env = {**os.environ, "FAKE_DIE_AFTER": "1"}
    with manul.Session(binary=FAKE, env=env, open_now=False) as s:
        s.open()
        with pytest.raises(ProtocolError, match="exited"):
            s.map()


def test_schema_passes_through():
    with session() as s:
        assert "CLICK" in s.schema()["verbs"]
