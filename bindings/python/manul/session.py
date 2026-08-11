"""The Python face of the Manul engine.

`Session` mirrors `pkg/agent.Session` in Go, method for method, so the two
languages describe the same thing the same way. Everything here is a thin call
over the protocol — no scoring, no DSL parsing, no CDP.
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from typing import Any, IO, Iterator, Sequence

from . import controls
from ._transport import Transport

__all__ = [
    "Session", "StepOutcome", "RunOutcome", "SuiteResult", "SuiteHunt",
    "PageMap", "MapGroup", "MapElement", "Value",
]


@dataclass(frozen=True)
class MapElement:
    label: str
    role: str = ""
    editable: bool = False


@dataclass(frozen=True)
class MapGroup:
    name: str
    elements: list[MapElement] = field(default_factory=list)
    truncated: int = 0


@dataclass(frozen=True)
class PageMap:
    """A landmark-grouped view of the page, budgeted for an LLM's context."""

    url: str = ""
    groups: list[MapGroup] = field(default_factory=list)

    def labels(self) -> list[str]:
        """Every element label on the page, flattened — handy for a quick look."""
        return [el.label for g in self.groups for el in g.elements]

    def __iter__(self) -> Iterator[MapGroup]:
        return iter(self.groups)


@dataclass(frozen=True)
class Value:
    """The result of reading one labelled thing off the page.

    ``found`` false is a normal answer, not an error: the label simply is not
    there right now.
    """

    value: str = ""
    found: bool = False
    reason: str = ""

    def __bool__(self) -> bool:
        return self.found

    def __str__(self) -> str:
        return self.value


@dataclass(frozen=True)
class StepOutcome:
    """What happened when one DSL line ran."""

    ok: bool = False
    step: str = ""
    action: str = ""
    value: str = ""
    url: str = ""
    reason: str = ""
    error: str = ""
    score: float = 0.0
    near: list[dict[str, Any]] = field(default_factory=list)

    def __bool__(self) -> bool:
        return self.ok


@dataclass(frozen=True)
class RunOutcome:
    """The aggregate of running a whole .hunt script."""

    ok: bool = False
    url: str = ""
    total_steps: int = 0
    passed: int = 0
    failed: int = 0

    def __bool__(self) -> bool:
        return self.ok


@dataclass(frozen=True)
class SuiteHunt:
    """One hunt's outcome inside a suite."""

    path: str = ""
    ok: bool = False
    #: True when a before_group hook refused this hunt. The suite carried on.
    skipped: bool = False
    tags: list[str] = field(default_factory=list)
    steps: int = 0
    passed: int = 0
    failed: int = 0
    error: str = ""

    def __bool__(self) -> bool:
        return self.ok


@dataclass(frozen=True)
class SuiteResult:
    """The aggregate of a suite run."""

    ok: bool = False
    total: int = 0
    passed: int = 0
    failed: int = 0
    skipped: int = 0
    hunts: list[SuiteHunt] = field(default_factory=list)
    #: Variables the suite hooks published, as they stood at the end.
    vars: dict[str, str] = field(default_factory=dict)

    def __bool__(self) -> bool:
        return self.ok

    def __iter__(self):
        return iter(self.hunts)


def _map_from(raw: Any) -> PageMap:
    if not isinstance(raw, dict):
        return PageMap()
    groups = []
    for g in raw.get("groups") or []:
        elements = [
            MapElement(
                label=e.get("label", ""),
                role=e.get("role", ""),
                editable=bool(e.get("editable", False)),
            )
            for e in (g.get("elements") or [])
        ]
        groups.append(
            MapGroup(
                name=g.get("name", ""),
                elements=elements,
                truncated=int(g.get("truncated", 0) or 0),
            )
        )
    return PageMap(url=raw.get("url", ""), groups=groups)


