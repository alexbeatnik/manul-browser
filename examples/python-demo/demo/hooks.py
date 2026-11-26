"""Suite hooks — around a whole run, or around every hunt with a given tag.

They fire under ``Session.run_suite([...])`` and under
``manul run hunts/ --hooks manul_hooks.py``. A single ``Session.run`` is one
hunt, not a suite, so it fires none of them.

| hook          | on failure                                  |
|---------------|---------------------------------------------|
| before_all    | suite aborts, after_all still runs          |
| before_group  | that hunt is skipped, the rest runs         |
| after_*       | reported, changes no result                 |
"""

from __future__ import annotations

import os
import sys
import time

import manul

EVENTS: list[str] = []


def _log(msg: str) -> None:
    EVENTS.append(msg)
    # stderr: under --hooks, stdout is the protocol (serve_hooks redirects it
    # anyway, but being explicit costs nothing).
    print(f"[hooks] {msg}", file=sys.stderr)


@manul.before_all
def global_config(ctx: manul.GlobalContext) -> None:
    """Global variables: visible as {placeholders} in every hunt of the suite."""
    ctx.set("base_url", os.environ.get("DEMO_BASE_URL", "http://localhost:8765"))
    ctx.set("env_name", os.environ.get("DEMO_ENV", "local"))
    ctx.set("signer", "Manul QA")
    ctx.metadata["started"] = time.time()  # scratch space, never reaches a hunt
    _log("before_all: globals published")


@manul.before_all
def api_token(ctx: manul.GlobalContext) -> None:
    """A second before_all in the same slot sees what the first one set."""
    assert ctx.variables["base_url"], "global_config must run first"
    ctx.set("api_token", f"tok-{ctx.variables['env_name']}-42")
    _log("before_all: token issued")


@manul.after_all
def report(ctx: manul.GlobalContext) -> None:
    _log(f"after_all: suite done, token was {ctx.variables.get('api_token')}")


@manul.before_group("auth")
def before_auth(ctx: manul.GlobalContext) -> None:
    ctx.set("login_user", "ada")
    ctx.set("login_password", "lovelace")
    _log("before_group(auth)")


@manul.after_group("auth")
def after_auth(ctx: manul.GlobalContext) -> None:
    _log("after_group(auth)")


@manul.before_group("smoke")
def before_smoke(ctx: manul.GlobalContext) -> None:
    ctx.set("run_kind", "smoke")
    _log("before_group(smoke)")


@manul.before_group("quarantine")
def quarantine(ctx: manul.GlobalContext) -> None:
    """Refuse hunts tagged `quarantine` — they are counted as skipped."""
    _log("before_group(quarantine): refusing")
    raise RuntimeError("quarantined: known-flaky, skipped on purpose")
