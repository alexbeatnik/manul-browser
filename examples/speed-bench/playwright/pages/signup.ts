import { expect, type Page } from 'playwright/test';

export class SignupPage {
  get fullName() {
    return this.page.getByLabel('Full name');
  }
  get email() {
    return this.page.getByLabel('Email');
  }
  get phone() {
    return this.page.getByLabel('Phone');
  }
  get country() {
    return this.page.getByLabel('Country');
  }
  get free() {
    return this.page.getByLabel('Free', { exact: true });
  }
  get pro() {
    return this.page.getByLabel('Pro', { exact: true });
  }
  get newsletter() {
    return this.page.getByLabel('Newsletter');
  }
  get terms() {
    return this.page.getByLabel('I accept the Terms');
  }
  get create() {
    return this.page.getByRole('button', { name: 'Create account' });
  }

  constructor(readonly page: Page) {}

  async open() {
    await this.page.goto('/form.html');
    await this.expectOpen();
  }

  async expectOpen() {
    await expect(this.page).toHaveURL(/\/form\.html$/);
    await expect(this.page.getByRole('heading', { name: 'Create an account' })).toBeVisible();
  }

  async fill(name: string, email: string) {
    await this.fullName.fill(name);
    await this.email.fill(email);
  }

  async submit() {
    await this.terms.check();
    await this.create.click();
  }

  thanks(text: string) {
    return this.page.getByText(text);
  }
}
