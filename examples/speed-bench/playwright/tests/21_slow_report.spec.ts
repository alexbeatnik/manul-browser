import { test, expect } from 'playwright/test';
import { SlowPage } from '../pages/slow';

test('slow_report', async ({ page }) => {
  const s = new SlowPage(page);
  await test.step('Open the slow page', async () => {
    await s.open();
  });
  await test.step('The spinner goes', async () => {
    await expect(s.text('Loading page...')).toBeHidden();
    await expect(s.text('Feed item 2')).toBeVisible();
  });
  await test.step('Build the report', async () => {
    await s.build.click();
    await expect(s.text('Report ready: 3 orders')).toBeVisible();
  });
  await test.step('Build it again', async () => {
    await s.build.click();
    await expect(s.text('Report ready: 3 orders')).toBeVisible();
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
    await expect(s.text('Settings saved')).toBeHidden();
  });
});
