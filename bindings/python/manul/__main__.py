"""`python -m manul` — the same shim as the `manul` console script."""

from __future__ import annotations

import sys

from ._cli import main

if __name__ == "__main__":
    sys.exit(main())
