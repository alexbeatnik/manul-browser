from __future__ import annotations

from .base import BasePage


class LoginPage(BasePage):
    path = "/index.html"
    title = "Login Page"

    def login_as(self, username: str, password: str, *, remember: bool = False) -> "DashboardPage":
        from .dashboard import DashboardPage

        self.do(
            f"FILL 'Username' field with '{username}'",
            f"FILL 'Password' field with '{password}'",
        )
        if remember:
            self.do("CHECK the checkbox for 'Remember me'")
        self.do("CLICK the 'Sign in' button")
        return DashboardPage(self.s, self.base_url).assert_loaded()  # type: ignore[return-value]

    def login_expecting_error(self, username: str, password: str) -> "LoginPage":
        self.do(
            f"FILL 'Username' field with '{username}'",
            f"FILL 'Password' field with '{password}'",
            "CLICK the 'Sign in' button",
        )
        return self.should_see("Invalid username or password")  # type: ignore[return-value]
