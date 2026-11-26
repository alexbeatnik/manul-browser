"""Python page objects over a ``manul.Session``.

Manul needs no selectors, so a page object here is not a bag of locators. It
is a *vocabulary*: methods that name what a user does on that page, each one a
few plain-English DSL lines. The engine still resolves every element.
"""

from .base import BasePage, StepFailed
from .catalog import CatalogPage
from .dashboard import DashboardPage
from .forms import FormsPage
from .login import LoginPage
from .widgets import WidgetsPage

__all__ = [
    "BasePage", "StepFailed",
    "LoginPage", "DashboardPage", "FormsPage", "WidgetsPage", "CatalogPage",
]
