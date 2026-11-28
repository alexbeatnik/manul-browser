import { test, expect } from 'playwright/test';
import { ShopPage } from '../pages/shop';

test('shop_add_each', async ({ page }) => {
  const shop = new ShopPage(page);
  await test.step('Open the shop', async () => {
    await shop.open();
    await expect(shop.cart(0)).toBeVisible();
  });
  await test.step('A plush', async () => {
    await shop.add('Manul Plush');
    await expect(shop.cart(1)).toBeVisible();
    await expect(shop.total).toHaveValue('24.50');
  });
  await test.step('A map', async () => {
    await shop.add('Steppe Map');
    await expect(shop.cart(2)).toBeVisible();
    await expect(shop.total).toHaveValue('36.50');
  });
  await test.step('Boots', async () => {
    await shop.add('Snow Boots');
    await expect(shop.cart(3)).toBeVisible();
    await expect(shop.total).toHaveValue('126.49');
  });
  await test.step('A second plush', async () => {
    await shop.add('Manul Plush');
    await expect(shop.cart(4)).toBeVisible();
    await expect(shop.total).toHaveValue('150.99');
  });
  await test.step('Help from the header', async () => {
    await shop.helpHeader.click();
    await expect(shop.text('Help from the header')).toBeVisible();
  });
  await test.step('A second map', async () => {
    await shop.add('Steppe Map');
    await expect(shop.cart(5)).toBeVisible();
    await expect(shop.total).toHaveValue('162.99');
  });
});
