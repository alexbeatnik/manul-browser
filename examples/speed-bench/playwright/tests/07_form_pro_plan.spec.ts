import { test, expect } from 'playwright/test';
import { SignupPage } from '../pages/signup';

test('form_pro_plan', async ({ page }) => {
  const form = new SignupPage(page);
  await test.step('Open the sign-up form', async () => {
    await form.open();
  });
  await test.step('Name and email', async () => {
    await form.fill('Grace Hopper', 'grace@example.com');
    await expect(form.email).toHaveValue('grace@example.com');
  });
  await test.step('Country', async () => {
    await form.country.selectOption('Japan');
    await expect(form.country).toHaveValue('Japan');
  });
  await test.step('The pro plan', async () => {
    await form.pro.check();
    await expect(form.pro).toBeChecked();
    await expect(form.free).not.toBeChecked();
  });
  await test.step('No newsletter', async () => {
    await form.newsletter.uncheck();
    await expect(form.newsletter).not.toBeChecked();
  });
  await test.step('Accept the terms', async () => {
    await form.terms.check();
    await expect(form.create).toBeEnabled();
  });
  await test.step('Create the account', async () => {
    await form.create.click();
    await expect(form.thanks('Thanks, Grace Hopper! Plan: pro, country: Japan, newsletter: no')).toBeVisible();
  });
});
