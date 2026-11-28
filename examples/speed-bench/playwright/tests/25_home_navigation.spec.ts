import { test } from 'playwright/test';
import { HomePage, home, link } from '../pages/home';
import { LoginPage } from '../pages/login';
import { MousePage } from '../pages/mouse';
import { QuotesPage } from '../pages/quotes';
import { SignupPage } from '../pages/signup';
import { SlowPage } from '../pages/slow';

test('home_navigation', async ({ page }) => {
  const homePage = new HomePage(page);
  await test.step('Open home', async () => {
    await homePage.open();
  });
  await test.step('The login page', async () => {
    await link(page, 'Login page').click();
    await new LoginPage(page).expectOpen();
  });
  await test.step('Back home', async () => {
    await home(page).click();
    await homePage.expectOpen();
  });
  await test.step('The sign-up form', async () => {
    await link(page, 'Sign-up form').click();
    await new SignupPage(page).expectOpen();
  });
  await test.step('The slow page', async () => {
    await home(page).click();
    await link(page, 'Slow page').click();
    await new SlowPage(page).expectOpen();
  });
  await test.step('Mouse and keyboard', async () => {
    await home(page).click();
    await link(page, 'Mouse and keyboard').click();
    await new MousePage(page).expectOpen();
  });
  await test.step('The quote of the day', async () => {
    await home(page).click();
    await link(page, 'Quote of the day').click();
    await new QuotesPage(page).expectOpen();
  });
  await test.step('Home once more', async () => {
    await home(page).click();
    await homePage.expectOpen();
  });
});
