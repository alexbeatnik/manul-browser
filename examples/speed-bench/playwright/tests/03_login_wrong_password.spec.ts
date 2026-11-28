import { test, expect } from 'playwright/test';
import { LoginPage } from '../pages/login';

test('login_wrong_password', async ({ page }) => {
  const login = new LoginPage(page);
  await test.step('Open the login page', async () => {
    await login.open();
  });
  await test.step('A wrong password', async () => {
    await login.username.fill('ada');
    await login.password.fill('wrong');
    await login.signInButton.click();
    await expect(login.error).toBeVisible();
  });
  await test.step('The right password in the wrong case', async () => {
    await login.password.fill('LOVELACE');
    await login.signInButton.click();
    await expect(login.error).toBeVisible();
    await expect(login.welcome('ada')).toBeHidden();
  });
  await test.step('The name in the wrong case', async () => {
    await login.username.fill('Ada');
    await login.password.fill('lovelace');
    await login.signInButton.click();
    await expect(login.error).toBeVisible();
  });
  await test.step('Both right', async () => {
    await login.username.fill('ada');
    await login.signInButton.click();
    await expect(login.welcome('ada')).toBeVisible();
    await expect(login.error).toBeHidden();
  });
  await test.step('Sign out', async () => {
    await login.logoutButton.click();
    await login.expectOpen();
  });
  await test.step('The error does not come back', async () => {
    await login.username.fill('ada');
    await expect(login.username).toHaveValue('ada');
    await expect(login.error).toBeHidden();
  });
});
