"""A user journey written against Python page objects (demo/pom)."""

from __future__ import annotations

from _common import ROOT, app_and_session, banner, check

import demo.handlers  # noqa: F401 — registers the custom controls the pages rely on
from demo import controls
from demo.pom import CatalogPage, FormsPage, LoginPage, StepFailed, WidgetsPage

with app_and_session() as (app, s):
    banner("Login → Dashboard")
    login = LoginPage(s, app.base_url).open()
    login.login_expecting_error("ada", "nope")
    dash = login.login_as("ada", "lovelace", remember=True)
    check(dash.role == "admin" and dash.order_total == 142.50, f"role={dash.role}, total={dash.order_total}")
    dash.edit_user("Grace Hopper").delete_user("Alan Turing")
    check(dash.load_whole_feed() == 4, "WHILE loop loaded the whole feed")

    banner("Forms (custom control: Delivery Date)")
    forms = FormsPage(s, app.base_url).open()
    forms.fill_profile(name="Ada Lovelace", email="ada@example.com", phone="+380440000000", bio="Notes on the Engine")
    forms.choose(country="Ukraine", plan="Pro", newsletter=False).set_delivery("2026-12-24")
    forms.upload_avatar(ROOT / "fixtures" / "avatar.png")
    summary = forms.submit()
    check(summary == {"full_name": "Ada Lovelace", "email": "ada@example.com", "phone": "+380440000000",
                      "bio": "Notes on the Engine", "country": "ua", "plan": "pro", "delivery": "2026-12-24"},
          f"submitted {summary}")

    banner("Widgets (custom controls: Signature Pad, Star Rating)")
    w = WidgetsPage(s, app.base_url).open().wait_ready()
    w.move_card_to_done().sign("Ada").rate(5).type_in_shadow("hello shadow")

    banner("Catalog: NEAR, readonly totals, backend, MOCK")
    c = CatalogPage(s, app.base_url).open().wait_loaded(3)
    c.add_to_cart("Manul Plush", qty=2).add_to_cart("Steppe Map")
    check(c.cart_total == 61.0, f"cart total {c.cart_total}")
    check("placed" in c.checkout(), "checkout placed an order")
    # MOCK lasts for the rest of the session, so it goes last.
    c.use_mock("mocks/products.json").wait_loaded(2).should_see("Mocked Yurt")

    banner("A page object turns a miss into a test failure")
    try:
        c.do("CLICK the 'Pay with gold' button")
    except StepFailed as e:
        check(e.outcome.reason, f"StepFailed: {e}")

    print("\n  custom controls that ran:", controls.CALLS)
    check(any(x.startswith("delivery_date") for x in controls.CALLS), "engine called back into Python")
