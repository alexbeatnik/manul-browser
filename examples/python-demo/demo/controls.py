"""Custom controls — Python that takes over when a step aims at a widget.

A canvas, a readonly date picker and a star widget cannot be driven as
"click the thing called X". Each gets one function here, so the .hunt keeps
saying what it means (``FILL 'Delivery Date' field with '2026-12-24'``).

The ``page`` label is the page's ``document.title``: that is what the engine
looks a control up by. ``"*"`` (``manul.ANY_PAGE``) matches every page, and a
page-specific handler beats a wildcard for the same target.
"""

from __future__ import annotations

import json

import manul

# Which handlers ran, in order. The demo scripts print this to prove the
# engine called back into Python instead of resolving the element itself.
CALLS: list[str] = []


@manul.custom_control(page="Forms Page", target="Delivery Date")
def delivery_date(ctx: manul.ControlContext) -> None:
    """A readonly input with a popup calendar. FILL goes through its JS API."""
    CALLS.append(f"delivery_date:{ctx.action}:{ctx.value}")
    if ctx.action != "input":
        raise ValueError(f"Delivery Date only supports FILL/TYPE, got {ctx.action!r}")
    ctx.eval(f"window.datePicker.set({json.dumps(ctx.value)})")


@manul.custom_control(page="Widgets Page", target="Signature Pad")
def signature_pad(ctx: manul.ControlContext) -> None:
    """A <canvas>. CLICK signs it with the {signer} variable from the hunt."""
    signer = ctx.vars.get("signer") or "Anonymous"
    CALLS.append(f"signature_pad:{ctx.action}:{signer}")
    ctx.eval(f"window.signaturePad.sign({json.dumps(signer)})")


@manul.custom_control(target="Star Rating")  # page defaults to "*" — any page
def star_rating(ctx: manul.ControlContext) -> None:
    """SELECT '4' from the 'Star Rating' dropdown → four stars."""
    CALLS.append(f"star_rating:{ctx.action}:{ctx.value}")
    stars = int(ctx.value or 0)
    if not 0 <= stars <= 5:
        raise ValueError(f"rating must be 0..5, got {stars}")
    ctx.eval(f"window.starRating.set({stars})")
    # current_url() is the other page primitive available mid-step.
    assert ctx.current_url().endswith("/widgets.html"), ctx.current_url()


@manul.custom_control(page=manul.ANY_PAGE, target="Cookie Banner")
def cookie_banner(ctx: manul.ControlContext) -> None:
    """Wildcard control: harmless on pages without a banner."""
    CALLS.append(f"cookie_banner:{ctx.action}")
    ctx.eval("document.querySelector('#cookie-banner')?.remove(); true")
