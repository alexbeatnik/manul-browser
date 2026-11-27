"""Page objects: what they ask the engine, against a session that records it."""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import manul  # noqa: E402
from manul.session import RunOutcome, StepOutcome, Value  # noqa: E402


class Recorder:
    """Stands in for a Session: records what it is asked, fails a step that
    names 'Missing'."""

    def __init__(self) -> None:
        self.calls: list[tuple[str, object]] = []

    def step(self, instruction: str) -> StepOutcome:
        self.calls.append(("step", instruction))
        ok = "Missing" not in instruction
        return StepOutcome(ok=ok, step=instruction, reason="ok" if ok else "not_found",
                           near=[] if ok else [{"label": "Sign in", "score": 0.4}])

    def run(self, source: str) -> RunOutcome:
        self.calls.append(("run", source))
        return RunOutcome(ok="Missing" not in source, total_steps=1, passed=1)

    def set_vars(self, **values: str) -> dict[str, str]:
        self.calls.append(("set_vars", values))
        return values

    def read(self, label: str) -> Value:
        self.calls.append(("read", label))
        return Value(value="24.50", found=True, reason="ok")


class Login(manul.Page):
    name = "Login"


def test_open_goes_by_name_and_asks_the_engine_whether_the_page_is_open():
    s = Recorder()
    Login(s).open()
    assert s.calls == [("step", "NAVIGATE to 'Login'"), ("step", "VERIFY that the page is 'Login'")]


def test_open_by_path_when_one_is_given():
    class Shop(manul.Page):
        name = "Shop"
        path = "/shop.html"

    s = Recorder()
    Shop(s, "http://localhost:8000/").open()
    assert s.calls[0] == ("step", "NAVIGATE to http://localhost:8000/shop.html")


def test_use_runs_the_page_files_action_with_its_variables():
    s = Recorder()
    Login(s).use("SignIn", user="ada", password="lovelace")
    assert s.calls == [("set_vars", {"user": "ada", "password": "lovelace"}), ("run", "USE Login.SignIn")]


def test_a_failed_step_raises_with_the_reason_and_the_closest_labels():
    s = Recorder()
    try:
        Login(s).do("CLICK the 'Missing' button")
    except manul.StepFailed as e:
        assert "not_found" in str(e) and "Sign in" in str(e)
        assert e.outcome.reason == "not_found"
    else:
        raise AssertionError("a failed step did not raise")


def test_a_page_without_a_name_says_so():
    try:
        manul.Page(Recorder()).open()
    except ValueError as e:
        assert "has no name" in str(e)
    else:
        raise AssertionError("a page with no name opened")


def test_value_of_reads_one_label():
    assert Login(Recorder()).value_of("Price") == "24.50"
