import { test, expect } from 'playwright/test';
import { SignupPage } from '../pages/signup';

test('form_countries', async ({ page }) => {
  const form = new SignupPage(page);
  await test.step('Open the sign-up form', async () => {
    await form.open();
  });
  await test.step('Name and terms', async () => {
    await form.fullName.fill('Kent Beck');
    await form.terms.check();
    await expect(form.create).toBeEnabled();
  });
  for (const country of ['Ukraine', 'Japan', 'Canada']) {
    await test.step(country, async () => {
      await form.country.selectOption(country);
      await form.create.click();
      await expect(form.thanks(`Thanks, Kent Beck! Plan: free, country: ${country}, newsletter: yes`)).toBeVisible();
    });
  }
  await test.step('Pro, without the newsletter', async () => {
    await form.pro.check();
    await form.newsletter.uncheck();
    await form.create.click();
    await expect(form.thanks('Thanks, Kent Beck! Plan: pro, country: Canada, newsletter: no')).toBeVisible();
  });
  await test.step('Back to free', async () => {
    await form.free.check();
    await form.create.click();
    await expect(form.thanks('Thanks, Kent Beck! Plan: free, country: Canada, newsletter: no')).toBeVisible();
  });
});
