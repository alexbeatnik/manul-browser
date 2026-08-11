"""The `manul` console script.

A wheel that carries the engine but leaves it buried in `site-packages` is only
half an install: `manul run checkout.hunt` is how most people meet this project.
So the entry point is a shim that hands the whole command line to the bundled
binary and gets out of the way — every subcommand, every flag, no wrapper
parsing anything it does not have to.
"""

from __future__ import annotations

import os
import subprocess
import sys
from typing import Sequence

from ._binary import find_binary
from .errors import EngineNotFound

__all__ = ["main"]


def main(argv: Sequence[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)

    try:
        binary = find_binary()
    except EngineNotFound as exc:
        print(f"manul: {exc}", file=sys.stderr)
        return 127

    if os.name == "nt":
        # `execv` on Windows starts a new process and lets this one exit, so the
        # shell returns to the prompt while the engine is still running. Wait for
        # it and pass its exit code on instead.
        try:
            return subprocess.call([binary, *args])
        except KeyboardInterrupt:
            return 130
        except OSError as exc:
            print(f"manul: cannot run {binary}: {exc}", file=sys.stderr)
            return 126

    try:
        os.execv(binary, [binary, *args])
    except OSError as exc:
        # Almost always a lost executable bit — a wheel built on a filesystem
        # that has none, or an installer that dropped the mode.
        print(
            f"manul: cannot execute {binary}: {exc}\n"
            f"If this is a permissions problem: chmod +x {binary}",
            file=sys.stderr,
        )
        return 126

    return 0  # unreachable: execv replaced this process


if __name__ == "__main__":
    sys.exit(main())
