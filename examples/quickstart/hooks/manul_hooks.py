"""Hook script for 12_call_your_code.hunt.

    pip install manul-browser
    manul 12_call_your_code.hunt --hooks hooks/manul_hooks.py

The engine starts this file and calls back into it. stdout is the protocol, so
print() here goes to stderr.
"""

import itertools
import time

import manul

_users = itertools.count(1)


@manul.before_all
def publish_globals(ctx):
    """Runs once, before the first hunt. What it sets becomes {placeholders}."""
    ctx.variables["site"] = "http://localhost:8000"
    ctx.variables["run_id"] = time.strftime("%H%M%S")


@manul.call("qs.new_user")
def new_user(ctx):
    """CALL HOST qs.new_user into {user} — a returned scalar lands in {user}."""
    return f"user{next(_users)}@example.com"


@manul.call("qs.sum")
def add(ctx):
    """CALL HOST qs.sum "1.50" "2" into {total} — arguments arrive as strings."""
    return f"{sum(float(a) for a in ctx.args):.2f}"


manul.serve_hooks()  # blocks until the run ends; must be last
