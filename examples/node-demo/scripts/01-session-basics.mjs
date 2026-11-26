// The Session API, method by method.
//
// step · run({source}) · map · read · readText · state · vars · setVars · schema
// plus what a session reports about itself, and what a failed step looks like.

import { banner, check, withAppAndSession } from './_common.mjs';

await withAppAndSession({}, async (app, s) => {
  banner('Session');
  console.log(`  engine ${s.engineVersion}, protocol ${s.protocol}, mode=${s.mode}, browser=${s.browser}`);
  console.log(`  handlers published on open: ${JSON.stringify(s.published)}`);
  check(s.mode === 'launch' && s.browser === 'chromium', 'launched Chromium (manul.config.json: headless)');

  banner('step() → StepOutcome');
  let out = await s.step(`NAVIGATE to ${app.baseUrl}/index.html`);
  check(out.ok && out.action, `NAVIGATE ok, action=${out.action}, url=${out.url}`);
  out = await s.step("FILL 'Username' field with 'ada'");
  check(out.ok && out.score > 0, `FILL resolved with score ${out.score.toFixed(2)}`);

  banner('A miss is an outcome, not an exception');
  const miss = await s.step("CLICK the 'Launch rockets' button");
  check(!miss.ok, `ok=false, reason=${miss.reason}`);
  console.log(`  near: ${JSON.stringify(miss.near.map((c) => [c.text, c.score]))}`);

  banner('state() / map() / read() / readText()');
  const state = await s.state();
  check(state.title === 'Login Page', `state: ${JSON.stringify(state)}`);
  const page = await s.map();
  for (const g of page.groups) {
    console.log(`  [${g.name}] ` + g.elements.map((e) => `${e.label}(${e.role}${e.editable ? ', editable' : ''})`).join(', '));
  }
  check(page.labels().includes('Sign in'), "map() lists 'Sign in'");
  check((await s.map({ maxPerGroup: 1, includeUnlabeled: true })).groups.length > 0, 'map({ maxPerGroup: 1, includeUnlabeled: true })');
  check((await s.read('Username')).value === 'ada', "read('Username') reads back what FILL wrote");
  check(!(await s.read('Nonexistent widget')).found, 'read() of a missing label → found=false, no exception');
  const text = await s.readText('main', { maxChars: 7 });
  check(text.startsWith('Sign in') && text.includes('truncated'), `readText('main', { maxChars: 7 }) = ${JSON.stringify(text)}`);

  banner('Variables live for the whole session');
  await s.setVars({ base_url: app.baseUrl, password: 'lovelace' });
  await s.step("FILL 'Password' field with '{password}'");
  await s.step("CLICK the 'Sign in' button");
  await s.step("EXTRACT the 'Order total' into {total}");
  const picked = await s.vars('total', 'missing');
  check(picked.total === '142.50' && picked.missing === '', `vars('total', 'missing') = ${JSON.stringify(picked)}`);
  check('base_url' in (await s.vars()), 'vars() with no names returns everything');

  banner("run({ source }) — a hunt as a string, sharing this session's state");
  const res = await s.run({
    source: "STEP 1: inline\n    VERIFY that 'Welcome, ada' is present\n    SET {seen} = yes\n    PRINT \"total was {total}\"\n",
  });
  check(res.ok && res.passed === res.totalSteps, `RunOutcome passed ${res.passed}/${res.totalSteps}`);

  banner('schema() — the engine describes its own DSL');
  const schema = await s.schema();
  console.log(`  keys: ${Object.keys(schema).sort().slice(0, 8).join(', ')}…`);
  check(schema.verbs, 'schema() returned the contract');

  banner('transport.call — the supported escape hatch');
  check((await s.transport.call('state')).url.endsWith('/dashboard.html'), 'raw protocol command');
});
