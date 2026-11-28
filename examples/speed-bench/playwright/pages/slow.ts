import { expect, type Page } from 'playwright/test';

export class SlowPage {
  get build() {
    return this.page.getByRole('button', { name: 'Build report' });
  }
  get loadMore() {
    return this.page.getByRole('button', { name: 'Load more' });
  }
  get save() {
    return this.page.getByRole('button', { name: 'Save settings' });
  }

  constructor(readonly page: Page) {}

  async open() {
    await this.page.goto('/dynamic.html');
    await this.expectOpen();
  }

  async expectOpen() {
    await expect(this.page).toHaveURL(/\/dynamic\.html$/);
    await expect(this.page.getByRole('heading', { name: 'Report' })).toBeVisible();
  }

  text(text: string) {
    return this.page.getByText(text);
  }
}
