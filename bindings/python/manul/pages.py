"""Page objects over a Session.

Manul needs no selectors, so a page object is not a bag of locators. It is a
page's name and a vocabulary: methods that say what a person does there, each
a few plain-English lines, or one of the actions the page's own page file
declares (``pages/login.hunt``: ``@page: Login``, ``STEP SignIn:``). The
engine still resolves every element, and it — not this class — decides
whether the page is open: ``VERIFY that the page is 'Login'`` asks for the
page's name in ``pages/``, its title, and what its page file ``@expect``\\ s.

    class LoginPage(manul.Page):
        name = "Login"

        def sign_in(self, user, password):
            return self.use("SignIn", user=user, password=password)

    LoginPage(session).open().sign_in("ada", "lovelace")
"""

from __future__ import annotations

from typing import TypeVar

from .session import RunOutcome, Session, StepOutcome

P = TypeVar("P", bound="Page")


class StepFailed(AssertionError):
    """A step a page object ran did not pass. Carries the engine's outcome:
    its reason and the closest real labels."""

    def __init__(self, outcome: StepOutcome | RunOutcome, step: str = "") -> None:
        self.outcome = outcome
        what = step or getattr(outcome, "step", "") or "the steps"
        reason = getattr(outcome, "reason", "") or "failed"
        error = getattr(outcome, "error", "")
        near = ", ".join(str(c.get("label", c.get("text", c))) for c in (getattr(outcome, "near", None) or [])[:3])
        super().__init__(
            f"{what!r} failed: {reason}"
            + (f" — {error}" if error else "")
            + (f" (closest: {near})" if near else "")
        )


class Page:
    """A page of the app under test.

    ``name`` is the page's name in ``pages/``: a page file's ``@page``, or a
    name a JSON fragment gives. ``path`` is where the page is under
    ``base_url``, for a page ``pages/`` has no address for.
    """

    #: The page's name in pages/.
    name: str = ""
    #: Where the page is under base_url, when pages/ does not know.
    path: str = ""

    def __init__(self, session: Session, base_url: str = "") -> None:
        self.s = session
        self.base_url = base_url.rstrip("/")

    def do(self: P, *steps: str) -> P:
        """Run DSL lines in order. A step that fails raises StepFailed.

        The session returns a failure rather than raising — not finding an
        element is an answer an agent reacts to — but for a test it is fatal,
        and a page object is where that is decided.
        """
        for step in steps:
            outcome = self.s.step(step)
            if not outcome:
                raise StepFailed(outcome, step)
        return self

    def open(self: P) -> P:
        """Go to the page — by its address under base_url when ``path`` is
        set, by its name in pages/ otherwise — and wait until it is open."""
        if self.path and self.base_url:
            self.do(f"NAVIGATE to {self.base_url}{self.path}")
        else:
            self.do(f"NAVIGATE to '{self._name()}'")
        return self.assert_loaded()

    def assert_loaded(self: P) -> P:
        """Wait, as long as a step waits, until the engine says this page is
        open: its name in pages/ or its title, and every text its page file
        @expects."""
        return self.do(f"VERIFY that the page is '{self._name()}'")

    def use(self: P, block: str, **variables: object) -> P:
        """Run one of the page file's actions — ``USE <name>.<block>`` —
        with the given variables set first."""
        if variables:
            self.s.set_vars(**{k: str(v) for k, v in variables.items()})
        step = f"USE {self._name()}.{block}"
        outcome = self.s.run(step)
        if not outcome:
            raise StepFailed(outcome, step)
        return self

    def should_see(self: P, *texts: str) -> P:
        return self.do(*(f"VERIFY that '{t}' is present" for t in texts))

    def should_not_see(self: P, *texts: str) -> P:
        return self.do(*(f"VERIFY that '{t}' is NOT present" for t in texts))

    def value_of(self, label: str) -> str:
        """Read one labelled value without a full page scan."""
        v = self.s.read(label)
        if not v:
            raise AssertionError(f"{label!r} is not on {self._name()}: {v.reason}")
        return v.value

    def _name(self) -> str:
        if not self.name:
            raise ValueError(f"{type(self).__name__} has no name: set name to the page's name in pages/")
        return self.name
