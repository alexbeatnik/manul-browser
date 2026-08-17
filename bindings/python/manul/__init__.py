"""Manul — browser automation in plain English, for humans and LLM agents.

This package is a thin client for the Manul engine. It ships the engine binary,
starts it, and speaks a JSON protocol to it; the scoring, the .hunt DSL and the
CDP transport all live in the engine itself, in exactly one implementation.

    import manul

    with manul.Session() as s:
        s.step("NAVIGATE to https://example.com")
        s.step("CLICK the 'Sign in' button")
        print(s.map().labels())

To drive a Chrome you already have open, launched with
``--remote-debugging-port=9222``::

    with manul.Session(mode="attach") as s:
        print(s.state()["url"])

That browser stays open when the session ends — Manul did not open it.
"""

from __future__ import annotations

from ._binary import find_binary
from ._hookhost import serve_hooks
from ._transport import Transport
from .controls import (
    ANY_PAGE,
    GlobalContext,
    after_all,
    after_group,
    before_all,
    before_group,
    list_hooks,
    CallContext,
    ControlContext,
    call,
    custom_control,
    diagnose_custom_control_miss,
    get_call,
    get_custom_control,
    list_calls,
    list_custom_controls,
    reset_registry,
)
from .errors import (
    EngineError,
    EngineNotFound,
    ManulError,
    ProtocolError,
    SessionClosed,
)
from .session import (
    MapElement,
    SuiteHunt,
    SuiteResult,
    MapGroup,
    PageMap,
    RunOutcome,
    Session,
    StepOutcome,
    Value,
)

__version__ = "0.1.1"

__all__ = [
    "Session",
    "StepOutcome",
    "RunOutcome",
    "PageMap",
    "MapGroup",
    "MapElement",
    "Value",
    "custom_control",
    "call",
    "ControlContext",
    "CallContext",
    "get_custom_control",
    "get_call",
    "list_custom_controls",
    "list_calls",
    "diagnose_custom_control_miss",
    "reset_registry",
    "ANY_PAGE",
    "before_all",
    "after_all",
    "before_group",
    "after_group",
    "GlobalContext",
    "list_hooks",
    "serve_hooks",
    "SuiteResult",
    "SuiteHunt",
    "ManulError",
    "EngineNotFound",
    "ProtocolError",
    "EngineError",
    "SessionClosed",
    "Transport",
    "find_binary",
    "__version__",
]
