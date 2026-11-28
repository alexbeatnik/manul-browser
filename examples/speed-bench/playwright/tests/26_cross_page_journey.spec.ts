import { test, expect } from 'playwright/test';
import { HomePage, home, link } from '../pages/home';
import { LoginPage } from '../pages/login';
import { MousePage } from '../pages/mouse';
import { ShopPage } from '../pages/shop';
import { SignupPage } from '../pages/signup';

test('cross_page_journey', async ({ page }) => {
  await test.step('Open home', async () => {
    await new HomePage(page).open();
  });
  await test.step('Sign in', async () => {
    await link(page, 'Login page').click();
    const login = new LoginPage(page);
    await login.signIn('ada', 'lovelace');
    await expect(login.welcome('ada')).toBeVisible();
  });
  await test.step('To the shop', async () => {
    await home(page).click();
    await page.locator('header').getByRole('link', { name: 'Shop' }).click();
    await new ShopPage(page).expectOpen();
  });
  await test.step('Buy a plush', async () => {
    const shop = new ShopPage(page);
    await shop.add('Manul Plush');
    await expect(shop.cart(1)).toBeVisible();
  });
  await test.step('Sign up', async () => {
    await home(page).click();
    await link(page, 'Sign-up form').click();
    const form = new SignupPage(page);
    await form.fill('Ada Lovelace', 'ada@example.com');
    await form.submit();
    await expect(form.thanks('Thanks, Ada Lovelace! Plan: free, country: Choose…, newsletter: yes')).toBeVisible();
  });
  await test.step('Hover the avatar', async () => {
    await home(page).click();
    await link(page, 'Mouse and keyboard').click();
    const m = new MousePage(page);
    await m.avatar.hover();
    await expect(m.text('Signed in as Manul')).toBeVisible();
  });
  await test.step('Read a quote', async () => {
    await home(page).click();
    await link(page, 'Quote of the day').click();
    await expect(page.getByText('Simple things should be simple.')).toBeVisible();
  });
});
