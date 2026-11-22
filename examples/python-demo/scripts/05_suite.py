"""Session.run_suite: before_all / after_all / before_group / after_group."""

from __future__ import annotations

import sys

from _common import ROOT, app_and_session, banner, check

import demo.handlers  # noqa: F401
from demo import hooks

HUNTS = [p for p in sorted((ROOT / "hunts").glob("*.hunt")) if p.name != "08_scheduled.hunt"]

with app_and_session() as (app, s):
    res = s.run_suite(HUNTS)

banner("SuiteResult")
for h in res:
    state = "SKIP" if h.skipped else ("PASS" if h else "FAIL")
    print(f"  {state:4} {h.path.split('/')[-1]:24} tags={h.tags} {h.passed}/{h.steps} {h.error}")
print(f"  total={res.total} passed={res.passed} failed={res.failed} skipped={res.skipped}")
print(f"  globals at the end: {res.vars}")
print(f"  hook events: {hooks.EVENTS}")

skipped = [h.path for h in res if h.skipped]
check(res.skipped == 1 and skipped[0].endswith("09_quarantine.hunt"), "before_group('quarantine') skipped its hunt")
check(res.failed == 0 and res.passed == res.total - 1, "every other hunt passed")
check(res.vars.get("api_token") == "tok-local-42", "before_all published globals")
check(hooks.EVENTS[0].startswith("before_all") and hooks.EVENTS[-1].startswith("after_all"), "after_all ran last")
check(hooks.EVENTS.count("before_group(auth)") == 1 == hooks.EVENTS.count("after_group(auth)"), "group hooks bracket tagged hunts")
# `ok` is False because a skipped hunt is not a pass — that is by design.
sys.exit(0 if res.failed == 0 else 1)
