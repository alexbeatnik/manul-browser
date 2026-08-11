"""NDJSON-over-stdio transport for `manul serve`.

This module owns the child process and the wire format, and nothing else. It
does not know what a page or a step is — that lives in `session.py`. Keeping the
split means the protocol can gain commands without this file changing.
"""

from __future__ import annotations

import json
import os
import subprocess
import threading
from typing import Any, IO, Sequence

from ._binary import find_binary
from .errors import EngineError, ProtocolError, SessionClosed

__all__ = ["Transport", "SUPPORTED_PROTOCOL_MAJOR"]

# This binding is written against protocol 1.x. Minor bumps only add things, so
# they are safe; a major bump changes existing shapes and must be refused.
SUPPORTED_PROTOCOL_MAJOR = 1


class Transport:
    """A live `manul serve --stdio` process.

    Requests are synchronous: each `call` writes one line and reads until the
    reply with a matching id arrives, passing over any events in between.
    """

    def __init__(
        self,
        binary: str | os.PathLike[str] | Sequence[str] | None = None,
        *,
        args: list[str] | None = None,
        stderr: IO[Any] | None = None,
        cwd: str | os.PathLike[str] | None = None,
        env: dict[str, str] | None = None,
    ) -> None:
        # A sequence is taken verbatim as the command prefix, so the engine can
        # be reached through a wrapper — `wsl`, `docker exec`, a shim script —
        # without this package needing to know about any of them.
        if isinstance(binary, (list, tuple)):
            prefix = [str(part) for part in binary]
            if not prefix:
                raise ValueError("binary sequence is empty")
        else:
            prefix = [find_binary(binary)]
        self._path = prefix[0]

        self._next_id = 0
        self._closed = False
        self._lock = threading.Lock()

        argv = [*prefix, "serve", "--stdio", *(args or [])]
        self._proc = subprocess.Popen(
            argv,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            # Engine logs go to stderr and are never parsed. Left inheriting the
            # parent's stderr by default so warnings stay visible.
            stderr=stderr,
            cwd=cwd,
            env=env,
            text=True,
            encoding="utf-8",
            bufsize=1,
        )

        self.protocol: str = ""
        self.engine_version: str = ""

        # on_invoke handles reverse calls — the engine asking this process to
        # run a custom control or a CALL handler. Set by Session; without it an
        # invoke is answered with an error rather than left to hang.
        self.on_invoke: Any = None

        self._read_ready()

    # ── lifecycle ────────────────────────────────────────────────────────────

    def _read_ready(self) -> None:
        """Consume the ready event the engine always writes first.

        A process that dies before emitting it is reported as a startup failure
        rather than left to hang on the first real request.
        """
        line = self._readline()
        if line is None:
            raise ProtocolError(
                f"engine at {self._path} exited before sending the ready event"
                f" (exit code {self._proc.poll()})"
            )
        try:
            msg = json.loads(line)
        except json.JSONDecodeError as exc:
            raise ProtocolError(f"first line was not JSON: {line!r}") from exc

        if msg.get("event") != "ready":
            raise ProtocolError(f"expected a ready event, got {msg!r}")

        self.protocol = str(msg.get("protocol", ""))
        self.engine_version = str(msg.get("engine", ""))

        major = self.protocol.split(".", 1)[0]
        if major != str(SUPPORTED_PROTOCOL_MAJOR):
            raise ProtocolError(
                f"engine speaks protocol {self.protocol}, this binding supports "
                f"{SUPPORTED_PROTOCOL_MAJOR}.x — upgrade the manul package"
            )

    @property
    def closed(self) -> bool:
        return self._closed

    def close(self, timeout: float = 5.0) -> None:
        """Shut the engine down, politely first.

        `close` gives the engine a chance to release its browser; only a process
        that ignores that is killed.
        """
        if self._closed:
            return
        self._closed = True

        try:
            self._send({"id": self._take_id(), "cmd": "close"})
            self._readline()
        except (OSError, ValueError, ProtocolError):
            pass

        try:
            if self._proc.stdin:
                self._proc.stdin.close()
        except OSError:
            pass

        try:
            self._proc.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            self._proc.kill()
            self._proc.wait()

    def __enter__(self) -> "Transport":
        return self

    def __exit__(self, *_exc: object) -> None:
        self.close()

    # ── wire ─────────────────────────────────────────────────────────────────

    def call(self, cmd: str, args: dict[str, Any] | None = None) -> Any:
        """Send one command and return its result.

        Raises `EngineError` when the engine answers ``ok: false`` — the session
        stays usable, so callers may catch it and carry on.
        """
        if self._closed:
            raise SessionClosed("session is closed")

        with self._lock:
            req_id = self._take_id()
            req: dict[str, Any] = {"id": req_id, "cmd": cmd}
            if args:
                # Drop unset optionals so the engine sees its own defaults
                # rather than a wall of nulls.
                req["args"] = {k: v for k, v in args.items() if v is not None}
            self._send(req)
            return self._await_reply(req_id)

    def _await_reply(self, req_id: int) -> Any:
        while True:
            line = self._readline()
            if line is None:
                raise ProtocolError(
                    f"engine exited while waiting for reply {req_id} "
                    f"(exit code {self._proc.poll()})"
                )
            try:
                msg = json.loads(line)
            except json.JSONDecodeError as exc:
                raise ProtocolError(f"unreadable line: {line!r}") from exc

            # A reverse call: run the handler here and answer it, then carry on
            # waiting. The engine is paused inside our request until we do.
            if "invoke" in msg:
                self._serve_invoke(msg)
                continue

            # Events carry no id and may arrive at any time.
            if "event" in msg and "id" not in msg:
                continue
            if msg.get("id") != req_id:
                continue

            if msg.get("ok"):
                return msg.get("result")
            err = msg.get("error") or {}
            raise EngineError(
                str(err.get("code", "unknown")),
                str(err.get("message", "engine reported a failure")),
            )

    def _serve_invoke(self, msg: dict[str, Any]) -> None:
        """Run one reverse call and write its result back.

        A handler that raises is reported to the engine, which fails the step —
        the session itself stays healthy, because a broken handler is a bug in
        one step, not a reason to tear down the browser.
        """
        invoke_id = msg.get("invoke")
        try:
            if self.on_invoke is None:
                raise LookupError(
                    f"engine asked for {msg.get('kind')!r} but no handler is registered"
                )
            result = self.on_invoke(msg)
        except BaseException as exc:  # noqa: BLE001 - reported, never swallowed
            self._send({
                "invoke": invoke_id,
                "ok": False,
                "error": {
                    "code": "handler_failed",
                    "message": f"{type(exc).__name__}: {exc}",
                },
            })
        else:
            self._send({"invoke": invoke_id, "ok": True, "result": result})

    def nested_call(self, cmd: str, args: dict[str, Any] | None = None) -> Any:
        """Issue a request from inside a reverse call.

        Only the engine's `page.*` primitives are available here — everything
        else would re-enter the step that is currently running. It bypasses the
        lock deliberately: the caller is already inside a `call` that holds it.
        """
        req_id = self._take_id()
        req: dict[str, Any] = {"id": req_id, "cmd": cmd}
        if args:
            req["args"] = {k: v for k, v in args.items() if v is not None}
        self._send(req)
        return self._await_reply(req_id)

    def _take_id(self) -> int:
        self._next_id += 1
        return self._next_id

    def _send(self, payload: dict[str, Any]) -> None:
        stdin = self._proc.stdin
        if stdin is None:
            raise ProtocolError("engine stdin is not available")
        stdin.write(json.dumps(payload, ensure_ascii=False) + "\n")
        stdin.flush()

    def _readline(self) -> str | None:
        stdout = self._proc.stdout
        if stdout is None:
            return None
        line = stdout.readline()
        if line == "":
            return None
        return line.strip()
