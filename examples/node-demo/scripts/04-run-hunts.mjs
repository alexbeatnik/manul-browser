// session.run({ path }): one .hunt at a time, sharing one browser.
//
// A single run is not a suite, so no suite hooks fire. The globals they would
// publish ({base_url}, {signer}, …) are seeded with setVars instead.

import { readdirSync } from 'node:fs';
import { join } from 'node:path';
import { EngineError } from 'manul-browser';
import { ROOT, banner, check, withAppAndSession } from './_common.mjs';
import '../src/handlers.mjs';

// 08 is for the daemon, 09 is refused by a suite hook — neither belongs here.
const HUNTS = readdirSync(join(ROOT, 'hunts')).filter((f) => f.endsWith('.hunt') && !['08', '09'].includes(f.slice(0, 2))).sort();

const failed = await withAppAndSession({}, async (app, s) => {
  await s.setVars({ base_url: app.baseUrl, env_name: 'local', signer: 'Manul QA', api_token: 'tok-local-42' });
  const failed = [];
  for (const name of HUNTS) {
    try {
      const res = await s.run({ path: join(ROOT, 'hunts', name) });
      console.log(`  ${res.ok ? '\x1b[32m✓' : '\x1b[31m✗'}\x1b[0m ${name.padEnd(24)} ${res.passed}/${res.totalSteps} steps  (${res.url})`);
      if (!res.ok) failed.push(name);
    } catch (e) {
      // A hunt that fails a step answers step_failed, so run() rejects.
      if (!(e instanceof EngineError)) throw e;
      console.log(`  \x1b[31m✗\x1b[0m ${name.padEnd(24)} ${e.code}: ${e.message}`);
      failed.push(name);
    }
  }
  banner('Variables a hunt left behind are still in the session');
  const v = await s.vars('order_total', 'country', 'ui_total', 'orders');
  console.log('  ', JSON.stringify(v));
  return failed;
});
check(failed.length === 0, failed.length ? `failed: ${failed}` : `all ${HUNTS.length} hunts passed`);
