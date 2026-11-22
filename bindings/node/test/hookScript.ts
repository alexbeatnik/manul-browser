/**
 * A hook script, as `manul run --hooks` would start it.
 *
 * `hookHost.test.ts` plays the engine against this: it spawns the script,
 * reads what it writes and answers. `HOOK_MODE` picks what gets registered, so
 * one fixture covers the cases.
 */

import { beforeAll, beforeGroup, call, customControl, serveHooks } from '../src/index.js';

const mode = process.env['HOOK_MODE'] ?? 'full';

if (mode === 'full') {
  beforeAll((ctx) => {
    // stdout is the protocol. This must land on stderr, or the engine's parser
    // reads half a log line as JSON.
    console.log('log from a hook');
    console.info('info from a hook');
    ctx.set('token', 'seeded');
  });

  beforeGroup('smoke', async (ctx) => {
    ctx.set('title', String(await ctx.eval('document.title')));
    ctx.set('url', await ctx.url());
  });

  call('boom', () => {
    throw new RangeError('deliberate');
  });

  // A BigInt has no JSON form. That is this handler's failure, not the host's.
  call('bigint', () => 10n);

  call('echo', (ctx) => ctx.args.join(','));

  customControl({ page: 'Checkout', target: 'Datepicker' }, () => {});
}

await serveHooks();
