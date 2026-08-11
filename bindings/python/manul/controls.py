"""Custom controls and CALL handlers — Python code the engine calls back into.

Some things a page does cannot be expressed as "click the thing called X": a
canvas signature pad, a third-party date picker, a widget that only responds to
synthetic events. A custom control claims one (page, target) pair and takes over
whenever a step aims at it, so the `.hunt` file keeps saying what it means and
the awkwardness stays in one Python function.

    import manul

    @manul.custom_control(page="Login Page", target="Username")
    def fill_username(ctx):
        ctx.eval(f"document.querySelector('#user').value = {ctx.value!r}")

    with manul.Session() as s:
        s.step("NAVIGATE to https://app.example.com/login")
        s.step("FILL 'Username' field with 'ada'")   # handler runs instead

`CALL HOST` (also spelled `CALL PYTHON`) reaches ordinary functions:

    @manul.call("compute_total")
    def compute_total(ctx):
        return str(sum(float(a) for a in ctx.args))

Registration is process-global and happens at import time, exactly as it did in
the standalone Python engine. A `Session` publishes whatever is registered when
it opens, so decorators do not need a session to exist yet.
"""

from __future__ import annotations

import inspect
import threading
from dataclasses import dataclass, field
from typing import Any, Callable

__all__ = [
    "ControlContext",
    "CallContext",
    "GlobalContext",
    "custom_control",
    "call",
    "before_all",
    "after_all",
    "before_group",
    "after_group",
    "get_custom_control",
    "get_call",
    "list_custom_controls",
    "list_calls",
    "list_hooks",
    "diagnose_custom_control_miss",
    "reset_registry",
    "ANY_PAGE",
]

# The page label that matches every page.
ANY_PAGE = "*"


def _normalise(value: str) -> str:
    """Registry keys are case- and whitespace-insensitive.

    Matches the engine's own normalisation, so a control registered here is
    found by the same key the Go side computes.
    """
    return " ".join(str(value).lower().split())


@dataclass(frozen=True)
class ControlContext:
    """The single argument every custom control handler receives."""

    target: str
    action: str
    value: str
    page: str
    step: str
    url: str = ""
    vars: dict[str, str] = field(default_factory=dict)
    # _session is the live session, used for the page primitives below. Private
    # because a handler must not drive the whole engine from here: the engine is
    # paused inside the step that triggered it.
    _session: Any = None

    def eval(self, js: str) -> Any:
        """Evaluate JavaScript in the page and return the decoded result.

        This is the handler's hands. It works while the engine is paused mid
        step, which the ordinary session methods deliberately do not.
        """
        if self._session is None:
            raise RuntimeError("this context is not attached to a session")
        return self._session._page_eval(js)

    def current_url(self) -> str:
        if self._session is None:
            raise RuntimeError("this context is not attached to a session")
        return self._session._page_url()


@dataclass(frozen=True)
class CallContext:
    """The single argument every CALL handler receives."""

    name: str
    args: list[str] = field(default_factory=list)
    vars: dict[str, str] = field(default_factory=dict)
    _session: Any = None

    def eval(self, js: str) -> Any:
        if self._session is None:
            raise RuntimeError("this context is not attached to a session")
        return self._session._page_eval(js)


@dataclass
class GlobalContext:
    """State shared across a whole suite, handed to every suite-level hook.

    Anything put in `variables` is published to every hunt that follows as a
    `{placeholder}` at global scope, so a hunt's own values still win.
    `metadata` is scratch space between hooks and never reaches a hunt.
    """

    variables: dict[str, str] = field(default_factory=dict)
    metadata: dict[str, Any] = field(default_factory=dict)
    _session: Any = None

    def set(self, name: str, value: str) -> None:
        """Publish a variable to every hunt in the suite."""
        self.variables[str(name)] = str(value)

    def eval(self, js: str) -> Any:
        """Evaluate JavaScript in the page, when there is one.

        `before_all` runs before any browser exists, so this raises there —
        which is the honest answer, not a silent None.
        """
        if self._session is None:
            raise RuntimeError("this context is not attached to a session")
        return self._session._page_eval(js)


