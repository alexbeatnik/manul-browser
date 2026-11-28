import { test, expect } from 'playwright/test';
import { MousePage } from '../pages/mouse';

test('mouse_hover_copy', async ({ page }) => {
  const m = new MousePage(page);
  await test.step('Open the page', async () => {
    await m.open();
  });
  await test.step('Hover the avatar', async () => {
    await m.avatar.hover();
    await expect(m.text('Signed in as Manul')).toBeVisible();
  });
  await test.step('Copy the link', async () => {
    await m.copyLink.dblclick();
    await expect(m.text('Link copied')).toBeVisible();
  });
  await test.step('Search', async () => {
    await m.search.fill('plush');
    await m.search.press('Enter');
    await expect(m.text('Results for plush')).toBeVisible();
  });
  await test.step('Hover again', async () => {
    await m.avatar.hover();
    await expect(m.text('Signed in as Manul')).toBeVisible();
  });
  await test.step('Open the dialog', async () => {
    await m.openDialog.click();
    await expect(m.text('Dialog is open')).toBeVisible();
  });
  await test.step('Close it with Escape', async () => {
    await page.keyboard.press('Escape');
    await expect(m.text('Dialog is open')).toBeHidden();
  });
});
