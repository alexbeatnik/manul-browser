import { expect, type Page } from 'playwright/test';

export class QuotesPage {
  get next() {
    return this.page.getByRole('button', { name: 'Next quote' });
  }

  constructor(readonly page: Page) {}

  async open() {
    await this.page.goto('/quotes.html');
    await this.expectOpen();
  }

  async expectOpen() {
    await expect(this.page).toHaveURL(/\/quotes\.html$/);
    await expect(this.page.getByRole('heading', { name: 'Quote of the day' })).toBeVisible();
  }

  text(text: string) {
    return this.page.getByText(text);
  }
}
