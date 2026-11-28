import { test, expect } from 'playwright/test';
import { MousePage } from '../pages/mouse';

test('keyboard_search', async ({ page }) => {
  const m = new MousePage(page);
  await test.step('Open the page', async () => {
    await m.open();
  });
  await test.step('Search for manul', async () => {
    await m.search.fill('manul');
    await m.search.press('Enter');
    await expect(m.text('Results for manul')).toBeVisible();
  });
  await test.step('Search for steppe', async () => {
    await m.search.fill('steppe');
    await m.search.press('Enter');
    await expect(m.text('Results for steppe')).toBeVisible();
    await expect(m.text('Results for manul')).toBeHidden();
  });
  await test.step('Open the dialog', async () => {
    await m.openDialog.click();
    await expect(m.text('Dialog is open')).toBeVisible();
  });
  await test.step('Close it', async () => {
    await page.keyboard.press('Escape');
    await expect(m.text('Dialog is open')).toBeHidden();
  });
  await test.step('Open it again', async () => {
    await m.openDialog.click();
    await expect(m.text('Dialog is open')).toBeVisible();
  });
  await test.step('Close it again', async () => {
    await page.keyboard.press('Escape');
    await expect(m.text('Dialog is open')).toBeHidden();
  });
  await test.step('One more search', async () => {
    await m.search.fill('snow');
    await m.search.press('Enter');
    await expect(m.text('Results for snow')).toBeVisible();
  });
});
