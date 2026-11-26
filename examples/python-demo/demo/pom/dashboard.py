from __future__ import annotations

from .base import BasePage


class DashboardPage(BasePage):
    path = "/dashboard.html"
    title = "Dashboard"

    @property
    def order_total(self) -> float:
        return float(self.value_of("Order total"))

    @property
    def role(self) -> str:
        return self.value_of("Role")

    def edit_user(self, name: str) -> "DashboardPage":
        return self.do(f"CLICK the 'Edit' button INSIDE 'Users' row with '{name}'").should_see(f"Editing {name}")  # type: ignore[return-value]

    def delete_user(self, name: str) -> "DashboardPage":
        # confirm() is accepted by the engine, so this really deletes.
        return self.do(f"CLICK the 'Delete' button NEAR '{name}'").should_see(f"Deleted {name}")  # type: ignore[return-value]

    def load_whole_feed(self) -> int:
        """WHILE as a one-liner: a hunt snippet run through Session.run(source=…)."""
        self.s.run(
            "STEP 1: feed\n"
            "    WHILE button 'Load More' exists:\n"
            "        CLICK the 'Load More' button\n"
        )
        return self.s.read_text("#feed").count("Feed item")

    def logout(self) -> "LoginPage":
        from .login import LoginPage

        self.do("CLICK the 'Logout' button ON HEADER")
        return LoginPage(self.s, self.base_url).assert_loaded()  # type: ignore[return-value]
