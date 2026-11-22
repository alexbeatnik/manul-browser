// The same page objects on Firefox, driven over WebDriver BiDi instead of CDP.
// Nothing in the hunts or page objects changes.

import { join } from 'node:path';
import { ROOT, banner, check, withAppAndSession } from './_common.mjs';
import '../src/handlers.mjs';
import { FormsPage, LoginPage, WidgetsPage } from '../src/pom/index.mjs';

await withAppAndSession({ browser: 'firefox', headless: true }, async (app, s) => {
  banner(`Firefox session: browser=${s.browser}, mode=${s.mode}`);
  check(s.browser === 'firefox', 'launched Firefox');
  const dash = await (await new LoginPage(s, app.baseUrl).open()).loginAs('grace', 'hopper');
  check((await dash.role()) === 'member', 'logged in as grace on Firefox');
  const forms = await new FormsPage(s, app.baseUrl).open();
  await forms.setDelivery('2027-01-01');
  await forms.uploadAvatar(join(ROOT, 'fixtures', 'avatar.png'));
  const w = await (await new WidgetsPage(s, app.baseUrl).open()).waitReady();
  await w.sign('Grace');
  await w.rate(2);
  await s.setVars({ base_url: app.baseUrl, signer: 'Grace' });
  const res = await s.run({ path: join(ROOT, 'hunts', '03_widgets.hunt') });
  check(res.ok, `03_widgets.hunt on Firefox: ${res.passed}/${res.totalSteps}`);
});
