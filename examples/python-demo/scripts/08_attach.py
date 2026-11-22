"""Attach to a Chrome that is already running, then drive it — from Python and
from the one-shot agent CLI (`manul map`, `manul read`, `manul run-step`).

In attach mode the browser is NOT closed when the session ends: Manul did not
open it. The script starts and stops this Chrome itself.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.request

import manul
from _common import app_and_session, banner, check

CANDIDATES = [
    os.environ.get("CHROME_PATH", ""),
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    shutil.which("google-chrome") or "", shutil.which("chromium") or "", shutil.which("chrome") or "",
    r"C:\Program Files\Google\Chrome\Application\chrome.exe",
]
chrome = next((c for c in CANDIDATES if c and os.path.exists(c)), None)
if not chrome:
    sys.exit("no Chrome found; set CHROME_PATH")

PORT = 9333
profile = tempfile.mkdtemp(prefix="manul-attach-")
proc = subprocess.Popen(
    [chrome, f"--remote-debugging-port={PORT}", f"--user-data-dir={profile}", "--headless=new",
     "--no-first-run", "--no-default-browser-check", "about:blank"],
    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
)
endpoint = f"http://127.0.0.1:{PORT}"
try:
    for _ in range(50):
        try:
            urllib.request.urlopen(f"{endpoint}/json/version", timeout=1)
            break
        except OSError:
            time.sleep(0.2)

    with app_and_session(mode="attach", cdp=endpoint) as (app, s):
        banner(f"Attached: mode={s.mode}, cdp={s.cdp}")
        check(s.mode == "attach", "attach mode")
        s.step(f"NAVIGATE to {app.base_url}/dashboard.html")
        check(s.state()["title"] == "Dashboard", "drove the existing tab")

        banner("One-shot agent CLI against the same browser")
        cli = [sys.executable, "-m", "manul"]
        out = subprocess.run(cli + ["map", "--cdp", endpoint, "--tab", "dashboard", "--max-per-group", "3"],
                             capture_output=True, text=True, check=True).stdout
        check('"groups"' in out, f"manul map → {out[:90]}…")
        out = subprocess.run(cli + ["read", "Order total", "--cdp", endpoint], capture_output=True, text=True).stdout
        check('"142.50"' in out, f"manul read → {out.strip()}")
        out = subprocess.run(cli + ["run-step", "CLICK the 'Load More' button", "--cdp", endpoint, "--compact"],
                             capture_output=True, text=True).stdout
        check('"ok":true' in out.replace(" ", ""), f"manul run-step → {out.strip()[:90]}")

    banner("Session closed — the browser it attached to is still up")
    with urllib.request.urlopen(f"{endpoint}/json/version", timeout=2) as r:
        check(r.status == 200, "Chrome survived the session")
    # The same, from the environment instead of arguments:
    os.environ.update(MANUL_BROWSER_MODE="attach", MANUL_CDP_ENDPOINT=endpoint)
    with manul.Session() as s2:
        check(s2.mode == "attach", "MANUL_BROWSER_MODE=attach + MANUL_CDP_ENDPOINT")
finally:
    proc.terminate()
    proc.wait(10)
    shutil.rmtree(profile, ignore_errors=True)
