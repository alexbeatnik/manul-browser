import { expect, type Page } from 'playwright/test';

export class HomePage {
  constructor(readonly page: Page) {}

  async open() {
    await this.page.goto('/index.html');
    await this.expectOpen();
  }

  async expectOpen() {
    await expect(this.page).toHaveURL(/\/(index\.html)?$/);
    await expect(this.page.getByText('Welcome to the Manul quickstart site')).toBeVisible();
  }
}

// The header's Home link, on every page of the site.
export const home = (page: Page) => page.getByRole('link', { name: 'Home', exact: true });

// A link of the home page's list.
export const link = (page: Page, name: string) => page.getByRole('main').getByRole('link', { name, exact: true });
