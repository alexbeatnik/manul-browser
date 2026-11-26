// Custom controls and CALL HOST from the session side, plus the registry API.

import * as manul from 'manul-browser';
import { banner, check, withAppAndSession } from './_common.mjs';
import { CALLS } from '../src/handlers.mjs';

banner('Registry introspection — before any session exists');
for (const c of manul.listCustomControls()) console.log(`  control  ${c.page.padStart(14)} / ${c.target}`);
console.log(`  calls    ${JSON.stringify(manul.listCalls())}`);
console.log(`  hooks    ${JSON.stringify(manul.listHooks())}`);
check(manul.getCustomControl('forms page', '  delivery   DATE ') !== null, 'lookup ignores case and whitespace');
check(manul.getCustomControl('Any Page At All', 'Star Rating') !== null, "page '*' matches every page");
check(manul.getCustomControl('Dashboard', 'Signature Pad') === null, 'page-specific control does not leak');
const hint = manul.diagnoseCustomControlMiss('Dashboard', 'Signature Pad');
check(hint?.includes('Widgets Page'), `miss diagnosis: ${hint}`);
check(manul.getCall('DEMO.Upper') !== null, 'CALL names are case-insensitive too');
check(manul.getHooks('before_all', '').length === 2, 'two beforeAll handlers share one slot');

await withAppAndSession({}, async (app, s) => {
  banner('Published on open');
  check(s.published.controls === 4 && s.published.calls >= 8, `published ${JSON.stringify(s.published)}`);

  banner('CALL HOST from a single step');
  let out = await s.step('CALL HOST demo.compute_total with args: "1.25" "2.50" into {sum}');
  check(out.ok && (await s.vars('sum')).sum === '3.75', 'scalar result → into {sum}');
  await s.step('CALL GO demo.make_user "qa"'); // CALL GO = CALL HOST = CALL PYTHON
  const v = await s.vars('new_user', 'new_email');
  check(v.new_user.startsWith('qa_') && v.new_email.endsWith('@example.com'), `object result → ${JSON.stringify(v)}`);
  await s.step(`NAVIGATE to ${app.baseUrl}/widgets.html`);
  await s.step('CALL HOST demo.page_title into {title}');
  check((await s.vars('title')).title === 'Widgets Page', 'a CALL handler can ctx.eval() in the live page');

  banner('A throwing handler fails the step, not the session');
  let bad = await s.step('CALL HOST demo.assert_equal "a" "b"');
  check(!bad.ok && bad.error.includes('AssertionError'), `error: ${bad.error}`);
  bad = await s.step("SELECT '9' from the 'Star Rating' dropdown");
  check(!bad.ok && bad.error.includes('rating must be 0..5'), `control error: ${bad.error}`);
  check((await s.step("SELECT '3' from the 'Star Rating' dropdown")).ok, 'session still usable afterwards');

  banner('Registering more handlers on a live session');
  manul.call('demo.late', () => 'registered after open');
  console.log(`  publishHandlers() → ${JSON.stringify(await s.publishHandlers())}`);
  await s.step('CALL HOST demo.late into {late}');
  check((await s.vars('late')).late === 'registered after open', 'late handler reachable');

  banner('The cookie banner wildcard control on any page');
  out = await s.step("CLICK the 'Cookie Banner'");
  check(out.ok, 'wildcard control handled a target that is not on the page');
});
console.log('\n  controls that ran:', CALLS);
