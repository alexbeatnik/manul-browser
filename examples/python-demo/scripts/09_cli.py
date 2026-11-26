"""The `manul` command line, with the same Python handlers via --hooks.

    python scripts/09_cli.py              # everything except the daemon
    python scripts/09_cli.py --daemon     # also run `manul daemon` for ~70s
"""

from __future__ import annotations

import json
import os
import subprocess
import sys

from _common import ROOT, AppServer, banner, check

MANUL = [sys.executable, "-m", "manul"]  # the console script, honouring $MANUL_BINARY
HOOKS = ["--hooks", "manul_hooks.py"]
# The engine runs a .py hook script with $MANUL_PYTHON, else python/python3 on
# PATH — which may not be the interpreter that has `manul` installed.
os.environ.setdefault("MANUL_PYTHON", sys.executable)


def manul(*args: str, stdin: str | None = None, env: dict | None = None, timeout: int = 600):
    print(f"  $ manul {' '.join(args)}", flush=True)
    r = subprocess.run(MANUL + list(args), cwd=ROOT, input=stdin, capture_output=True, text=True,
                       env={**os.environ, **(env or {})}, timeout=timeout)
    with open(ROOT / "reports" / "cli.log", "a", encoding="utf-8") as log:
        log.write(f"\n$ manul {' '.join(args)}\n{r.stdout}\n{r.stderr}")
    return r


(ROOT / "reports").mkdir(exist_ok=True)
with AppServer() as app:
    os.environ["DEMO_BASE_URL"] = app.base_url  # read by the before_all hook in the hook process

    banner("Whole directory, Python hooks spawned by the engine, HTML report")
    # --tags picks every hunt but the quarantined one: on the CLI a hunt that
    # a before_group hook refuses is BROKEN, and BROKEN exits 1.
    r = manul("run", "hunts/", *HOOKS, "--tags", "smoke,regression,monitoring",
              "--headless", "--html-report", "--screenshot", "on-fail")
    print("\n".join("    " + ln for ln in r.stdout.splitlines()[-3:]))
    check(r.returncode == 0, "every selected hunt passed")
    # One report per hunt (and per @data row): reports/report_<title>_<time>.html.
    reports = sorted((ROOT / "reports").glob("report_*.html"))
    check(reports, f"{len(reports)} HTML report(s), e.g. {reports[-1].name if reports else '-'}")
    check((ROOT / "reports" / "run_history.json").exists(), "reports/run_history.json appended")

    banner("--tags smoke --jsonl: one JSON object per step, then a summary")
    r = manul("run", "hunts/", *HOOKS, "--tags", "smoke", "--jsonl")
    lines = [json.loads(ln) for ln in r.stdout.splitlines() if ln.startswith("{")]
    kinds = {ln.get("event") or ln.get("type") for ln in lines}  # the engine emits "event"
    check(r.returncode == 0 and "step" in kinds, f"{len(lines)} JSON lines, types {sorted(k for k in kinds if k)}")

    banner("--workers 2 --retries 1 --json: parallel browsers, one summary")
    r = manul("run", "hunts/", *HOOKS, "--tags", "forms,widgets,data", "--workers", "2", "--retries", "1", "--json")
    check(r.returncode == 0, "parallel run passed")
    # --json prints one result document per hunt (and per @data row).
    docs, text, dec = [], r.stdout.strip(), json.JSONDecoder()
    while text:
        doc, end = dec.raw_decode(text)
        docs.append(doc)
        text = text[end:].strip()
    print("    " + ", ".join(f"{d['title']}={d['passed']}/{d['total_steps']}" for d in docs))

    banner("--explain --disable-cache: per-channel score breakdown")
    r = manul("run", "hunts/02_forms.hunt", *HOOKS, "--explain", "--disable-cache", "--screenshot", "none")
    breakdowns = r.stdout.count("Breakdown:") + r.stderr.count("Breakdown:")
    check(r.returncode == 0 and breakdowns > 0, f"{breakdowns} breakdown lines")

    banner("Auto-annotate: page names come from pages/localhost_8765.json")
    r = manul("run", "hunts/06_data_driven.hunt", *HOOKS, env={"MANUL_AUTO_ANNOTATE": "true"})
    navs = sorted({ln.split("Auto-Nav:")[1].strip() for ln in (r.stdout + r.stderr).splitlines() if "Auto-Nav:" in ln})
    check(r.returncode == 0, f"Auto-Nav page names: {navs}")

    banner("A hunt from stdin")
    r = manul("run", "-", "--headless", stdin=f"STEP 1: stdin\n    NAVIGATE to {app.base_url}/index.html\n"
                                              "    VERIFY that 'Sign in' is present\nDONE.\n")
    check(r.returncode == 0, "manul run - (stdin)")

    banner("A hunt refused by before_group is BROKEN; a failing hunt is FAIL — both exit 1")
    r = manul("run", "hunts/09_quarantine.hunt", *HOOKS)
    check(r.returncode == 1 and "quarantined" in r.stdout + r.stderr, "before_group refused it")
    r = manul("run", "hunts/09_quarantine.hunt")
    check(r.returncode == 1, "without the hooks it runs — no {base_url} global, no such text — and fails")

    banner("scan / schema / pages / controls")
    r = manul("scan", f"{app.base_url}/forms.html", "--output", "reports/scanned_forms.hunt", "--headless")
    check(r.returncode == 0 and (ROOT / "reports" / "scanned_forms.hunt").exists(), "scan wrote a draft hunt")
    r = manul("schema")
    check("verbs" in json.loads(r.stdout), "schema is JSON with verbs")
    # The subcommand wins over the pages/ directory beside it.
    r = manul("pages", "list")
    check(r.returncode == 0 and "pages" in r.stdout, r.stdout.strip().splitlines()[0])
    r = manul("controls", "list")
    print(f"    {r.stdout.strip()}  (lists Go-registered controls; Python ones arrive at runtime via --hooks)")

    if "--daemon" in sys.argv:
        banner("daemon: runs hunts with @schedule: (08_scheduled.hunt, every 1 minute)")
        print("  $ manul daemon hunts/ --headless   (waits for the first scheduled run, ~60s)", flush=True)
        proc = subprocess.Popen(MANUL + ["daemon", "hunts/", "--headless"], cwd=ROOT, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        seen = []
        try:
            for line in proc.stdout:  # type: ignore[union-attr]
                seen.append(line.rstrip())
                if "finished" in line:
                    break
        finally:
            proc.terminate()
            proc.wait(15)
        check(any("BLOCK PASS" in ln for ln in seen), "the scheduled hunt ran and passed")

    print("\n  not automated here (interactive): `manul record <url>`, "
          "`manul run manual/debug.hunt --debug`, `--break-lines 9`")
