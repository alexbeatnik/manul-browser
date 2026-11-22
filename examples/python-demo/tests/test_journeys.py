"""Page-object tests and hunt files, side by side in one pytest run."""

from __future__ import annotations

from pathlib import Path

import pytest

from demo.pom import CatalogPage, DashboardPage, FormsPage, LoginPage, StepFailed, WidgetsPage

HUNTS = Path(__file__).resolve().parent.parent / "hunts"


@pytest.mark.parametrize("user,password,role", [("ada", "lovelace", "admin"), ("alan", "turing", "member")])
def test_login_roles(session, base_url, user, password, role):
    dash = LoginPage(session, base_url).open().login_as(user, password)
    assert dash.role == role
    dash.logout()


def test_bad_login_stays_on_login_page(session, base_url):
    LoginPage(session, base_url).open().login_expecting_error("ada", "nope").assert_loaded()


def test_dashboard_users(session, base_url):
    LoginPage(session, base_url).open().login_as("ada", "lovelace")
    dash = DashboardPage(session, base_url).open()
    assert dash.order_total == pytest.approx(142.5)
    dash.edit_user("Grace Hopper")


def test_form_roundtrip(session, base_url):
    forms = FormsPage(session, base_url).open()
    forms.fill_profile(name="Grace Hopper", email="grace@example.com", phone="1", bio="COBOL")
    forms.choose(country="Poland", plan="Free", newsletter=True).set_delivery("2026-12-01")
    assert forms.submit()["delivery"] == "2026-12-01"


def test_widgets_custom_controls(session, base_url):
    WidgetsPage(session, base_url).open().wait_ready().sign("pytest").rate(3)


def test_catalog_cart(session, base_url):
    cat = CatalogPage(session, base_url).open().wait_loaded(3).add_to_cart("Steppe Map", qty=3)
    assert cat.cart_total == pytest.approx(36.0)


def test_missing_element_raises(session, base_url):
    with pytest.raises(StepFailed) as exc:
        LoginPage(session, base_url).open().do("CLICK the 'Forgot password' link")
    assert exc.value.outcome.reason == "not_found"


# Each .hunt is a test too. 08 is the daemon's, 09 is meant to be skipped by
# a suite hook, 11 installs a session-wide MOCK and so would leak into others.
@pytest.mark.parametrize("hunt", [p for p in sorted(HUNTS.glob("*.hunt")) if p.name[:2] not in ("08", "09", "11")],
                         ids=lambda p: p.stem)
def test_hunt(session, hunt):
    result = session.run(path=hunt)
    assert result.ok, f"{result.failed} of {result.total_steps} steps failed"
