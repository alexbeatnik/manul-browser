from __future__ import annotations

from .base import BasePage


class CatalogPage(BasePage):
    path = "/catalog.html"
    title = "Catalog"

    def wait_loaded(self, count: int) -> "CatalogPage":
        return self.should_see(f"Loaded {count} products")  # type: ignore[return-value]

    def add_to_cart(self, product: str, qty: int = 1) -> "CatalogPage":
        for _ in range(qty):
            self.do(f"CLICK the 'Add to cart' button NEAR '{product}'")
        return self

    @property
    def cart_total(self) -> float:
        return float(self.value_of("Cart total"))

    def checkout(self) -> str:
        self.do("CLICK the 'Checkout' button", "VERIFY that 'placed' is present")
        return self.s.read_text("#order-status")

    def use_mock(self, mock_file: str) -> "CatalogPage":
        """MOCK holds for the rest of the session. The path is relative to the
        engine's working directory, because a run-step has no .hunt file."""
        return self.do(
            f"MOCK GET \"/api/products\" with '{mock_file}'",
            "CLICK the 'Refresh catalog' button",
        )  # type: ignore[return-value]
