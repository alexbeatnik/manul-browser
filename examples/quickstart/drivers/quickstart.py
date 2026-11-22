"""Drive the engine from Python, a step at a time.

    pip install manul-browser
    python drivers/quickstart.py

Run it from examples/quickstart with the site being served (see README.md).
"""

from pathlib import Path

import manul

SITE = "http://localhost:8000"
HERE = Path(__file__).resolve().parent.parent

with manul.Session(headless=True) as s:
    # One DSL line per call; the outcome says what was done and how sure it was.
    s.step(f"NAVIGATE to {SITE}/login.html")
    s.step("FILL 'Username' field with 'ada'")
    s.step("FILL 'Password' field with 'lovelace'")
    out = s.step("CLICK the 'Sign in' button")
    print(f"{out.action}: ok={out.ok} score={out.score:.2f}")

    # read() asks for one labelled thing; not found is an answer, not an error.
    print("role:", s.read("Role").value)

    # map() is what the page offers, grouped — the short way to show it to an LLM.
    print("labels:", s.map().labels()[:10])

    # Variables live as long as the session does.
    s.step(f"NAVIGATE to {SITE}/shop.html")
    s.step("CLICK the 'Add to cart' button NEAR 'Snow Boots'")
    s.step("EXTRACT the 'Cart total' into {total}")
    print("total:", s.vars("total")["total"])

    # A whole .hunt file runs in the same browser. A failing step raises
    # manul.EngineError(code="step_failed").
    result = s.run(path=HERE / "02_login.hunt")
    print(f"02_login.hunt: {result.passed}/{result.total_steps} steps passed")
