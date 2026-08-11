"""Exceptions raised by the Manul binding."""

from __future__ import annotations

__all__ = [
    "ManulError",
    "EngineNotFound",
    "ProtocolError",
    "EngineError",
    "SessionClosed",
]


class ManulError(Exception):
    """Base class for every error this package raises."""


class EngineNotFound(ManulError):
    """The `manul` binary could not be located.

    Carries the places that were searched so the message is actionable rather
    than a bare "not found".
    """

    def __init__(self, searched: list[str]) -> None:
        self.searched = searched
        super().__init__(
            "could not find the manul binary. Looked in:\n  "
            + "\n  ".join(searched)
            + "\nSet MANUL_BINARY to an explicit path, or put `manul` on PATH."
        )


class ProtocolError(ManulError):
    """The engine spoke something this binding does not understand.

    Raised for an unreadable line, a missing ready event, or a major protocol
    version this binding was not written against.
    """


class EngineError(ManulError):
    """The engine rejected a request.

    This is a normal, recoverable answer — the session stays usable. `code` is
    the machine-readable reason (``bad_request``, ``not_open``, ``internal``…).
    """

    def __init__(self, code: str, message: str) -> None:
        self.code = code
        self.message = message
        super().__init__(f"{code}: {message}")


class SessionClosed(ManulError):
    """A call was made on a session that is already closed."""
