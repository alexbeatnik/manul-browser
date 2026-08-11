"""The child half of `manul run --hooks`.

Everything else in this package spawns the engine and drives it. This module is
the one place where that is inverted: the engine spawns *this* process, and this
process only answers.

Nothing here decides anything. Which script to run, which interpreter to run it
with, when hooks fire and in what order are all the engine's business — this is
a read-a-line, call-a-function, write-a-line loop over the same NDJSON reverse
calls a `Session` already handles, and it reuses that same dispatcher.

A hook script is therefore just::

    import manul

    @manul.before_all
    def sign_in(ctx):
        ctx.vars["token"] = get_token()

    manul.serve_hooks()

and is started with ``manul run hunts/ --hooks manul_hooks.py``.

**stdout belongs to the protocol.** A `print()` in a hook script would land in
the middle of a JSON line and break the engine's parser, so this module points
`sys.stdout` at stderr for the duration and hands the real stream to the writer.
That is the same rule the engine imposes on itself in serve mode, applied in the
one direction a user is likely to trip over.
"""

from __future__ import annotations

import json
import sys
from typing import Any, TextIO

from . import controls

__all__ = ["serve_hooks"]


class _HookHost:
    """One conversation with the engine that started this process."""

    def __init__(self, stdin: TextIO, stdout: TextIO) -> None:
        self._in = stdin
        self._out = stdout
        self._id = 0

    # ── wire ────────────────────────────────────────────────────────────────

    def _send(self, payload: dict[str, Any]) -> None:
        self._out.write(json.dumps(payload, separators=(",", ":")) + "\n")
        self._out.flush()

    def _readline(self) -> dict[str, Any] | None:
        while True:
            line = self._in.readline()
            if line == "":
                return None  # engine closed our input: time to exit
            line = line.strip().lstrip("﻿")
            if not line:
                continue
            return json.loads(line)

    def _call(self, cmd: str, args: dict[str, Any] | None = None) -> Any:
        """Send a command to the engine and wait for its reply."""
        self._id += 1
        req_id = self._id
        payload: dict[str, Any] = {"id": req_id, "cmd": cmd}
        if args is not None:
            payload["args"] = args
        self._send(payload)

        msg = self._readline()
        if msg is None:
            raise ConnectionError(f"engine closed the stream while {cmd!r} was pending")
        if not msg.get("ok"):
            err = msg.get("error") or {}
            raise RuntimeError(f"{cmd}: {err.get('message', 'rejected by the engine')}")
        return msg.get("result")

    # ── page primitives ─────────────────────────────────────────────────────
    #
    # Named to match Session, because the handler contexts reach for these on
    # whatever object owns the pipe and must not care which one it is.

    def _page_eval(self, js: str) -> Any:
        return self._call("page.eval", {"js": js})

    def _page_url(self) -> str:
        return self._call("page.url") or ""

    # ── loop ────────────────────────────────────────────────────────────────

    def run(self) -> int:
        payload = controls._registration_payload()
        if any(payload.values()):
            self._call("register", payload)
        self._call("ready")

        while True:
            msg = self._readline()
            if msg is None:
                return 0
            if "invoke" not in msg:
                # The engine only ever writes invocations here; anything else
                # means the two sides disagree about the protocol, and guessing
                # would corrupt the stream rather than fail it.
                raise RuntimeError(f"expected an invocation, got {msg!r}")

            reply: dict[str, Any] = {"invoke": msg["invoke"]}
            try:
                reply["result"] = controls.dispatch_invoke(msg, self)
                reply["ok"] = True
            except Exception as exc:  # a failing handler is an answer, not a crash
                reply["ok"] = False
                reply["error"] = {"code": "handler_failed", "message": f"{type(exc).__name__}: {exc}"}
            self._send(reply)


def serve_hooks() -> int:
    """Answer the engine's callbacks until it closes this process's input.

    Call it at the end of a hook script. It blocks, and returns only when the
    run is over.
    """
    real_stdout = sys.stdout
    sys.stdout = sys.stderr  # see the module docstring
    try:
        return _HookHost(sys.stdin, real_stdout).run()
    finally:
        sys.stdout = real_stdout
