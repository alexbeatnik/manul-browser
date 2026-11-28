import { test, expect } from 'playwright/test';
import { LoginPage } from '../pages/login';

test('login_switch_users', async ({ page }) => {
  const login = new LoginPage(page);
  await test.step('Open the login page', async () => {
    await login.open();
  });
  await test.step('Ada signs in', async () => {
    await login.signIn('ada', 'lovelace');
    await expect(login.welcome('ada')).toBeVisible();
    await expect(login.role).toHaveValue('admin');
  });
  await test.step('Ada signs out', async () => {
    await login.signOut();
    await expect(login.welcome('ada')).toBeHidden();
  });
  await test.step('Grace signs in', async () => {
    await login.signIn('grace', 'hopper');
    await expect(login.welcome('grace')).toBeVisible();
    await expect(login.role).toHaveValue('member');
  });
  await test.step('Grace signs out', async () => {
    await login.signOut();
    await expect(login.welcome('grace')).toBeHidden();
  });
  await test.step('Ada signs in again', async () => {
    await login.signIn('ada', 'lovelace');
    await expect(login.welcome('ada')).toBeVisible();
  });
  await test.step('And out', async () => {
    await login.signOut();
    await expect(login.password).toHaveValue('');
  });
});
