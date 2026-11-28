import { test, expect } from 'playwright/test';
import { HomePage, home } from '../pages/home';
import { LoginPage } from '../pages/login';

test('login_admin', async ({ page }) => {
  const login = new LoginPage(page);
  await test.step('Open the login page', async () => {
    await login.open();
  });
  await test.step('Type the username', async () => {
    await login.username.fill('ada');
    await expect(login.username).toHaveValue('ada');
  });
  await test.step('Type the password', async () => {
    await login.password.fill('lovelace');
    await expect(login.password).toHaveValue('lovelace');
  });
  await test.step('Ask to be remembered', async () => {
    await login.remember.check();
    await expect(login.remember).toBeChecked();
  });
  await test.step('Sign in', async () => {
    await login.signInButton.click();
    await expect(login.welcome('ada')).toBeVisible();
    await expect(login.role).toHaveValue('admin');
  });
  await test.step('Sign out', async () => {
    await login.logoutButton.click();
    await login.expectOpen();
    await expect(login.welcome('ada')).toBeHidden();
  });
  await test.step('Go home', async () => {
    await home(page).click();
    await new HomePage(page).expectOpen();
  });
});
