"""The four exceptions, and when each one is raised."""

from __future__ import annotations

import os
import sys

import manul
from _common import ROOT, app_and_session, banner, check

banner("EngineNotFound — the binary could not be located")
# find_binary tries: binary= › $MANUL_BINARY › the copy in the wheel › PATH.
# A missing explicit path falls through to the next candidate, so this only
# raises where nothing else is installed (a source checkout without a wheel).
saved = os.environ.pop("MANUL_BINARY", None)
saved_path, os.environ["PATH"] = os.environ.get("PATH", ""), ""
try:
    found = manul.find_binary("/definitely/not/manul")
    print(f"  fell through to {found} (installed wheel) — EngineNotFound not reachable here")
except manul.EngineNotFound as e:
    check(e.searched, f"searched: {e.searched}")
finally:
    os.environ["PATH"] = saved_path
    if saved:
        os.environ["MANUL_BINARY"] = saved

banner("ProtocolError — something that is not the engine")
try:
    manul.Session(binary=[sys.executable, "-c", "print('hello, I am not manul')"])
except manul.ProtocolError as e:
    check(True, f"ProtocolError: {str(e)[:70]}…")

banner("open_now=False — publish handlers and open later")
s = manul.Session(open_now=False, cwd=ROOT)
try:
    try:
        s.step("NAVIGATE to about:blank")
    except manul.EngineError as e:
        check(e.code == "not_open", f"EngineError code={e.code!r} before open()")
    s.open()
    check(s.step("NAVIGATE to about:blank"), "works after open()")
finally:
    s.close()

banner("EngineError — the engine rejected a request; the session survives")
with app_and_session() as (app, s):
    try:
        s.read_text("div[[[")
    except manul.EngineError as e:
        check(e.code == "bad_request", f"bad selector → {e.code}: {e.message}")
    try:
        s.run(path=ROOT / "hunts" / "missing.hunt")
    except manul.EngineError as e:
        check(e.code == "bad_request", f"missing hunt → {e.code}")
    check(s.step(f"NAVIGATE to {app.base_url}/index.html"), "session still usable")

banner("SessionClosed — after close()")
s.close()  # closing twice is safe
try:
    s.state()
except manul.SessionClosed:
    check(s.closed, "SessionClosed raised, session.closed is True")

check(issubclass(manul.EngineError, manul.ManulError), "every error derives from ManulError")
