#!/usr/bin/env python3
"""A stand-in for `manul serve --stdio`.

It speaks the protocol and nothing else — no browser, no scoring — so the
binding's transport and result mapping can be tested on any machine, in
milliseconds, with no Chrome installed.

Behaviour is steered by env vars so a single script can play every part:

    FAKE_PROTOCOL   protocol version to announce      (default 1.0)
    FAKE_NO_READY   exit before the ready event       (set to 1)
    FAKE_JUNK_READY emit a non-JSON first line        (set to 1)
    FAKE_DIE_AFTER  exit after N requests
"""

from __future__ import annotations

import json
import os
import re
import sys


def out(obj: dict) -> None:
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()


def main() -> int:
    if os.environ.get("FAKE_NO_READY"):
        return 3
    if os.environ.get("FAKE_JUNK_READY"):
        sys.stdout.write("not json at all\n")
        sys.stdout.flush()
        return 0

    out({
        "event": "ready",
        "protocol": os.environ.get("FAKE_PROTOCOL", "1.0"),
        "engine": "0.0.0-fake",
    })

    die_after = int(os.environ.get("FAKE_DIE_AFTER", "0") or 0)
    opened = False
    variables: dict[str, str] = {}
    controls: list[dict] = []
    calls: list[str] = []
    hooks: list[dict] = []
    seen = 0
    state = {"invoke": 0}

    def norm(s: str) -> str:
        return " ".join(str(s).lower().split())

    def invoke(payload: dict):
        """Reverse call: ask the client to run a handler and wait for it.

        While waiting, serve the page.* primitives and refuse anything else —
        the same rule the real engine applies, because the step is paused.
        """
        state["invoke"] += 1
        ident = state["invoke"]
        out({**payload, "invoke": ident})

        for raw in sys.stdin:
            raw = raw.strip()
            if not raw:
                continue
            msg = json.loads(raw)

            if "cmd" in msg:
                nid, ncmd = msg.get("id"), msg["cmd"]
                nargs = msg.get("args") or {}
                if ncmd == "page.eval":
                    js = nargs.get("js", "")
                    value = {"document.title": "Example"}.get(js, None)
                    out({"id": nid, "ok": True, "result": value})
                elif ncmd == "page.url":
                    out({"id": nid, "ok": True, "result": "https://example.com"})
                else:
                    out({"id": nid, "ok": False, "error": {
                        "code": "bad_request",
                        "message": f"{ncmd!r} is not available while a handler is running",
                    }})
                continue

            if msg.get("invoke") != ident:
                raise RuntimeError(f"bad invoke reply {msg!r}")
            if not msg.get("ok"):
                raise RuntimeError((msg.get("error") or {}).get("message", "handler failed"))
            return msg.get("result")

        raise RuntimeError("client closed while a handler was pending")

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        seen += 1
        if die_after and seen > die_after:
            return 4

        try:
            req = json.loads(line)
        except json.JSONDecodeError:
            out({"id": None, "ok": False,
                 "error": {"code": "bad_request", "message": "malformed"}})
            continue

        rid, cmd = req.get("id"), req.get("cmd")
        args = req.get("args") or {}

        def fail(code: str, msg: str) -> None:
            out({"id": rid, "ok": False, "error": {"code": code, "message": msg}})

        if cmd == "close":
            out({"id": rid, "ok": True, "result": {}})
            return 0

        if cmd == "schema":
            out({"id": rid, "ok": True, "result": {"verbs": ["CLICK", "FILL"]}})
            continue

        if cmd == "register":
            controls.extend(args.get("controls") or [])
            calls.extend(args.get("calls") or [])
            hooks.extend(args.get("hooks") or [])
            out({"id": rid, "ok": True, "result": {
                "controls": len(args.get("controls") or []),
                "calls": len(args.get("calls") or []),
                "hooks": len(args.get("hooks") or []),
            }})
            continue

        if cmd == "open":
            if opened:
                fail("already_open", "session already open")
                continue
            opened = True
            mode = args.get("mode") or ("attach" if args.get("cdp") else "launch")
            out({"id": rid, "ok": True, "result": {
                "mode": mode,
                "cdp": args.get("cdp", ""),
                "url": "https://example.com",
            }})
            continue

        if not opened:
            fail("not_open", "no session; call open first")
            continue

        if cmd == "run-step":
            step = args.get("step", "")

            # CALL HOST / CALL PYTHON / CALL GO — one spelling in the engine.
            m = re.match(
                r"""^CALL\s+(?:HOST|PYTHON|GO)\s+(\S+)   # handler name
                    (?:\s+with\s+args:\s*(.*?))?         # optional args
                    (?:\s+(?:into|to)\s+\{(\w+)\})?$     # optional capture
                """,
                step, re.I | re.X)
            if m:
                name, raw_args, into = m.group(1), m.group(2) or "", m.group(3)
                call_args = re.findall(r'"([^"]*)"', raw_args)
                try:
                    result = invoke({"kind": "call", "name": name,
                                     "args": call_args, "vars": dict(variables)})
                except RuntimeError as exc:
                    out({"id": rid, "ok": True, "result": {
                        "ok": False, "step": step, "reason": "action_failed",
                        "error": str(exc)}})
                    continue
                if isinstance(result, dict):
                    variables.update({k: str(v) for k, v in result.items()})
                if into is not None and result is not None:
                    variables[into] = str(result)
                out({"id": rid, "ok": True, "result": {
                    "ok": True, "step": step, "action": "call", "reason": "ok"}})
                continue

            # A registered control claims the step before DOM resolution.
            target = None
            tm = re.search(r"'([^']+)'", step)
            if tm:
                target = tm.group(1)
            claimed = next(
                (c for c in controls
                 if norm(c["target"]) == norm(target or "")
                 and norm(c["page"]) in (norm("Login Page"), "*")),
                None,
            )
            if claimed is not None:
                value_m = re.search(r"with '([^']*)'", step)
                try:
                    invoke({
                        "kind": "custom_control",
                        "page": "Login Page",
                        "target": target,
                        "action": "input" if value_m else "clickable",
                        "value": value_m.group(1) if value_m else "",
                        "step": step,
                        "vars": dict(variables),
                    })
                except RuntimeError as exc:
                    out({"id": rid, "ok": True, "result": {
                        "ok": False, "step": step, "reason": "action_failed",
                        "error": str(exc)}})
                    continue
                out({"id": rid, "ok": True, "result": {
                    "ok": True, "step": step, "action": "custom-control",
                    "reason": "ok"}})
                continue

            # A step naming "Missing" is the failure case, so tests can cover
            # ok=false without it being an exception.
            ok = "Missing" not in step
            out({"id": rid, "ok": True, "result": {
                "ok": ok,
                "step": step,
                "action": "click" if ok else "",
                "reason": "ok" if ok else "not_found",
                "score": 0.91 if ok else 0.0,
                "near": [] if ok else [{"label": "Other", "score": 0.2}],
            }})
            continue

        if cmd == "run":
            out({"id": rid, "ok": True, "result": {
                "ok": True, "url": "https://example.com",
                "total_steps": 3, "passed": 3, "failed": 0,
            }})
            continue

        if cmd == "run-suite":
            # Tags come from the file name here: "<name>@<tag>,<tag>.hunt".
            # The real engine reads @tags: from the file; this stands in for it
            # without needing a parser.
            def tags_of(path: str) -> list[str]:
                stem = os.path.basename(path).rsplit(".", 1)[0]
                return stem.split("@", 1)[1].split(",") if "@" in stem else []

            def fire(kind: str, tag: str = ""):
                if not any(h["kind"] == kind and (h.get("tag") or "") == tag for h in hooks):
                    return None
                return invoke({"kind": "hook", "hook": kind, "tag": tag,
                               "vars": dict(variables)})

            paths = args.get("paths") or []
            result = {"ok": True, "total": len(paths), "passed": 0,
                      "failed": 0, "skipped": 0, "hunts": []}
            try:
                published = fire("before_all")
                if isinstance(published, dict):
                    variables.update({k: str(v) for k, v in published.items()})
            except RuntimeError as exc:
                fire("after_all")
                out({"id": rid, "ok": False, "error": {
                    "code": "step_failed", "message": f"suite aborted: {exc}"}})
                continue

            for path in paths:
                tags = tags_of(path)
                entry = {"path": path, "tags": tags, "ok": False}
                skipped = False
                for tag in tags:
                    try:
                        fire("before_group", tag)
                    except RuntimeError as exc:
                        entry.update({"skipped": True, "error": str(exc)})
                        result["skipped"] += 1
                        result["ok"] = False
                        skipped = True
                        break
                if skipped:
                    result["hunts"].append(entry)
                    continue

                # "fail" in the name marks a hunt that should not pass.
                ok = "fail" not in os.path.basename(path)
                entry.update({"ok": ok, "steps": 2,
                              "passed": 2 if ok else 1, "failed": 0 if ok else 1})
                result["passed" if ok else "failed"] += 1
                if not ok:
                    result["ok"] = False
                for tag in tags:
                    try:
                        fire("after_group", tag)
                    except RuntimeError:
                        pass  # teardown failures never change a result
                result["hunts"].append(entry)

            try:
                fire("after_all")
            except RuntimeError:
                pass
            result["vars"] = dict(variables)
            out({"id": rid, "ok": True, "result": result})
            continue

        if cmd == "map":
            cap = args.get("maxPerGroup") or 8
            out({"id": rid, "ok": True, "result": {
                "url": "https://example.com",
                "groups": [
                    {"name": "Page", "elements": [
                        {"label": "Sign in", "role": "button"},
                        {"label": "Email", "role": "textbox", "editable": True},
                    ][:cap], "truncated": 0},
                ],
            }})
            continue

        if cmd == "read":
            if args.get("selector"):
                out({"id": rid, "ok": True, "result": {
                    "text": "region text", "selector": args["selector"]}})
            else:
                label = args.get("label", "")
                found = label != "Nothing"
                out({"id": rid, "ok": True, "result": {
                    "value": "42.00" if found else "",
                    "found": found,
                    "reason": "ok" if found else "not_found",
                }})
            continue

        if cmd == "vars":
            if args.get("set"):
                variables.update(args["set"])
            if args.get("get"):
                out({"id": rid, "ok": True,
                     "result": {k: variables.get(k, "") for k in args["get"]}})
            else:
                out({"id": rid, "ok": True, "result": dict(variables)})
            continue

        if cmd == "state":
            out({"id": rid, "ok": True, "result": {
                "title": "Example", "url": "https://example.com"}})
            continue

        fail("bad_request", f"unknown cmd {cmd!r}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
