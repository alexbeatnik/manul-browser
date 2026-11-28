import { test, expect } from 'playwright/test';
import { SignupPage } from '../pages/signup';

test('form_terms_gate', async ({ page }) => {
  const form = new SignupPage(page);
  await test.step('Open the sign-up form', async () => {
    await page.goto('/form.html');
    await expect(form.create).toBeDisabled();
  });
  await test.step('A name alone does not unlock it', async () => {
    await form.fullName.fill('Alan Turing');
    await expect(form.create).toBeDisabled();
  });
  await test.step('Accept the terms', async () => {
    await form.terms.check();
    await expect(form.create).toBeEnabled();
  });
  await test.step('Take them back', async () => {
    await form.terms.uncheck();
    await expect(form.create).toBeDisabled();
  });
  await test.step('Accept them again', async () => {
    await form.terms.check();
    await expect(form.terms).toBeChecked();
  });
  await test.step('Create the account', async () => {
    await form.create.click();
    await expect(form.thanks('Thanks, Alan Turing! Plan: free, country: Choose…, newsletter: yes')).toBeVisible();
  });
  await test.step('Take them back once more', async () => {
    await form.terms.uncheck();
    await expect(form.create).toBeDisabled();
    await expect(form.thanks('Thanks, Alan Turing!')).toBeVisible();
  });
});
