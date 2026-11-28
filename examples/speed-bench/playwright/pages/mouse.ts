import { expect, type Page } from 'playwright/test';

export class MousePage {
  get avatar() {
    return this.page.getByLabel('Avatar');
  }
  get copyLink() {
    return this.page.getByRole('button', { name: 'Copy link' });
  }
  get fileArea() {
    return this.page.getByLabel('File area');
  }
  get rename() {
    return this.page.getByRole('button', { name: 'Rename' });
  }
  get done() {
    return this.page.getByLabel('Done');
  }
  get search() {
    return this.page.getByLabel('Search');
  }
  get openDialog() {
    return this.page.getByRole('button', { name: 'Open dialog' });
  }
  get deleteAccount() {
    return this.page.getByRole('button', { name: 'Delete account' });
  }

  constructor(readonly page: Page) {}

  async open() {
    await this.page.goto('/mouse.html');
    await this.expectOpen();
  }

  async expectOpen() {
    await expect(this.page).toHaveURL(/\/mouse\.html$/);
    await expect(this.page.getByRole('heading', { name: 'Board', exact: true })).toBeVisible();
  }

  tile(name: string) {
    return this.page.getByText(name, { exact: true });
  }

  text(text: string) {
    return this.page.getByText(text);
  }
}
