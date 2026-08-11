"""Make the wheel platform-specific.

The package is pure Python, but it ships a Go binary, so a `py3-none-any` wheel
would install a Windows engine on Linux. Nothing in hatchling infers that on its
own — there is no extension module to give the game away — so this hook states
the tag outright and marks the wheel impure.

The target comes from ``MANUL_TARGET`` as a Go ``GOOS/GOARCH`` pair, which is
exactly what the release workflow already has in hand. Local builds without it
fall back to the host platform.

Building with no binary present is refused. A wheel that installs cleanly and
then fails at the first `Session()` is worse than a build error, and it is the
easiest mistake to make here.
"""

from __future__ import annotations

import os
import stat
import sysconfig
from pathlib import Path
from typing import Any

from hatchling.builders.hooks.plugin.interface import BuildHookInterface

#: Go target -> (binary file name, wheel platform tag).
#:
#: The Linux tags are compressed sets: the binary is built with CGO_ENABLED=0,
#: so it is statically linked and has no libc to be compatible with. glibc and
#: musl both run it, and the manylinux2014 alias keeps older pip resolvers happy.
TARGETS: dict[str, tuple[str, str]] = {
    "windows/amd64": ("manul.exe", "win_amd64"),
    "windows/arm64": ("manul.exe", "win_arm64"),
    "darwin/arm64": ("manul", "macosx_11_0_arm64"),
    "darwin/amd64": ("manul", "macosx_10_9_x86_64"),
    "linux/amd64": (
        "manul",
        "manylinux_2_17_x86_64.manylinux2014_x86_64.musllinux_1_1_x86_64",
    ),
    "linux/arm64": (
        "manul",
        "manylinux_2_17_aarch64.manylinux2014_aarch64.musllinux_1_1_aarch64",
    ),
}


def _host_target() -> str:
    """Best guess at the Go target for the machine doing the build."""
    plat = sysconfig.get_platform()
    goos = (
        "windows" if plat.startswith("win")
        else "darwin" if plat.startswith("macosx")
        else "linux"
    )
    goarch = (
        "arm64" if ("arm64" in plat or "aarch64" in plat)
        else "amd64"
    )
    return f"{goos}/{goarch}"


class ManulBinaryHook(BuildHookInterface):
    PLUGIN_NAME = "custom"

    def initialize(self, version: str, build_data: dict[str, Any]) -> None:
        bin_dir = Path(self.root) / "manul" / "_bin"
        present = sorted(p for p in bin_dir.glob("*") if p.is_file()) if bin_dir.is_dir() else []

        if not present:
            # Deliberate escape hatch: `pip install -e .` and the test suite
            # never need the binary, and CI builds the sdist this way.
            if os.environ.get("MANUL_ALLOW_PURE_WHEEL"):
                self.app.display_warning(
                    "manul: no engine binary in manul/_bin/ — building a pure "
                    "wheel because MANUL_ALLOW_PURE_WHEEL is set. Do not publish it."
                )
                return
            raise RuntimeError(
                "no engine binary found in manul/_bin/.\n"
                "Build one first, e.g.\n"
                "  cd core && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \\\n"
                "      go build -trimpath -ldflags='-s -w' \\\n"
                "      -o ../bindings/python/manul/_bin/manul ./cmd/manul\n"
                "then set MANUL_TARGET=linux/amd64. Set MANUL_ALLOW_PURE_WHEEL=1 "
                "to build a binary-less wheel on purpose."
            )

        target = os.environ.get("MANUL_TARGET") or _host_target()
        if target not in TARGETS:
            raise RuntimeError(
                f"MANUL_TARGET={target!r} is not a supported target. "
                f"Known: {', '.join(sorted(TARGETS))}"
            )
        binary_name, tag = TARGETS[target]

        # One wheel, one engine. A stale binary left over from the previous
        # target in the matrix would otherwise be shipped inside this one.
        unexpected = [p.name for p in present if p.name != binary_name]
        if unexpected:
            raise RuntimeError(
                f"manul/_bin/ holds {', '.join(unexpected)}, which does not belong "
                f"in a {target} wheel (expected exactly {binary_name}). "
                "Clean the directory between targets."
            )

        binary = bin_dir / binary_name

        # A zip stores the mode bits and pip restores them — but only if they
        # were right on disk. `actions/download-artifact` drops the executable
        # bit, so set it here rather than trusting whatever produced the file.
        #
        # A Windows filesystem has no bit to set, and the resulting wheel would
        # install an engine nobody can run. That is a build failure, not a
        # warning: it is invisible until someone on Linux tries the release.
        if not binary_name.endswith(".exe"):
            mode = binary.stat().st_mode
            binary.chmod(mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
            if not binary.stat().st_mode & 0o111:
                raise RuntimeError(
                    f"cannot set the executable bit on {binary} — a {target} wheel "
                    "built here would install a binary nobody can run.\n"
                    "Build Linux and macOS wheels on a POSIX host (that is what the "
                    "release workflow does)."
                )

        build_data["pure_python"] = False
        build_data["infer_tag"] = False
        build_data["tag"] = f"py3-none-{tag}"

        size_mb = binary.stat().st_size / (1024 * 1024)
        self.app.display_info(
            f"manul: {target} wheel, tag py3-none-{tag}, "
            f"engine {binary_name} ({size_mb:.1f} MiB)"
        )
