"""CALL HOST handlers — plain Python functions a .hunt can call.

    CALL HOST demo.compute_total with args: "24.50" "89.99" into {total}
    CALL PYTHON demo.make_user "qa"          # same command, other spelling

A scalar result lands in ``into {var}``; a dict result becomes several
variables at once; ``None`` sets nothing.
"""

from __future__ import annotations

import json
import urllib.request
import uuid

import manul



@manul.call("demo.compute_total")
def compute_total(ctx: manul.CallContext) -> str:
    return f"{sum(float(a) for a in ctx.args):.2f}"


@manul.call("demo.upper")
def upper(ctx: manul.CallContext) -> str:
    return " ".join(ctx.args).upper()


@manul.call("demo.make_user")
def make_user(ctx: manul.CallContext) -> dict[str, str]:
    """Dict result → {new_user}, {new_email}, {new_id} all become variables."""
    prefix = ctx.args[0] if ctx.args else "user"
    uid = uuid.uuid4().hex[:6]
    return {"new_user": f"{prefix}_{uid}", "new_email": f"{prefix}_{uid}@example.com", "new_id": uid}


def _api(ctx: manul.CallContext, method: str, path: str):
    base = ctx.vars.get("base_url") or "http://localhost:8765"
    req = urllib.request.Request(f"{base}{path}", method=method, data=b"{}" if method == "POST" else None)
    with urllib.request.urlopen(req, timeout=5) as r:
        return json.load(r)


@manul.call("demo.seed_db")
def seed_db(ctx: manul.CallContext) -> str:
    """Runs in [SETUP], before any browser exists — talks to the backend only."""
    _api(ctx, "POST", "/api/reset")
    return "seeded"


@manul.call("demo.cleanup_db")
def cleanup_db(ctx: manul.CallContext) -> None:
    _api(ctx, "POST", "/api/reset")
    return None


@manul.call("demo.api_order_count")
def api_order_count(ctx: manul.CallContext) -> str:
    """Talk to the backend directly — verify what the UI did."""
    return str(len(_api(ctx, "GET", "/api/orders")))


@manul.call("demo.page_title")
def page_title(ctx: manul.CallContext) -> str:
    """A CALL handler can reach into the live page too."""
    return str(ctx.eval("document.title"))


@manul.call("demo.assert_equal")
def assert_equal(ctx: manul.CallContext) -> None:
    """A raising handler fails the step — that is how a CALL asserts."""
    left, right = (ctx.args + ["", ""])[:2]
    if left != right:
        raise AssertionError(f"{left!r} != {right!r}")
