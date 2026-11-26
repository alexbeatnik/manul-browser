"""Manul Python demo: everything the binding can do, against a local site.

Importing ``demo.handlers`` registers every custom control, CALL HOST handler
and suite hook with the binding. The ``manul.Session`` that opens afterwards
publishes them to the engine — no session is needed at import time.
"""

from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
HUNTS = ROOT / "hunts"
