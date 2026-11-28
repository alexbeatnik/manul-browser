import { test, expect } from 'playwright/test';
import { HomePage, home } from '../pages/home';
import { QuotesPage } from '../pages/quotes';

const FIRST = 'Simple things should be simple.';
const SECOND = 'Make it work, make it right, make it fast.';

test('quotes_cycle', async ({ page }) => {
  const q = new QuotesPage(page);
  await test.step('Open the quotes', async () => {
    await page.goto('/quotes.html');
    await expect(q.text(FIRST)).toBeVisible();
    await expect(q.text('Alan Kay')).toBeVisible();
  });
  await test.step('The next quote', async () => {
    await q.next.click();
    await expect(q.text(SECOND)).toBeVisible();
    await expect(q.text('Kent Beck')).toBeVisible();
  });
  await test.step('Round to the first', async () => {
    await q.next.click();
    await expect(q.text(FIRST)).toBeVisible();
    await expect(q.text('Kent Beck')).toBeHidden();
  });
  await test.step('The second again', async () => {
    await q.next.click();
    await expect(q.text('Kent Beck')).toBeVisible();
  });
  await test.step('The first again', async () => {
    await q.next.click();
    await expect(q.text('Alan Kay')).toBeVisible();
  });
  await test.step('The second once more', async () => {
    await q.next.click();
    await expect(q.text(SECOND)).toBeVisible();
  });
  await test.step('Go home', async () => {
    await home(page).click();
    await new HomePage(page).expectOpen();
  });
});
