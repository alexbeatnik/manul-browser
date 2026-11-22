"""Shared setup for the demo scripts: import path, engine binary, app server."""

from __future__ import annotations

import contextlib
import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))

# A source checkout builds the engine into .bin/ (see README). An installed
# wheel carries its own and needs none of this.
_local_engine = ROOT / ".bin" / ("manul.exe" if os.name == "nt" else "manul")
if "MANUL_BINARY" not in os.environ and _local_engine.exists():
    os.environ["MANUL_BINARY"] = str(_local_engine)

import manul  # noqa: E402

from demo.app_server import AppServer  # noqa: E402


def banner(title: str) -> None:
    print(f"\n\033[1;36m══ {title} \033[0m", flush=True)


def check(cond: object, what: str) -> None:
    """A demo assertion that says what it proved."""
    if not cond:
        raise SystemExit(f"\033[31m✗ {what}\033[0m")
    print(f"  \033[32m✓\033[0m {what}", flush=True)


@contextlib.contextmanager
def app_and_session(**session_kwargs):
    """Start the site and a session whose working directory is the demo root.

    The engine reads manul.config.json, pages/ and relative MOCK/UPLOAD paths
    from its working directory, so ``cwd`` matters. Engine logs go to a file:
    stdout of the engine is the protocol, its stderr is for humans.
    """
    log_dir = ROOT / "reports"
    log_dir.mkdir(exist_ok=True)
    with open(log_dir / "engine.log", "a", encoding="utf-8") as log, AppServer() as app:
        with manul.Session(cwd=ROOT, stderr=log, **session_kwargs) as s:
            yield app, s
