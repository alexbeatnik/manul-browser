import path from 'node:path';
import { test, expect } from 'playwright/test';
import { HomePage, home, link } from '../pages/home';
import { QuotesPage } from '../pages/quotes';

const MOCK = path.join(__dirname, '..', 'mocks', 'quotes.json');

test('quotes_mocked', async ({ page }) => {
  const q = new QuotesPage(page);
  await test.step('Answer the API from a file', async () => {
    await page.route('**/api/quotes.json', (route) => route.fulfill({ path: MOCK }));
    await page.goto('/quotes.html');
    await expect(q.text('Mocked one.')).toBeVisible();
    await expect(q.text('Alan Kay')).toBeHidden();
  });
  await test.step('The next quote', async () => {
    await q.next.click();
    await expect(q.text('Mocked two.')).toBeVisible();
    await expect(q.text('Pallas')).toBeVisible();
  });
  await test.step('Round to the first', async () => {
    await q.next.click();
    await expect(q.text('Mocked one.')).toBeVisible();
  });
  await test.step('The second again', async () => {
    await q.next.click();
    await expect(q.text('Mocked two.')).toBeVisible();
  });
  await test.step('Go home', async () => {
    await home(page).click();
    await new HomePage(page).expectOpen();
  });
  await test.step('Back to the quotes', async () => {
    await link(page, 'Quote of the day').click();
    await expect(q.text('Mocked one.')).toBeVisible();
  });
  await test.step('Still mocked', async () => {
    await q.next.click();
    await expect(q.text('Mocked two.')).toBeVisible();
    await expect(q.text('Kent Beck')).toBeHidden();
  });
});
