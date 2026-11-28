import { test, expect } from 'playwright/test';
import { MousePage } from '../pages/mouse';

test('mouse_everything', async ({ page }) => {
  const m = new MousePage(page);
  await test.step('Open the page', async () => {
    await page.goto('/mouse.html');
    await expect(page.getByRole('heading', { name: 'Board', exact: true })).toBeVisible();
  });
  await test.step('Hover', async () => {
    await m.avatar.hover();
    await expect(m.text('Signed in as Manul')).toBeVisible();
  });
  await test.step('Double click', async () => {
    await m.copyLink.dblclick();
    await expect(m.text('Link copied')).toBeVisible();
  });
  await test.step('Right click', async () => {
    await m.fileArea.click({ button: 'right' });
    await m.rename.click();
    await expect(m.text('Renamed')).toBeVisible();
  });
  await test.step('Drag', async () => {
    await m.tile('Fix the build').dragTo(m.done);
    await expect(m.text('Fix the build moved to Done')).toBeVisible();
  });
  await test.step('A dialog', async () => {
    await m.openDialog.click();
    await expect(m.text('Dialog is open')).toBeVisible();
  });
  await test.step('A confirm', async () => {
    await page.keyboard.press('Escape');
    page.once('dialog', (d) => d.accept());
    await m.deleteAccount.click();
    await expect(m.text('Account deleted')).toBeVisible();
    await expect(m.text('Dialog is open')).toBeHidden();
  });
});
