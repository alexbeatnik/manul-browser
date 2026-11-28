import { test, expect } from 'playwright/test';
import { HomePage, home } from '../pages/home';
import { LoginPage } from '../pages/login';

test('login_unknown_user', async ({ page }) => {
  const login = new LoginPage(page);
  await test.step('Open the login page', async () => {
    await login.open();
  });
  await test.step('A name that is not an account', async () => {
    await login.username.fill('alan');
    await login.password.fill('turing');
    await login.signInButton.click();
    await expect(login.error).toBeVisible();
  });
  await test.step('Another one', async () => {
    await login.username.fill('manul');
    await login.signInButton.click();
    await expect(login.error).toBeVisible();
    await expect(page.getByText('Welcome')).toBeHidden();
  });
  await test.step("Another account's password", async () => {
    await login.username.fill('grace');
    await login.password.fill('lovelace');
    await login.signInButton.click();
    await expect(login.error).toBeVisible();
  });
  await test.step('Her own password', async () => {
    await login.password.fill('hopper');
    await login.signInButton.click();
    await expect(login.welcome('grace')).toBeVisible();
    await expect(login.role).toHaveValue('member');
  });
  await test.step('Sign out', async () => {
    await login.logoutButton.click();
    await login.expectOpen();
  });
  await test.step('Go home', async () => {
    await home(page).click();
    await new HomePage(page).expectOpen();
  });
});
