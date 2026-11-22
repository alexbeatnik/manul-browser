// session.runSuite: beforeAll / afterAll / beforeGroup / afterGroup.

import { readdirSync } from 'node:fs';
import { join } from 'node:path';
import { ROOT, banner, check, withAppAndSession } from './_common.mjs';
import { EVENTS } from '../src/handlers.mjs';

const HUNTS = readdirSync(join(ROOT, 'hunts'))
  .filter((f) => f.endsWith('.hunt') && f !== '08_scheduled.hunt')
  .sort()
  .map((f) => join(ROOT, 'hunts', f));

const res = await withAppAndSession({}, (app, s) => s.runSuite(HUNTS));

banner('SuiteResult');
for (const h of res.hunts) {
  const state = h.skipped ? 'SKIP' : h.ok ? 'PASS' : 'FAIL';
  console.log(`  ${state} ${h.path.split('/').pop().padEnd(24)} tags=${JSON.stringify(h.tags)} ${h.passed}/${h.steps} ${h.error}`);
}
console.log(`  total=${res.total} passed=${res.passed} failed=${res.failed} skipped=${res.skipped}`);
console.log(`  globals at the end: ${JSON.stringify(res.vars)}`);
console.log(`  hook events: ${JSON.stringify(EVENTS)}`);

const skipped = res.hunts.filter((h) => h.skipped).map((h) => h.path);
check(res.skipped === 1 && skipped[0].endsWith('09_quarantine.hunt'), "beforeGroup('quarantine') skipped its hunt");
check(res.failed === 0 && res.passed === res.total - 1, 'every other hunt passed');
check(res.vars.api_token === 'tok-local-42', 'beforeAll published globals');
check(EVENTS[0].startsWith('beforeAll') && EVENTS.at(-1).startsWith('afterAll'), 'afterAll ran last');
check(EVENTS.filter((e) => e === 'beforeGroup(auth)').length === 1 && EVENTS.filter((e) => e === 'afterGroup(auth)').length === 1, 'group hooks bracket tagged hunts');
// `ok` is false because a skipped hunt is not a pass — that is by design.
