import { test, expect } from 'playwright/test';
import { ShopPage } from '../pages/shop';

test('shop_team_remove', async ({ page }) => {
  const shop = new ShopPage(page);
  await test.step('Open the shop', async () => {
    await page.goto('/shop.html');
    await expect(shop.text('Alan Turing')).toBeVisible();
  });
  await test.step('Remove Alan', async () => {
    await shop.teamButton('Alan Turing', 'Remove').click();
    await expect(shop.text('Removed Alan Turing')).toBeVisible();
  });
  await test.step('He is gone from the table', async () => {
    await shop.helpFooter.click();
    await expect(shop.text('Help from the footer')).toBeVisible();
    await expect(shop.text('Alan Turing')).toBeHidden();
  });
  await test.step('Remove Grace', async () => {
    await shop.teamButton('Grace Hopper', 'Remove').click();
    await expect(shop.text('Removed Grace Hopper')).toBeVisible();
  });
  await test.step('She is gone too', async () => {
    await shop.teamButton('Ada Lovelace', 'Edit').click();
    await expect(shop.text('Editing Ada Lovelace')).toBeVisible();
    await expect(shop.text('Grace Hopper')).toBeHidden();
  });
  await test.step('The shop still works', async () => {
    await shop.addButton('Steppe Map').click();
    await expect(shop.text('Added Steppe Map')).toBeVisible();
  });
  await test.step('Remove Ada', async () => {
    await shop.teamButton('Ada Lovelace', 'Remove').click();
    await expect(shop.text('Removed Ada Lovelace')).toBeVisible();
  });
  await test.step('The table is empty', async () => {
    await shop.helpHeader.click();
    await expect(shop.text('Help from the header')).toBeVisible();
    await expect(shop.text('Ada Lovelace')).toBeHidden();
  });
});