_lock = threading.Lock()
# (page_key, target_key) -> handler
_controls: dict[tuple[str, str], Callable[[ControlContext], Any]] = {}
# (page_key, target_key) -> the labels as originally written, for diagnostics
_control_meta: dict[tuple[str, str], dict[str, str]] = {}
_calls: dict[str, Callable[[CallContext], Any]] = {}
# kind -> list of handlers; group kinds key on (kind, tag)
_hooks: dict[str, list[Callable[[GlobalContext], Any]]] = {}
_group_hooks: dict[tuple[str, str], list[Callable[[GlobalContext], Any]]] = {}

BEFORE_ALL = "before_all"
AFTER_ALL = "after_all"
BEFORE_GROUP = "before_group"
AFTER_GROUP = "after_group"


def _check_signature(func: Callable, *, what: str, label: str) -> None:
    """Reject handlers that cannot be called with a single context argument.

    The standalone engine went through a signature change here, and the failure
    mode of a stale handler — a TypeError deep inside a step — was miserable to
    diagnose. Catching it at registration costs nothing and names the problem.
    """
    if not callable(func):
        raise TypeError(f"{what} {label!r} must be callable, got {type(func).__name__}")

    try:
        sig = inspect.signature(func)
    except (TypeError, ValueError):  # builtins and C functions
        return

    required = [
        p
        for p in sig.parameters.values()
        if p.default is inspect.Parameter.empty
        and p.kind in (p.POSITIONAL_ONLY, p.POSITIONAL_OR_KEYWORD)
    ]
    takes_varargs = any(p.kind is p.VAR_POSITIONAL for p in sig.parameters.values())

    if len(required) > 1:
        names = ", ".join(p.name for p in required)
        raise TypeError(
            f"{what} {label!r} must take a single context argument, "
            f"but requires {len(required)} ({names}). "
            f"Handlers receive one context object."
        )
    if not required and not takes_varargs:
        raise TypeError(
            f"{what} {label!r} must accept a context argument, but takes none."
        )


def custom_control(page: str = ANY_PAGE, target: str = "") -> Callable:
    """Register a handler for one (page, target) pair.

    `page` defaults to every page, which is what you want for a widget that
    appears throughout an app. Matching is case-insensitive on both.
    """
    if not str(target).strip():
        raise ValueError("custom_control requires a target")

    def decorate(func: Callable[[ControlContext], Any]) -> Callable[[ControlContext], Any]:
        label = f"{page} / {target}"
        _check_signature(func, what="custom control", label=label)
        key = (_normalise(page), _normalise(target))
        with _lock:
            _controls[key] = func
            _control_meta[key] = {"page": str(page), "target": str(target)}
        return func

    return decorate


def call(name: str) -> Callable:
    """Register a handler for `CALL HOST <name>` (a.k.a. `CALL PYTHON`)."""
    if not str(name).strip():
        raise ValueError("call requires a name")

    def decorate(func: Callable[[CallContext], Any]) -> Callable[[CallContext], Any]:
        _check_signature(func, what="call handler", label=str(name))
        with _lock:
            _calls[_normalise(name)] = func
        return func

    return decorate


def before_all(func: Callable[[GlobalContext], Any]) -> Callable[[GlobalContext], Any]:
    """Run once before the suite, ahead of any hunt and any browser.

    A failure here aborts the suite: its job is to establish preconditions, and
    running the rest without them only wastes time. Whatever it puts in
    `ctx.variables` is visible to every hunt that follows.
    """
    _check_signature(func, what="before_all hook", label=getattr(func, "__name__", "hook"))
    with _lock:
        _hooks.setdefault(BEFORE_ALL, []).append(func)
    return func


def after_all(func: Callable[[GlobalContext], Any]) -> Callable[[GlobalContext], Any]:
    """Run once after the suite, whatever happened.

    Every after_all hook runs even if an earlier one failed — cleanup that
    stops halfway leaves more behind than it saves. Failures are reported and
    change no result.
    """
    _check_signature(func, what="after_all hook", label=getattr(func, "__name__", "hook"))
    with _lock:
        _hooks.setdefault(AFTER_ALL, []).append(func)
    return func


