"""Session.run(path=…): one .hunt at a time, sharing one browser.

A single run is not a suite, so no suite hooks fire. The globals they would
publish ({base_url}, {signer}, …) are seeded with set_vars instead.
"""

from __future__ import annotations

import sys

import manul
from _common import ROOT, app_and_session, banner, check

import demo.handlers  # noqa: F401

# 08 is for the daemon, 09 is refused by a suite hook — neither belongs here.
HUNTS = [p for p in sorted((ROOT / "hunts").glob("*.hunt")) if p.name[:2] not in ("08", "09")]

with app_and_session() as (app, s):
    s.set_vars(base_url=app.base_url, env_name="local", signer="Manul QA", api_token="tok-local-42")
    failed = []
    for hunt in HUNTS:
        try:
            res = s.run(path=hunt)
        except manul.EngineError as e:  # a hunt that fails a step answers step_failed
            print(f"  \033[31m✗\033[0m {hunt.name:24} {e.code}: {e.message}")
            failed.append(hunt.name)
            continue
        mark = "\033[32m✓\033[0m" if res else "\033[31m✗\033[0m"
        print(f"  {mark} {hunt.name:24} {res.passed}/{res.total_steps} steps  ({res.url})")
        if not res:
            failed.append(hunt.name)
    banner("Variables a hunt left behind are still in the session")
    print("  ", {k: v for k, v in s.vars().items() if k in ("order_total", "country", "ui_total", "orders")})
    check(not failed, f"all {len(HUNTS)} hunts passed" if not failed else f"failed: {failed}")
sys.exit(1 if failed else 0)
