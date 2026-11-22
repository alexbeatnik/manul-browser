"""Run the whole demo: every script, then the pytest suite.

    python run_all.py                 # Chromium only
    python run_all.py --firefox       # + the Firefox (WebDriver BiDi) script
    python run_all.py --attach        # + attach to a running Chrome, agent CLI
    python run_all.py --daemon        # + `manul daemon` (~70s)
    python run_all.py --all
"""

from __future__ import annotations

import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent
args = set(sys.argv[1:])
everything = "--all" in args

steps: list[list[str]] = [
    ["scripts/01_session_basics.py"],
    ["scripts/02_page_objects.py"],
    ["scripts/03_controls_and_calls.py"],
    ["scripts/04_run_hunts.py"],
    ["scripts/05_suite.py"],
    ["scripts/06_errors.py"],
]
if everything or "--firefox" in args:
    steps.append(["scripts/07_firefox.py"])
if everything or "--attach" in args:
    steps.append(["scripts/08_attach.py"])
steps.append(["scripts/09_cli.py"] + (["--daemon"] if everything or "--daemon" in args else []))
steps.append(["-m", "pytest", "tests", "-q"])

results = []
for step in steps:
    name = " ".join(step)
    print(f"\n\033[1;35m▶ {name}\033[0m", flush=True)
    t0 = time.time()
    code = subprocess.call([sys.executable, *step], cwd=ROOT)
    results.append((name, code, time.time() - t0))

print("\n\033[1mSummary\033[0m")
PASS, FAIL = "\033[32mPASS\033[0m", "\033[31mFAIL\033[0m"
for name, code, secs in results:
    print(f"  {PASS if code == 0 else FAIL}  {secs:5.1f}s  {name}")
sys.exit(max(code for _, code, _ in results))
