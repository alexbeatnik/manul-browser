// Drive the engine from Node, a step at a time.
//
//   npm install manul-browser
//   node drivers/quickstart.mjs
//
// Run it from examples/quickstart with the site being served (see README.md).

import { fileURLToPath } from 'node:url';
import { Session } from 'manul-browser';

const SITE = 'http://localhost:8000';
const hunt = fileURLToPath(new URL('../02_login.hunt', import.meta.url));

const s = await Session.launch({ headless: true });
try {
  // One DSL line per call; the outcome says what was done and how sure it was.
  await s.step(`NAVIGATE to ${SITE}/login.html`);
  await s.step("FILL 'Username' field with 'ada'");
  await s.step("FILL 'Password' field with 'lovelace'");
  const out = await s.step("CLICK the 'Sign in' button");
  console.log(`${out.action}: ok=${out.ok} score=${out.score.toFixed(2)}`);

  // read() asks for one labelled thing; not found is an answer, not an error.
  console.log('role:', (await s.read('Role')).value);

  // map() is what the page offers, grouped — the short way to show it to an LLM.
  console.log('labels:', (await s.map()).labels().slice(0, 10));

  // Variables live as long as the session does.
  await s.step(`NAVIGATE to ${SITE}/shop.html`);
  await s.step("CLICK the 'Add to cart' button NEAR 'Snow Boots'");
  await s.step("EXTRACT the 'Cart total' into {total}");
  console.log('total:', (await s.vars('total')).total);

  // A whole .hunt file runs in the same browser. A failing step throws.
  const result = await s.run({ path: hunt });
  console.log(`02_login.hunt: ${result.passed}/${result.totalSteps} steps passed`);
} finally {
  await s.close();
}
