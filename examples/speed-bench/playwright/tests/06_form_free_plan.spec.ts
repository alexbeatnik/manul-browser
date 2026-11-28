import { test, expect } from 'playwright/test';
import { SignupPage } from '../pages/signup';

test('form_free_plan', async ({ page }) => {
  const form = new SignupPage(page);
  await test.step('Open the sign-up form', async () => {
    await form.open();
  });
  await test.step('Full name', async () => {
    await form.fullName.fill('Ada Lovelace');
    await expect(form.fullName).toHaveValue('Ada Lovelace');
  });
  await test.step('Email', async () => {
    await form.email.fill('ada@example.com');
    await expect(form.email).toHaveValue('ada@example.com');
  });
  await test.step('Phone, typed', async () => {
    await form.phone.pressSequentially('+380 44 000 0000');
    await expect(form.phone).toHaveValue('+380 44 000 0000');
  });
  await test.step('Country', async () => {
    await form.country.selectOption('Ukraine');
    await expect(form.country).toHaveValue('Ukraine');
  });
  await test.step('Accept the terms', async () => {
    await form.terms.check();
    await expect(form.create).toBeEnabled();
  });
  await test.step('Create the account', async () => {
    await form.create.click();
    await expect(form.thanks('Thanks, Ada Lovelace! Plan: free, country: Ukraine, newsletter: yes')).toBeVisible();
  });
});
