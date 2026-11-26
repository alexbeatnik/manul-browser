"""Hook script for the CLI: ``manul run hunts/ --hooks manul_hooks.py``.

The engine spawns this file and calls back into it. It reuses the very same
handlers the Python scripts use — registration is just importing them.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import manul  # noqa: E402

import demo.handlers  # noqa: E402,F401 — registers controls, calls and hooks

manul.serve_hooks()  # blocks until the engine closes stdin; must be last
