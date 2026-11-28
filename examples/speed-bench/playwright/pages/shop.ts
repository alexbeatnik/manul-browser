import { expect, type Page } from 'playwright/test';

export class ShopPage {
  get total() {
    return this.page.getByLabel('Cart total');
  }
  get helpHeader() {
    return this.page.locator('header').getByRole('link', { name: 'Help' });
  }
  get helpFooter() {
    return this.page.locator('footer').getByRole('link', { name: 'Help' });
  }

  constructor(readonly page: Page) {}

  async open() {
    await this.page.goto('/shop.html');
    await this.expectOpen();
  }

  async expectOpen() {
    await expect(this.page).toHaveURL(/\/shop\.html$/);
    await expect(this.page.getByText('Cart total')).toBeVisible();
  }

  addButton(product: string) {
    return this.page.locator('.card').filter({ hasText: product }).getByRole('button', { name: 'Add to cart' });
  }

  async add(product: string) {
    await this.addButton(product).click();
    await expect(this.page.getByText(`Added ${product}`)).toBeVisible();
  }

  cart(n: number) {
    return this.page.getByText(`Cart: ${n}`, { exact: true });
  }

  teamButton(person: string, button: 'Edit' | 'Remove') {
    return this.page.getByRole('row', { name: person }).getByRole('button', { name: button });
  }

  text(text: string) {
    return this.page.getByText(text);
  }
}
