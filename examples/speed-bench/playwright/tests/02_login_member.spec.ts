import { test, expect } from 'playwright/test';
import { LoginPage } from '../pages/login';

test('login_member', async ({ page }) => {
  const login = new LoginPage(page);
  await test.step('Open the login page', async () => {
    await login.open();
  });
  await test.step('Sign in as grace', async () => {
    await login.signIn('grace', 'hopper');
    await expect(login.welcome('grace')).toBeVisible();
    await expect(login.role).toHaveValue('member');
  });
  await test.step('Sign out, and the fields are empty', async () => {
    await login.signOut();
    await expect(login.username).toHaveValue('');
  });
  await test.step('Sign in as ada', async () => {
    await login.signIn('ada', 'lovelace');
    await expect(login.welcome('ada')).toBeVisible();
    await expect(login.role).toHaveValue('admin');
  });
  await test.step('Sign out again', async () => {
    await login.signOut();
    await expect(login.welcome('ada')).toBeHidden();
  });
  await test.step('Ask to remember grace', async () => {
    await login.username.fill('grace');
    await login.remember.check();
    await expect(login.remember).toBeChecked();
  });
  await test.step('Sign in as grace once more', async () => {
    await login.password.fill('hopper');
    await login.signInButton.click();
    await expect(login.welcome('grace')).toBeVisible();
  });
});
