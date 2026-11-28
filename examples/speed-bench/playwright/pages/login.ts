import { expect, type Page } from 'playwright/test';

export class LoginPage {
  get username() {
    return this.page.getByLabel('Username');
  }
  get password() {
    return this.page.getByLabel('Password');
  }
  get remember() {
    return this.page.getByLabel('Remember me');
  }
  get signInButton() {
    return this.page.getByRole('button', { name: 'Sign in' });
  }
  get logoutButton() {
    return this.page.getByRole('button', { name: 'Logout' });
  }
  get role() {
    return this.page.getByLabel('Role');
  }
  get error() {
    return this.page.getByText('Invalid username or password');
  }

  constructor(readonly page: Page) {}

  async open() {
    await this.page.goto('/login.html');
    await this.expectOpen();
  }

  async expectOpen() {
    await expect(this.page).toHaveURL(/\/login\.html$/);
    await expect(this.page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
  }

  async signIn(user: string, password: string) {
    await this.username.fill(user);
    await this.password.fill(password);
    await this.signInButton.click();
  }

  async signOut() {
    await this.logoutButton.click();
    await this.expectOpen();
  }

  welcome(user: string) {
    return this.page.getByText(`Welcome, ${user}`);
  }
}
