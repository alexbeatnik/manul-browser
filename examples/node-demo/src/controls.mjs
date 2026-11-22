// Custom controls — JavaScript that takes over when a step aims at a widget.
//
// A canvas, a readonly date picker and a star widget cannot be driven as
// "click the thing called X". Each gets one function here, so the .hunt keeps
// saying what it means (FILL 'Delivery Date' field with '2026-12-24').
//
// The `page` label is the page's document.title: that is what the engine looks
// a control up by. ANY_PAGE ('*', also the default) matches every page, and a
// page-specific handler beats a wildcard for the same target.

import { ANY_PAGE, customControl } from 'manul-browser';

/** Which handlers ran, in order — proof the engine called back into Node. */
export const CALLS = [];

customControl({ page: 'Forms Page', target: 'Delivery Date' }, async (ctx) => {
  CALLS.push(`delivery_date:${ctx.action}:${ctx.value}`);
  if (ctx.action !== 'input') {
    throw new Error(`Delivery Date only supports FILL/TYPE, got ${JSON.stringify(ctx.action)}`);
  }
  await ctx.eval(`window.datePicker.set(${JSON.stringify(ctx.value)})`);
});

customControl({ page: 'Widgets Page', target: 'Signature Pad' }, async (ctx) => {
  const signer = ctx.vars.signer || 'Anonymous';
  CALLS.push(`signature_pad:${ctx.action}:${signer}`);
  await ctx.eval(`window.signaturePad.sign(${JSON.stringify(signer)})`);
});

// The string form registers for any page.
customControl('Star Rating', async (ctx) => {
  CALLS.push(`star_rating:${ctx.action}:${ctx.value}`);
  const stars = Number(ctx.value || 0);
  if (!(stars >= 0 && stars <= 5)) throw new Error(`rating must be 0..5, got ${ctx.value}`);
  await ctx.eval(`window.starRating.set(${stars})`);
  // url() is the other page primitive available mid-step.
  const url = await ctx.url();
  if (!url.endsWith('/widgets.html')) throw new Error(`unexpected page ${url}`);
});

customControl({ page: ANY_PAGE, target: 'Cookie Banner' }, async (ctx) => {
  CALLS.push(`cookie_banner:${ctx.action}`);
  await ctx.eval("document.querySelector('#cookie-banner')?.remove(); true");
});
