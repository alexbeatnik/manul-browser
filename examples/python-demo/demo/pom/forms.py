from __future__ import annotations

import json
from pathlib import Path

from .base import BasePage


class FormsPage(BasePage):
    path = "/forms.html"
    title = "Forms Page"

    def fill_profile(self, *, name: str, email: str, phone: str, bio: str) -> "FormsPage":
        return self.do(  # type: ignore[return-value]
            f"FILL 'Full name' field with '{name}'",
            f"FILL 'Email' field with '{email}'",
            f"TYPE '{phone}' into the 'Phone' field",
            f"FILL 'Bio' field with '{bio}'",
        )

    def choose(self, *, country: str, plan: str, newsletter: bool) -> "FormsPage":
        self.do(f"SELECT '{country}' from the 'Country' dropdown", f"CLICK the radio button for '{plan}'")
        self.do(("CHECK" if newsletter else "UNCHECK") + " the checkbox for 'Newsletter'")
        return self

    def set_delivery(self, date: str) -> "FormsPage":
        # Handled by the demo.controls.delivery_date custom control.
        return self.do(f"FILL 'Delivery Date' field with '{date}'")  # type: ignore[return-value]

    def upload_avatar(self, path: Path) -> "FormsPage":
        return self.do(f"UPLOAD '{path}' to 'Avatar'").should_see(f"Uploaded: {path.name}")  # type: ignore[return-value]

    def submit(self) -> dict:
        self.do(
            "VERIFY that 'Submit' is disabled",
            "CHECK the checkbox for 'I accept the Terms'",
            "VERIFY that 'Submit' is enabled",
            "CLICK the 'Submit' button",
        ).should_see("Form submitted")
        # read_text: the visible text of a CSS region, for the non-labelled bits.
        return json.loads(self.s.read_text("#summary"))
