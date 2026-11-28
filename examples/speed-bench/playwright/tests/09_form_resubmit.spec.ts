import { test, expect } from 'playwright/test';
import { SignupPage } from '../pages/signup';

test('form_resubmit', async ({ page }) => {
  const form = new SignupPage(page);
  await test.step('Open the sign-up form', async () => {
    await form.open();
  });
  await test.step('A short name', async () => {
    await form.fullName.fill('Ada');
    await expect(form.fullName).toHaveValue('Ada');
  });
  await test.step('Send it', async () => {
    await form.terms.check();
    await form.create.click();
    await expect(form.thanks('Thanks, Ada! Plan: free, country: Choose…, newsletter: yes')).toBeVisible();
  });
  await test.step('The full name and a country', async () => {
    await form.fullName.fill('Ada Lovelace');
    await form.country.selectOption('Canada');
    await expect(form.country).toHaveValue('Canada');
  });
  await test.step('Send it again', async () => {
    await form.create.click();
    await expect(form.thanks('Thanks, Ada Lovelace! Plan: free, country: Canada, newsletter: yes')).toBeVisible();
  });
  await test.step('Change the plan', async () => {
    await form.pro.check();
    await form.create.click();
    await expect(form.thanks('Thanks, Ada Lovelace! Plan: pro, country: Canada, newsletter: yes')).toBeVisible();
  });
  await test.step('Drop the newsletter', async () => {
    await form.newsletter.uncheck();
    await form.create.click();
    await expect(form.thanks('Thanks, Ada Lovelace! Plan: pro, country: Canada, newsletter: no')).toBeVisible();
  });
});
