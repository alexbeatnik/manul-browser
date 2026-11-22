// Page-object tests and hunt files, side by side under node:test.
//
// One app server and one browser for the whole file: a Session is expensive to
// open and cheap to reuse, so each test starts from a known page instead.

import { after, before, describe, it } from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync } from 'node:fs';
import { join } from 'node:path';
import { Session } from 'manul-browser';
import { ROOT } from '../scripts/_common.mjs';
import { startAppServer } from '../src/app-server.mjs';
import '../src/handlers.mjs';
import { CatalogPage, DashboardPage, FormsPage, LoginPage, StepFailed, WidgetsPage } from '../src/pom/index.mjs';

let app;
let s;

before(async () => {
  app = await startAppServer();
  s = await Session.launch({ cwd: ROOT, stderr: 'ignore', browser: process.env.MANUL_BROWSER || 'chromium' });
  await s.setVars({ base_url: app.baseUrl, signer: 'node:test', env_name: 'test', api_token: 'tok-test' });
});

after(async () => {
  await s?.close();
  await app?.close();
});

describe('page objects', () => {
  for (const [user, password, role] of [['ada', 'lovelace', 'admin'], ['alan', 'turing', 'member']]) {
    it(`logs in as ${user} with role ${role}`, async () => {
      const dash = await (await new LoginPage(s, app.baseUrl).open()).loginAs(user, password);
      assert.equal(await dash.role(), role);
      await dash.logout();
    });
  }

  it('stays on the login page after a bad password', async () => {
    const login = await new LoginPage(s, app.baseUrl).open();
    await login.loginExpectingError('ada', 'nope');
    await login.assertLoaded();
  });

  it('reads the dashboard and edits a user', async () => {
    await (await new LoginPage(s, app.baseUrl).open()).loginAs('ada', 'lovelace');
    const dash = await new DashboardPage(s, app.baseUrl).open();
    assert.equal(await dash.orderTotal(), 142.5);
    await dash.editUser('Grace Hopper');
  });

  it('round-trips the profile form', async () => {
    const forms = await new FormsPage(s, app.baseUrl).open();
    await forms.fillProfile({ name: 'Grace Hopper', email: 'grace@example.com', phone: '1', bio: 'COBOL' });
    await forms.choose({ country: 'Poland', plan: 'Free', newsletter: true });
    await forms.setDelivery('2026-12-01');
    assert.equal((await forms.submit()).delivery, '2026-12-01');
  });

  it('drives widgets through custom controls', async () => {
    const w = await (await new WidgetsPage(s, app.baseUrl).open()).waitReady();
    await w.sign('node:test');
    await w.rate(3);
  });

  it('adds to the cart by product name', async () => {
    const cat = await new CatalogPage(s, app.baseUrl).open();
    await cat.waitLoaded(3);
    await cat.addToCart('Steppe Map', 3);
    assert.equal(await cat.cartTotal(), 36);
  });

  it('throws StepFailed for a missing element', async () => {
    const login = await new LoginPage(s, app.baseUrl).open();
    await assert.rejects(() => login.do("CLICK the 'Forgot password' link"), (e) => e instanceof StepFailed && e.outcome.reason === 'not_found');
  });
});

// Each .hunt is a test too. 08 is the daemon's, 09 is meant to be skipped by a
// suite hook, 11 installs a session-wide MOCK and so would leak into others.
describe('hunts', () => {
  const hunts = readdirSync(join(ROOT, 'hunts')).filter((f) => f.endsWith('.hunt') && !['08', '09', '11'].includes(f.slice(0, 2))).sort();
  for (const name of hunts) {
    it(name, async () => {
      const res = await s.run({ path: join(ROOT, 'hunts', name) });
      assert.ok(res.ok, `${res.failed} of ${res.totalSteps} steps failed`);
    });
  }
});
