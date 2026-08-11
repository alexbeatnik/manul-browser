"""End-to-end check that an installed wheel actually drives Chrome.

Deliberately not part of the pytest suite: everything in `tests/` runs against a
fake engine precisely so it needs no browser, and this needs a real one. The
release workflow runs it on every platform a wheel is built for.

That matters more here than it usually would. The engine had only ever been run
on Windows; the wheels put it on Linux and macOS for the first time, and Chrome
discovery, temp profiles and path handling are exactly where that shows up.

    MANUL_CHROME=/path/to/chrome python tests/smoke_release.py
"""

from __future__ import annotations

import os
import pathlib
import sys
import tempfile

import manul

PAGE = """<!doctype html>
<meta charset="utf-8">
<title>Manul smoke</title>
<h1>Checkout</h1>
<label for="email">Email address</label>
<input id="email">
<button id="signin">Sign in</button>
"""


def main() -> int:
    print(f"package {manul.__version__} from {manul.__file__}")
    print(f"engine  {manul.find_binary()}")

    with tempfile.TemporaryDirectory() as tmp:
        page = pathlib.Path(tmp) / "smoke.html"
        page.write_text(PAGE, encoding="utf-8")

        session = manul.Session(
            headless=True,
            executable_path=os.environ.get("MANUL_CHROME") or None,
        )
        with session as s:
            steps = [
                f"NAVIGATE to {page.as_uri()}",
                "FILL 'Email address' field with 'ada@example.com'",
                "CLICK the 'Sign in' button",
            ]
            for step in steps:
                out = s.step(step)
                print(f"  {'ok ' if out else 'FAIL'} {step}")
                if not out:
                    print(f"       reason: {out.reason}  near: {out.near}")
                    return 1

            # Printed rather than asserted on: which label wins here is a known
            # open question (ROADMAP §verify 6), and seeing the answer per
            # platform is worth more than a green tick.
            labels = s.map().labels()
            print(f"  map labels: {labels}")
            if not labels:
                print("       map returned nothing at all")
                return 1

    print("smoke ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
