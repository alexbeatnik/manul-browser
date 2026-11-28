import { test, expect } from 'playwright/test';
import { HomePage, home } from '../pages/home';
import { SlowPage } from '../pages/slow';

test('slow_feed', async ({ page }) => {
  const s = new SlowPage(page);
  await test.step('Open the slow page', async () => {
    await page.goto('/dynamic.html');
    await expect(s.text('Feed item 2')).toBeVisible();
  });
  await test.step('More of the feed', async () => {
    await s.loadMore.click();
    await expect(s.text('Feed item 4')).toBeVisible();
  });
  await test.step('The rest of the feed', async () => {
    await s.loadMore.click();
    await expect(s.text('Feed item 6')).toBeVisible();
    await expect(s.loadMore).toBeHidden();
  });
  await test.step('Save the settings', async () => {
    await s.save.click();
    await expect(s.text('Settings saved')).toBeVisible();
  });
  await test.step('The toast goes', async () => {
    await expect(s.text('Settings saved')).toBeHidden();
  });
  await test.step('Build the report', async () => {
    await s.build.click();
    await expect(s.text('Report ready: 3 orders')).toBeVisible();
  });
  await test.step('Go home', async () => {
    await home(page).click();
    await new HomePage(page).expectOpen();
  });
});
