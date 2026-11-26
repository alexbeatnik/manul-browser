"""The same page objects on Firefox, driven over WebDriver BiDi instead of CDP.

Nothing in the hunts or page objects changes. `MANUL_BROWSER=firefox` does the
same from the environment, or `"browser": "firefox"` in manul.config.json.
"""

from __future__ import annotations

from _common import ROOT, app_and_session, banner, check

import demo.handlers  # noqa: F401
from demo.pom import FormsPage, LoginPage, WidgetsPage

with app_and_session(browser="firefox", headless=True) as (app, s):
    banner(f"Firefox session: browser={s.browser}, mode={s.mode}")
    check(s.browser == "firefox", "launched Firefox")
    dash = LoginPage(s, app.base_url).open().login_as("grace", "hopper")
    check(dash.role == "member", "logged in as grace on Firefox")
    FormsPage(s, app.base_url).open().set_delivery("2027-01-01").upload_avatar(ROOT / "fixtures" / "avatar.png")
    WidgetsPage(s, app.base_url).open().wait_ready().sign("Grace").rate(2)
    s.set_vars(base_url=app.base_url, signer="Grace")
    res = s.run(path=ROOT / "hunts" / "03_widgets.hunt")
    check(res, f"03_widgets.hunt on Firefox: {res.passed}/{res.total_steps}")
