"""Example hook script for `manul run --hooks`.

Run it with:

    manul run ../saucedemo.hunt --hooks manul_hooks.py

The engine spawns this file, asks it what it owns, and then calls back into it
whenever a hunt reaches something it registered. Nothing here talks to Chrome or
parses a `.hunt` file — that is all engine. This process only answers.

Requires `pip install manul-browser`; the import is the whole dependency.
"""

import manul


@manul.before_all
def sign_in(ctx):
    """Runs once, before any browser exists.

    Whatever this publishes into `ctx.variables` becomes a `{placeholder}` in
    every hunt of the run — the usual reason to reach for a suite hook at all.
    """
    print("suite starting")  # stdout is the protocol, so this goes to stderr
    ctx.variables["token"] = "seeded-once-for-the-whole-suite"


@manul.after_all
def clean_up(ctx):
    """Always runs, even when the suite failed."""
    print("suite finished; token was", ctx.variables.get("token"))


@manul.before_group("smoke")
def per_smoke_hunt(ctx):
    """Runs before each hunt whose header carries `@tags: smoke`.

    Under `--workers > 1` this fires from several hunts at once. The engine
    serialises the callbacks over the single pipe, so no locking is needed here.
    """
    ctx.variables["run_kind"] = "smoke"


@manul.call("py.upper")
def upper(ctx):
    """Reachable from a hunt as `CALL HOST py.upper "text" into {var}`.

    A returned scalar lands in `into {var}`; a returned dict sets several
    variables at once.
    """
    return ctx.args[0].upper() if ctx.args else ""


@manul.custom_control(page="*", target="Cookie Consent")
def dismiss_cookies(ctx):
    """Intercepts a step aimed at this element, anywhere in the suite.

    The hunt keeps a single readable line; the awkward part lives here, with the
    page reachable through the same primitives an embedded Go handler gets.
    """
    ctx.eval("document.querySelector('#cookie-accept')?.click()")


# Blocks until the engine closes this process's input, which it does after the
# last after-all hook. Must be the last line.
manul.serve_hooks()
