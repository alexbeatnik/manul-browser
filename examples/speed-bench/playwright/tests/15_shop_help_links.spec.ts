import { test, expect } from 'playwright/test';
import { ShopPage } from '../pages/shop';

test('shop_help_links', async ({ page }) => {
  const shop = new ShopPage(page);
  await test.step('Open the shop', async () => {
    await shop.open();
  });
  await test.step('Help from the header', async () => {
    await shop.helpHeader.click();
    await expect(shop.text('Help from the header')).toBeVisible();
  });
  await test.step('Help from the footer', async () => {
    await shop.helpFooter.click();
    await expect(shop.text('Help from the footer')).toBeVisible();
    await expect(shop.text('Help from the header')).toBeHidden();
  });
  await test.step('A product', async () => {
    await shop.addButton('Manul Plush').click();
    await expect(shop.text('Added Manul Plush')).toBeVisible();
  });
  await test.step('Help replaces it', async () => {
    await shop.helpHeader.click();
    await expect(shop.text('Help from the header')).toBeVisible();
    await expect(shop.text('Added Manul Plush')).toBeHidden();
  });
  await test.step('The footer once more', async () => {
    await shop.helpFooter.click();
    await expect(shop.text('Help from the footer')).toBeVisible();
  });
  await test.step('An edit replaces that', async () => {
    await shop.teamButton('Alan Turing', 'Edit').click();
    await expect(shop.text('Editing Alan Turing')).toBeVisible();
    await expect(shop.text('Help from the footer')).toBeHidden();
  });
});
