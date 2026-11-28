import { test, expect } from 'playwright/test';
import { ShopPage } from '../pages/shop';

test('shop_team_edit', async ({ page }) => {
  const shop = new ShopPage(page);
  await test.step('Open the shop', async () => {
    await page.goto('/shop.html');
    await expect(page.getByRole('heading', { name: 'Team' })).toBeVisible();
  });
  await test.step('Edit Ada', async () => {
    await shop.teamButton('Ada Lovelace', 'Edit').click();
    await expect(shop.text('Editing Ada Lovelace')).toBeVisible();
  });
  await test.step('Edit Grace', async () => {
    await shop.teamButton('Grace Hopper', 'Edit').click();
    await expect(shop.text('Editing Grace Hopper')).toBeVisible();
  });
  await test.step('Edit Alan', async () => {
    await shop.teamButton('Alan Turing', 'Edit').click();
    await expect(shop.text('Editing Alan Turing')).toBeVisible();
  });
  await test.step('Edit Ada again', async () => {
    await shop.teamButton('Ada Lovelace', 'Edit').click();
    await expect(shop.text('Editing Ada Lovelace')).toBeVisible();
    await expect(shop.text('Editing Alan Turing')).toBeHidden();
  });
  await test.step('A product in between', async () => {
    await shop.addButton('Manul Plush').click();
    await expect(shop.text('Added Manul Plush')).toBeVisible();
  });
  await test.step('Edit Grace again', async () => {
    await shop.teamButton('Grace Hopper', 'Edit').click();
    await expect(shop.text('Editing Grace Hopper')).toBeVisible();
    await expect(shop.cart(1)).toBeVisible();
  });
});
