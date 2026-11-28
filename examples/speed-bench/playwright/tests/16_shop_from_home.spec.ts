import { test, expect } from 'playwright/test';
import { HomePage, home } from '../pages/home';
import { ShopPage } from '../pages/shop';

test('shop_from_home', async ({ page }) => {
  const shop = new ShopPage(page);
  const headerShop = page.locator('header').getByRole('link', { name: 'Shop' });
  await test.step('Open home', async () => {
    await new HomePage(page).open();
  });
  await test.step('The shop from the header', async () => {
    await headerShop.click();
    await shop.expectOpen();
  });
  await test.step('A map', async () => {
    await shop.addButton('Steppe Map').click();
    await expect(shop.cart(1)).toBeVisible();
  });
  await test.step('Boots', async () => {
    await shop.addButton('Snow Boots').click();
    await expect(shop.cart(2)).toBeVisible();
    await expect(shop.total).toHaveValue('101.99');
  });
  await test.step('Back home', async () => {
    await home(page).click();
    await new HomePage(page).expectOpen();
  });
  await test.step('The shop again, empty', async () => {
    await headerShop.click();
    await expect(shop.cart(0)).toBeVisible();
    await expect(shop.total).toHaveValue('0.00');
  });
  await test.step('A plush', async () => {
    await shop.addButton('Manul Plush').click();
    await expect(shop.cart(1)).toBeVisible();
    await expect(shop.total).toHaveValue('24.50');
  });
});
