// CALL HOST handlers — plain functions a .hunt can call.
//
//   CALL HOST demo.compute_total with args: "24.50" "89.99" into {total}
//   CALL GO demo.cleanup_db         # same command, other spelling
//
// A scalar result lands in `into {var}`; an object becomes several variables;
// null/undefined sets nothing.

import { randomBytes } from 'node:crypto';
import { call } from 'manul-browser';

async function api(ctx, method, path) {
  const base = ctx.vars.base_url || 'http://localhost:8766';
  // A bounded wait: a backend that is down fails the step instead of hanging it.
  const res = await fetch(`${base}${path}`, {
    method,
    body: method === 'POST' ? '{}' : undefined,
    signal: AbortSignal.timeout(5000),
  });
  return res.json();
}

call('demo.compute_total', (ctx) => ctx.args.reduce((s, a) => s + Number(a), 0).toFixed(2));

call('demo.upper', (ctx) => ctx.args.join(' ').toUpperCase());

call('demo.make_user', (ctx) => {
  const prefix = ctx.args[0] ?? 'user';
  const uid = randomBytes(3).toString('hex');
  return { new_user: `${prefix}_${uid}`, new_email: `${prefix}_${uid}@example.com`, new_id: uid };
});

// Runs in [SETUP], before any browser exists — talks to the backend only.
call('demo.seed_db', async (ctx) => {
  await api(ctx, 'POST', '/api/reset');
  return 'seeded';
});

call('demo.cleanup_db', async (ctx) => {
  await api(ctx, 'POST', '/api/reset');
  return null;
});

// Verify what the UI did, straight from the backend.
call('demo.api_order_count', async (ctx) => String((await api(ctx, 'GET', '/api/orders')).length));

// A CALL handler can reach into the live page too.
call('demo.page_title', async (ctx) => String(await ctx.eval('document.title')));

// A throwing handler fails the step — that is how a CALL asserts.
call('demo.assert_equal', (ctx) => {
  const [left = '', right = ''] = ctx.args;
  if (left !== right) throw new Error(`AssertionError: ${JSON.stringify(left)} !== ${JSON.stringify(right)}`);
  return null;
});
