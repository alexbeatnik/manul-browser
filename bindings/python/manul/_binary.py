"""Locating the `manul` engine binary.

A wheel normally ships the binary inside the package, but a developer working on
the engine wants their own build to win without reinstalling anything. Hence the
order below: an explicit override first, the developer's PATH before the bundled
copy is not offered — an installed wheel should be self-contained and
predictable, so the bundled binary is preferred over whatever PATH happens to
hold, and MANUL_BINARY exists for the developer who wants to override that.
"""

from __future__ import annotations

import os
import shutil
import sys
from pathlib import Path

from .errors import EngineNotFound

__all__ = ["find_binary", "BINARY_NAME"]

BINARY_NAME = "manul.exe" if sys.platform == "win32" else "manul"


def _bundled_path() -> Path:
    return Path(__file__).resolve().parent / "_bin" / BINARY_NAME


def find_binary(explicit: str | os.PathLike[str] | None = None) -> str:
    """Return the path to the engine binary.

    Resolution order:

    1. `explicit`, when the caller passed one.
    2. ``$MANUL_BINARY``.
    3. The copy bundled in this package (what a wheel installs).
    4. ``manul`` on ``PATH``.

    Raises `EngineNotFound` listing everything that was tried.
    """
    searched: list[str] = []

    if explicit:
        p = Path(explicit)
        if p.is_file():
            return str(p)
        searched.append(f"{p} (explicit path)")

    env = os.environ.get("MANUL_BINARY")
    if env:
        p = Path(env)
        if p.is_file():
            return str(p)
        searched.append(f"{p} (MANUL_BINARY)")

    bundled = _bundled_path()
    if bundled.is_file():
        return str(bundled)
    searched.append(f"{bundled} (bundled with this package)")

    on_path = shutil.which("manul")
    if on_path:
        return on_path
    searched.append("manul on PATH")

    raise EngineNotFound(searched)
