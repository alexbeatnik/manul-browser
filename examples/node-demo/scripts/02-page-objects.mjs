// A user journey written against page objects (src/pom).

import { join } from 'node:path';
import { ROOT, banner, check, withAppAndSession } from './_common.mjs';
import { CALLS } from '../src/handlers.mjs'; // registers the custom controls the pages rely on
import { CatalogPage, FormsPage, LoginPage, StepFailed, WidgetsPage } from '../src/pom/index.mjs';

await withAppAndSession({}, async (app, s) => {
  banner('Login → Dashboard');
  const login = await new LoginPage(s, app.baseUrl).open();
  await login.loginExpectingError('ada', 'nope');
  const dash = await login.loginAs('ada', 'lovelace', { remember: true });
  const [role, total] = [await dash.role(), await dash.orderTotal()];
  check(role === 'admin' && total === 142.5, `role=${role}, total=${total}`);
  await dash.editUser('Grace Hopper');
  await dash.deleteUser('Alan Turing');
  check((await dash.loadWholeFeed()) === 4, 'WHILE loop loaded the whole feed');

  banner('Forms (custom control: Delivery Date)');
  const forms = await new FormsPage(s, app.baseUrl).open();
  await forms.fillProfile({ name: 'Ada Lovelace', email: 'ada@example.com', phone: '+380440000000', bio: 'Notes on the Engine' });
  await forms.choose({ country: 'Ukraine', plan: 'Pro', newsletter: false });
  await forms.setDelivery('2026-12-24');
  await forms.uploadAvatar(join(ROOT, 'fixtures', 'avatar.png'));
  const summary = await forms.submit();
  check(
    JSON.stringify(summary) === JSON.stringify({ full_name: 'Ada Lovelace', email: 'ada@example.com', phone: '+380440000000', bio: 'Notes on the Engine', country: 'ua', plan: 'pro', delivery: '2026-12-24' }),
    `submitted ${JSON.stringify(summary)}`,
  );

  banner('Widgets (custom controls: Signature Pad, Star Rating)');
  const w = await (await new WidgetsPage(s, app.baseUrl).open()).waitReady();
  await w.moveCardToDone();
  await w.sign('Ada');
  await w.rate(5);
  await w.typeInShadow('hello shadow');

  banner('Catalog: NEAR, readonly totals, backend, MOCK');
  const c = await new CatalogPage(s, app.baseUrl).open();
  await c.waitLoaded(3);
  await c.addToCart('Manul Plush', 2);
  await c.addToCart('Steppe Map');
  const cart = await c.cartTotal();
  check(cart === 61, `cart total ${cart}`);
  check((await c.checkout()).includes('placed'), 'checkout placed an order');
  // MOCK lasts for the rest of the session, so it goes last.
  await c.useMock('mocks/products.json');
  await c.waitLoaded(2);
  await c.shouldSee('Mocked Yurt');

  banner('A page object turns a miss into a test failure');
  try {
    await c.do("CLICK the 'Pay with gold' button");
    check(false, 'expected StepFailed');
  } catch (e) {
    check(e instanceof StepFailed && e.outcome.reason, `StepFailed: ${e.message}`);
  }

  console.log('\n  custom controls that ran:', CALLS);
  check(CALLS.some((x) => x.startsWith('delivery_date')), 'engine called back into Node');
});
