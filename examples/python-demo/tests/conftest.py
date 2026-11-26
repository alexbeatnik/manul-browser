"""pytest wiring: one app server and one browser for the whole run.

A ``manul.Session`` is expensive to open and cheap to reuse, so it is
session-scoped; each test starts from a known page instead of a new browser.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))
_engine = ROOT / ".bin" / ("manul.exe" if os.name == "nt" else "manul")
if "MANUL_BINARY" not in os.environ and _engine.exists():
    os.environ["MANUL_BINARY"] = str(_engine)

import manul  # noqa: E402

import demo.handlers  # noqa: E402,F401 — custom controls and CALL handlers
from demo.app_server import AppServer  # noqa: E402


def pytest_addoption(parser: pytest.Parser) -> None:
    parser.addoption("--browser-engine", default=os.environ.get("MANUL_BROWSER", "chromium"),
                     help="chromium (CDP) or firefox (WebDriver BiDi)")


@pytest.fixture(scope="session")
def app():
    with AppServer() as srv:
        yield srv


@pytest.fixture(scope="session")
def session(app, request):
    (ROOT / "reports").mkdir(exist_ok=True)
    with open(ROOT / "reports" / "pytest-engine.log", "w", encoding="utf-8") as log:
        with manul.Session(cwd=ROOT, stderr=log, browser=request.config.getoption("--browser-engine")) as s:
            s.set_vars(base_url=app.base_url, signer="pytest", env_name="test", api_token="tok-test")
            yield s


@pytest.fixture
def base_url(app) -> str:
    return app.base_url
