"""The Session API, method by method.

step · run(source) · map · read · read_text · state · vars · set_vars · schema
plus what a session reports about itself, and what a *failed* step looks like.
"""

from __future__ import annotations

from _common import app_and_session, banner, check

with app_and_session() as (app, s):
    banner("Session")
    print(f"  engine {s.engine_version}, protocol {s.protocol}, mode={s.mode}, browser={s.browser}")
    print(f"  handlers published on open: {s.published}")
    check(s.mode == "launch" and s.browser == "chromium", "launched Chromium (manul.config.json: headless)")

    banner("step() → StepOutcome")
    out = s.step(f"NAVIGATE to {app.base_url}/index.html")
    check(out.ok and out.action, f"NAVIGATE ok, action={out.action!r}, url={out.url}")
    out = s.step("FILL 'Username' field with 'ada'")
    check(out and out.score > 0, f"FILL resolved with score {out.score:.2f}")

    banner("A miss is an outcome, not an exception")
    miss = s.step("CLICK the 'Launch rockets' button")
    check(not miss, f"ok=False, reason={miss.reason!r}")
    print(f"  near: {[(c.get('text'), c.get('score')) for c in miss.near]}")

    banner("state() / map() / read() / read_text()")
    check(s.state()["title"] == "Login Page", f"state: {s.state()}")
    page = s.map()
    for group in page:
        print(f"  [{group.name}] " + ", ".join(f"{e.label}({e.role}{', editable' if e.editable else ''})" for e in group.elements))
    check("Sign in" in page.labels(), "map() lists 'Sign in'")
    check(len(s.map(max_per_group=1, include_unlabeled=True).groups) > 0, "map(max_per_group=1, include_unlabeled=True)")
    check(s.read("Username").value == "ada", "read('Username') reads back what FILL wrote")
    check(not s.read("Nonexistent widget"), "read() of a missing label → found=False, no exception")
    text = s.read_text("main", max_chars=7)
    check(text.startswith("Sign in") and "truncated" in text, f"read_text('main', max_chars=7) = {text!r}")

    banner("Variables live for the whole session")
    s.set_vars(base_url=app.base_url, password="lovelace")
    s.step("FILL 'Password' field with '{password}'")
    s.step("CLICK the 'Sign in' button")
    s.step("EXTRACT the 'Order total' into {total}")
    check(s.vars("total", "missing") == {"total": "142.50", "missing": ""}, f"vars('total', 'missing') = {s.vars('total', 'missing')}")
    check("base_url" in s.vars(), "vars() with no names returns everything")

    banner("run(source=…) — a hunt as a string, sharing this session's state")
    res = s.run(
        "STEP 1: inline\n"
        "    VERIFY that 'Welcome, ada' is present\n"
        "    SET {seen} = yes\n"
        "    PRINT \"total was {total}\"\n"
    )
    check(res.ok and res.passed == res.total_steps, f"RunOutcome passed {res.passed}/{res.total_steps}")

    banner("schema() — the engine describes its own DSL")
    schema = s.schema()
    print(f"  keys: {sorted(schema)[:8]}…")
    check(schema, "schema() returned the contract")
