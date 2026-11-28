import { test, expect } from 'playwright/test';
import { MousePage } from '../pages/mouse';

test('mouse_menu_and_board', async ({ page }) => {
  const m = new MousePage(page);
  await test.step('Open the page', async () => {
    await m.open();
  });
  await test.step('Rename from the context menu', async () => {
    await m.fileArea.click({ button: 'right' });
    await m.rename.click();
    await expect(m.text('Renamed')).toBeVisible();
  });
  await test.step('Drag the first tile', async () => {
    await m.tile('Write tests').dragTo(m.done);
    await expect(m.text('Write tests moved to Done')).toBeVisible();
  });
  await test.step('Drag the second tile', async () => {
    await m.tile('Fix the build').dragTo(m.done);
    await expect(m.text('Fix the build moved to Done')).toBeVisible();
  });
  await test.step('Copy the link', async () => {
    await m.copyLink.dblclick();
    await expect(m.text('Link copied')).toBeVisible();
  });
  await test.step('Search', async () => {
    await m.search.fill('boots');
    await m.search.press('Enter');
    await expect(m.text('Results for boots')).toBeVisible();
  });
  await test.step('Delete the account', async () => {
    page.once('dialog', (d) => d.accept());
    await m.deleteAccount.click();
    await expect(m.text('Account deleted')).toBeVisible();
  });
});
