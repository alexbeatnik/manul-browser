"""Custom controls and CALL HOST from the session side, plus the registry API."""

from __future__ import annotations

import manul
from _common import app_and_session, banner, check

import demo.handlers  # noqa: F401
from demo import controls

banner("Registry introspection — before any session exists")
for c in manul.list_custom_controls():
    print(f"  control  {c['page']:>14} / {c['target']}")
print(f"  calls    {manul.list_calls()}")
print(f"  hooks    {manul.list_hooks()}")
check(manul.get_custom_control("forms page", "  delivery   DATE ") is not None, "lookup ignores case and whitespace")
check(manul.get_custom_control("Any Page At All", "Star Rating") is not None, "page='*' matches every page")
check(manul.get_custom_control("Dashboard", "Signature Pad") is None, "page-specific control does not leak")
hint = manul.diagnose_custom_control_miss("Dashboard", "Signature Pad")
check(hint and "Widgets Page" in hint, f"miss diagnosis: {hint}")
check(manul.get_call("DEMO.Upper") is not None, "CALL names are case-insensitive too")

with app_and_session() as (app, s):
    banner("Published on open")
    check(s.published["controls"] == 4 and s.published["calls"] >= 8, f"published {s.published}")

    banner("CALL HOST from a single step")
    out = s.step('CALL HOST demo.compute_total with args: "1.25" "2.50" into {sum}')
    check(out and s.vars("sum")["sum"] == "3.75", "scalar result → into {sum}")
    s.step('CALL PYTHON demo.make_user "qa"')  # CALL PYTHON = CALL HOST = CALL GO
    v = s.vars("new_user", "new_email")
    check(v["new_user"].startswith("qa_") and v["new_email"].endswith("@example.com"), f"dict result → {v}")
    s.step(f"NAVIGATE to {app.base_url}/widgets.html")
    s.step("CALL HOST demo.page_title into {title}")
    check(s.vars("title")["title"] == "Widgets Page", "a CALL handler can ctx.eval() in the live page")

    banner("A raising handler fails the step, not the session")
    bad = s.step('CALL HOST demo.assert_equal "a" "b"')
    check(not bad and "AssertionError" in bad.error, f"error: {bad.error}")
    bad = s.step("SELECT '9' from the 'Star Rating' dropdown")
    check(not bad and "rating must be 0..5" in bad.error, f"control error: {bad.error}")
    check(s.step("SELECT '3' from the 'Star Rating' dropdown"), "session still usable afterwards")

    banner("Registering more handlers on a live session")

    @manul.call("demo.late")
    def late(ctx: manul.CallContext) -> str:
        return "registered after open"

    print(f"  publish_handlers() → {s.publish_handlers()}")
    s.step("CALL HOST demo.late into {late}")
    check(s.vars("late")["late"] == "registered after open", "late handler reachable")

    banner("The cookie banner wildcard control on any page")
    check(s.step("CLICK the 'Cookie Banner'"), "wildcard control handled a target that is not on the page")

print("\n  controls that ran:", controls.CALLS)