class Session:
    """A live browser session driven by the engine.

    Either start a browser::

        with manul.Session() as s:
            s.step("NAVIGATE to https://example.com")

    or drive one that is already running::

        with manul.Session(mode="attach", cdp="http://127.0.0.1:9222") as s:
            print(s.map().labels())

    In ``attach`` mode the browser is left running on close — the session did
    not open it.

    Variables survive between steps for the life of the session, which is the
    whole reason this is a session and not a series of one-shot commands::

        s.step("EXTRACT the 'Order total' into {total}")
        print(s.vars()["total"])
    """

    def __init__(
        self,
        *,
        mode: str | None = None,
        cdp: str | None = None,
        tab: str | None = None,
        headless: bool | None = None,
        port: int | None = None,
        executable_path: str | None = None,
        binary: str | os.PathLike[str] | Sequence[str] | None = None,
        stderr: IO[Any] | None = None,
        cwd: str | os.PathLike[str] | None = None,
        env: dict[str, str] | None = None,
        open_now: bool = True,
    ) -> None:
        self._t = Transport(binary, stderr=stderr, cwd=cwd, env=env)
        self._t.on_invoke = self._dispatch_invoke
        self._opened = False
        self.mode: str = ""
        self.cdp: str = ""
        #: How many handlers were published to the engine on open.
        self.published: dict[str, int] = {"controls": 0, "calls": 0, "hooks": 0}

        self._open_args = {
            "mode": mode,
            "cdp": cdp,
            "tab": tab,
            "headless": headless,
            "port": port,
            "executablePath": executable_path,
        }
        if open_now:
            self.open()

    # ── lifecycle ────────────────────────────────────────────────────────────

    def open(self) -> "Session":
        """Open the browser session. Called for you unless ``open_now=False``.

        A failure here closes the transport rather than leaving an orphaned
        engine process behind.
        """
        if self._opened:
            return self
        try:
            # Handlers are published before the browser exists: they describe
            # this process, and a decorator at import time must not depend on
            # a session having been created yet.
            self.publish_handlers()
            res = self._t.call("open", self._open_args) or {}
        except BaseException:
            self._t.close()
            raise
        self._opened = True
        self.mode = res.get("mode", "")
        self.cdp = res.get("cdp", "")
        return self

    def publish_handlers(self) -> dict[str, int]:
        """Tell the engine which custom controls and CALL handlers exist here.

        Called automatically by `open`. Call it again after registering more
        handlers on a session that is already running.
        """
        payload = controls._registration_payload()
        if not any(payload.values()):
            self.published = {"controls": 0, "calls": 0, "hooks": 0}
        else:
            self.published = dict(self._t.call("register", payload) or {})
        return self.published

    def close(self) -> None:
        """End the session and stop the engine. Safe to call twice."""
        self._t.close()
        self._opened = False

    @property
    def closed(self) -> bool:
        return self._t.closed

    @property
    def engine_version(self) -> str:
        return self._t.engine_version

    @property
    def protocol(self) -> str:
        return self._t.protocol

    def __enter__(self) -> "Session":
        return self

    def __exit__(self, *_exc: object) -> None:
        self.close()

    # ── acting ───────────────────────────────────────────────────────────────

    def step(self, instruction: str) -> StepOutcome:
        """Run one plain-English DSL line.

        A step that resolves nothing returns ``ok=False`` rather than raising:
        not finding an element is an outcome an agent reacts to, not a crash.
        """
        raw = self._t.call("run-step", {"step": instruction}) or {}
        return StepOutcome(
            ok=bool(raw.get("ok", False)),
            step=raw.get("step", instruction),
            action=raw.get("action", ""),
            value=raw.get("value", "") or "",
            url=raw.get("url", "") or "",
            reason=raw.get("reason", ""),
            error=raw.get("error", "") or "",
            score=float(raw.get("score", 0.0) or 0.0),
            near=list(raw.get("near") or []),
        )

    def run(self, source: str | None = None, *, path: str | os.PathLike[str] | None = None) -> RunOutcome:
        """Run a whole .hunt script, given either its text or a path to it."""
        if (source is None) == (path is None):
            raise ValueError("run() takes exactly one of source or path")
        args = {"source": source} if source is not None else {"path": str(path)}
        raw = self._t.call("run", args) or {}
        return RunOutcome(
            ok=bool(raw.get("ok", False)),
            url=raw.get("url", ""),
            total_steps=int(raw.get("total_steps", 0) or 0),
            passed=int(raw.get("passed", 0) or 0),
            failed=int(raw.get("failed", 0) or 0),
        )

    def run_suite(self, paths: "list[str | os.PathLike[str]]") -> "SuiteResult":
        """Run several .hunt files with the suite lifecycle applied.

        This is not a loop over `run`: `@before_all` / `@after_all` bracket the
        whole set, and `@before_group(tag=…)` fires per hunt based on the
        `@tags:` in the file — which only the engine parses. A hunt skipped by a
        group hook does not stop the rest.
        """
        raw = self._t.call("run-suite", {"paths": [str(p) for p in paths]}) or {}
        return SuiteResult(
            ok=bool(raw.get("ok", False)),
            total=int(raw.get("total", 0) or 0),
            passed=int(raw.get("passed", 0) or 0),
            failed=int(raw.get("failed", 0) or 0),
            skipped=int(raw.get("skipped", 0) or 0),
            vars=dict(raw.get("vars") or {}),
            hunts=[
                SuiteHunt(
                    path=h.get("path", ""),
                    ok=bool(h.get("ok", False)),
                    skipped=bool(h.get("skipped", False)),
                    tags=list(h.get("tags") or []),
                    steps=int(h.get("steps", 0) or 0),
                    passed=int(h.get("passed", 0) or 0),
                    failed=int(h.get("failed", 0) or 0),
                    error=h.get("error", "") or "",
                )
                for h in (raw.get("hunts") or [])
            ],
        )

    # ── perceiving ───────────────────────────────────────────────────────────

    def map(self, *, max_per_group: int | None = None, include_unlabeled: bool | None = None) -> PageMap:
        """A compact, grouped map of what is on the page.

        This is the cheap way to show a page to an LLM: several times fewer
        tokens than raw HTML for the same context.
        """
        raw = self._t.call(
            "map",
            {"maxPerGroup": max_per_group, "includeUnlabeled": include_unlabeled},
        )
        return _map_from(raw)

    def read(self, label: str, *, max_chars: int | None = None) -> Value:
        """Read the value of one labelled element, without a full page scan."""
        raw = self._t.call("read", {"label": label, "maxChars": max_chars}) or {}
        return Value(
            value=raw.get("value", "") or "",
            found=bool(raw.get("found", False)),
            reason=raw.get("reason", ""),
        )

    def read_text(self, selector: str, *, max_chars: int | None = None) -> str:
        """Read the visible text of a CSS region."""
        raw = self._t.call("read", {"selector": selector, "maxChars": max_chars}) or {}
        return raw.get("text", "") or ""

    def state(self) -> dict[str, str]:
        """The current page's title and URL."""
        raw = self._t.call("state") or {}
        return {"title": raw.get("title", ""), "url": raw.get("url", "")}

    # ── variables ────────────────────────────────────────────────────────────

    def vars(self, *names: str) -> dict[str, str]:
        """Read session variables, all of them or just the ones named.

        Names that are not set come back as empty strings, so a caller never
        has to tell "missing key" apart from "missing variable".
        """
        args = {"get": list(names)} if names else None
        return dict(self._t.call("vars", args) or {})

    def set_vars(self, **values: str) -> dict[str, str]:
        """Seed variables and return the resulting snapshot."""
        return dict(self._t.call("vars", {"set": dict(values)}) or {})

    # ── engine ───────────────────────────────────────────────────────────────

    def schema(self) -> dict[str, Any]:
        """The engine's own description of its DSL and JSON shapes."""
        return dict(self._t.call("schema") or {})

    # ── reverse calls ────────────────────────────────────────────────────────

    def _dispatch_invoke(self, msg: dict[str, Any]) -> Any:
        """Route one engine callback to the handler that claimed it."""
        kind = msg.get("kind")

        if kind == "custom_control":
            page = msg.get("page", "")
            target = msg.get("target", "")
            handler = controls.get_custom_control(page, target)
            if handler is None:
                hint = controls.diagnose_custom_control_miss(page, target)
                raise LookupError(hint or f"no custom control for {target!r} on page {page!r}")
            return handler(
                controls.ControlContext(
                    target=target,
                    action=msg.get("action", ""),
                    value=msg.get("value", "") or "",
                    page=page,
                    step=msg.get("step", ""),
                    url=msg.get("url", "") or "",
                    vars=dict(msg.get("vars") or {}),
                    _session=self,
                )
            )

        if kind == "hook":
            hook, tag = msg.get("hook", ""), msg.get("tag", "")
            handlers = controls.get_hooks(hook, tag)
            if not handlers:
                raise LookupError(f"no {hook!r} hook registered" + (f" for tag {tag!r}" if tag else ""))

            ctx = controls.GlobalContext(
                variables=dict(msg.get("vars") or {}),
                _session=self,
            )
            # Every handler in a slot runs, and they share one context so a
            # later hook sees what an earlier one published.
            for handler in handlers:
                handler(ctx)
            # Only the variables travel back; metadata is scratch space that
            # never reaches a hunt.
            return ctx.variables

        if kind == "call":
            name = msg.get("name", "")
            handler = controls.get_call(name)
            if handler is None:
                raise LookupError(f"no CALL handler registered for {name!r}")
            return handler(
                controls.CallContext(
                    name=name,
                    args=list(msg.get("args") or []),
                    vars=dict(msg.get("vars") or {}),
                    _session=self,
                )
            )

        raise LookupError(f"unknown callback kind {kind!r}")

    # Page primitives, valid only while a handler is running. They exist so a
    # Python handler can inspect and touch the page the way an embedded Go
    # handler does with its browser.Page.
    def _page_eval(self, js: str) -> Any:
        return self._t.nested_call("page.eval", {"js": js})

    def _page_url(self) -> str:
        return self._t.nested_call("page.url") or ""
