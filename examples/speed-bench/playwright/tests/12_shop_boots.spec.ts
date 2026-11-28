import { test, expect } from 'playwright/test';
import { ShopPage } from '../pages/shop';

test('shop_boots', async ({ page }) => {
  const shop = new ShopPage(page);
  await test.step('Open the shop', async () => {
    await shop.open();
  });
  await test.step('Boots', async () => {
    await shop.addButton('Snow Boots').click();
    await expect(shop.total).toHaveValue('89.99');
  });
  await test.step('Boots again', async () => {
    await shop.addButton('Snow Boots').click();
    await expect(shop.total).toHaveValue('179.98');
  });
  await test.step('A plush', async () => {
    await shop.addButton('Manul Plush').click();
    await expect(shop.total).toHaveValue('204.48');
  });
  await test.step('A map', async () => {
    await shop.addButton('Steppe Map').click();
    await expect(shop.total).toHaveValue('216.48');
    await expect(shop.cart(4)).toBeVisible();
  });
  await test.step('Edit a team member', async () => {
    await shop.teamButton('Ada Lovelace', 'Edit').click();
    await expect(shop.text('Editing Ada Lovelace')).toBeVisible();
  });
  await test.step('Boots a third time', async () => {
    await shop.addButton('Snow Boots').click();
    await expect(shop.cart(5)).toBeVisible();
    await expect(shop.total).toHaveValue('306.47');
  });
});