def before_group(tag: str) -> Callable:
    """Run before each hunt whose `@tags:` includes `tag`.

    A failure skips that hunt, and only that hunt.
    """
    if not str(tag).strip():
        raise ValueError("before_group requires a tag")

    def decorate(func: Callable[[GlobalContext], Any]) -> Callable[[GlobalContext], Any]:
        _check_signature(func, what="before_group hook", label=str(tag))
        with _lock:
            _group_hooks.setdefault((BEFORE_GROUP, _normalise(tag)), []).append(func)
        return func

    return decorate


def after_group(tag: str) -> Callable:
    """Run after each hunt whose `@tags:` includes `tag`. Failures are reported."""
    if not str(tag).strip():
        raise ValueError("after_group requires a tag")

    def decorate(func: Callable[[GlobalContext], Any]) -> Callable[[GlobalContext], Any]:
        _check_signature(func, what="after_group hook", label=str(tag))
        with _lock:
            _group_hooks.setdefault((AFTER_GROUP, _normalise(tag)), []).append(func)
        return func

    return decorate


def get_hooks(kind: str, tag: str = "") -> list[Callable[[GlobalContext], Any]]:
    """Every handler registered for one hook kind, in registration order."""
    with _lock:
        if kind in (BEFORE_GROUP, AFTER_GROUP):
            return list(_group_hooks.get((kind, _normalise(tag)), []))
        return list(_hooks.get(kind, []))


def list_hooks() -> list[dict[str, str]]:
    """The hook slots the engine must know about — one per (kind, tag).

    Deliberately not one per handler: the engine registers a single bridge per
    slot, and this side runs every handler in that slot when it fires. Declaring
    a slot twice would make the engine call back twice and run each handler
    twice.
    """
    with _lock:
        out = [{"kind": kind, "tag": ""} for kind, fns in _hooks.items() if fns]
        out += [
            {"kind": kind, "tag": tag}
            for (kind, tag), fns in _group_hooks.items()
            if fns
        ]
    return sorted(out, key=lambda h: (h["kind"], h["tag"]))


def get_custom_control(page: str, target: str) -> Callable[[ControlContext], Any] | None:
    """Look a control up, falling back to a handler registered for any page."""
    tkey = _normalise(target)
    with _lock:
        handler = _controls.get((_normalise(page), tkey))
        if handler is None:
            handler = _controls.get((_normalise(ANY_PAGE), tkey))
    return handler


def get_call(name: str) -> Callable[[CallContext], Any] | None:
    with _lock:
        return _calls.get(_normalise(name))


def list_custom_controls() -> list[dict[str, str]]:
    """Every registered control, as written, sorted for stable output."""
    with _lock:
        meta = list(_control_meta.values())
    return sorted(meta, key=lambda m: (m["page"].lower(), m["target"].lower()))


def list_calls() -> list[str]:
    with _lock:
        return sorted(_calls)


def diagnose_custom_control_miss(page: str, target: str) -> str | None:
    """Explain a near-miss, or return None when there is nothing useful to say.

    The overwhelmingly common mistake is registering the right target under the
    wrong page label, which otherwise looks exactly like not registering it at
    all.
    """
    tkey = _normalise(target)
    with _lock:
        siblings = [
            meta["page"]
            for (pkey, t), meta in _control_meta.items()
            if t == tkey and pkey != _normalise(page)
        ]
    if not siblings:
        return None
    return (
        f"No custom control for {target!r} on page {page!r}, but one is "
        f"registered for {', '.join(repr(p) for p in sorted(siblings))}. "
        f"Check the page label, or register with page='*' to match any page."
    )


def reset_registry() -> None:
    """Drop every registration. For tests."""
    with _lock:
        _controls.clear()
        _control_meta.clear()
        _calls.clear()
        _hooks.clear()
        _group_hooks.clear()


def _registration_payload() -> dict[str, Any]:
    """What a session publishes to the engine when it opens."""
    with _lock:
        controls = [
            {"page": meta["page"], "target": meta["target"]}
            for meta in _control_meta.values()
        ]
        calls = sorted(_calls)
    return {"controls": controls, "calls": calls, "hooks": list_hooks()}
