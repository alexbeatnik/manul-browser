from __future__ import annotations

import manul


class StepFailed(AssertionError):
    """A step returned ok=False. Carries the engine's reason and nearby candidates."""

    def __init__(self, outcome: manul.StepOutcome) -> None:
        self.outcome = outcome
        near = ", ".join(str(c.get("text", c)) for c in outcome.near[:3])
        super().__init__(
            f"{outcome.step!r} failed: {outcome.reason}"
            + (f" — {outcome.error}" if outcome.error else "")
            + (f" (nearby: {near})" if near else "")
        )


class BasePage:
    #: Path under the site root.
    path = "/"
    #: document.title — also the page label custom controls are registered under.
    title = ""

    def __init__(self, session: manul.Session, base_url: str) -> None:
        self.s = session
        self.base_url = base_url.rstrip("/")

    # ── plumbing ────────────────────────────────────────────────────────────

    def do(self, *steps: str) -> "BasePage":
        """Run DSL lines in order. A step that fails raises StepFailed.

        The binding deliberately returns failures instead of raising; a page
        object is the right place to decide that, for a test, a miss is fatal.
        """
        for step in steps:
            outcome = self.s.step(step)
            if not outcome:
                raise StepFailed(outcome)
        return self

    def open(self) -> "BasePage":
        return self.do(f"NAVIGATE to {self.base_url}{self.path}").assert_loaded()

    def assert_loaded(self) -> "BasePage":
        state = self.s.state()
        if self.title and state["title"] != self.title:
            raise AssertionError(f"expected page {self.title!r}, on {state['title']!r} ({state['url']})")
        return self

    def should_see(self, *texts: str) -> "BasePage":
        return self.do(*(f"VERIFY that '{t}' is present" for t in texts))

    def should_not_see(self, *texts: str) -> "BasePage":
        return self.do(*(f"VERIFY that '{t}' is NOT present" for t in texts))

    def value_of(self, label: str) -> str:
        """Read one labelled value without a full page scan."""
        v = self.s.read(label)
        if not v:
            raise AssertionError(f"{label!r} not on {self.title}: {v.reason}")
        return v.value
