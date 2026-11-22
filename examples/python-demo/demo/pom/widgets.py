from __future__ import annotations

from .base import BasePage


class WidgetsPage(BasePage):
    path = "/widgets.html"
    title = "Widgets Page"

    def wait_ready(self) -> "WidgetsPage":
        return self.do("WAIT FOR 'Loading widgets...' to disappear")  # type: ignore[return-value]

    def move_card_to_done(self) -> "WidgetsPage":
        return self.do("DRAG 'Card A' and drop it into 'Done column'").should_see("Card A moved to Done")  # type: ignore[return-value]

    def sign(self, signer: str) -> "WidgetsPage":
        # The custom control reads {signer} from session variables.
        self.s.set_vars(signer=signer)
        return self.do("CLICK the 'Signature Pad'").should_see(f"Signed by {signer}")  # type: ignore[return-value]

    def rate(self, stars: int) -> "WidgetsPage":
        return self.do(f"SELECT '{stars}' from the 'Star Rating' dropdown").should_see(f"Rating: {stars}")  # type: ignore[return-value]

    def type_in_shadow(self, text: str) -> "WidgetsPage":
        return self.do(f"TYPE '{text}' into the 'Shadow Field' field").should_see(f"Echo: {text}")  # type: ignore[return-value]
