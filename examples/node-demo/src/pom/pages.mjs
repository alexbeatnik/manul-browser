import { BasePage } from './base.mjs';

export class LoginPage extends BasePage {
  static path = '/index.html';
  static title = 'Login Page';

  async loginAs(username, password, { remember = false } = {}) {
    await this.do(`FILL 'Username' field with '${username}'`, `FILL 'Password' field with '${password}'`);
    if (remember) await this.do("CHECK the checkbox for 'Remember me'");
    await this.do("CLICK the 'Sign in' button");
    return new DashboardPage(this.s, this.baseUrl).assertLoaded();
  }

  async loginExpectingError(username, password) {
    await this.do(
      `FILL 'Username' field with '${username}'`,
      `FILL 'Password' field with '${password}'`,
      "CLICK the 'Sign in' button",
    );
    return this.shouldSee('Invalid username or password');
  }
}

export class DashboardPage extends BasePage {
  static path = '/dashboard.html';
  static title = 'Dashboard';

  async orderTotal() {
    return Number(await this.valueOf('Order total'));
  }

  role() {
    return this.valueOf('Role');
  }

  async editUser(name) {
    await this.do(`CLICK the 'Edit' button INSIDE 'Users' row with '${name}'`);
    return this.shouldSee(`Editing ${name}`);
  }

  async deleteUser(name) {
    // confirm() is accepted by the engine, so this really deletes.
    await this.do(`CLICK the 'Delete' button INSIDE 'Users' row with '${name}'`);
    return this.shouldSee(`Deleted ${name}`);
  }

  /** WHILE as a one-liner: a hunt snippet run through session.run({ source }). */
  async loadWholeFeed() {
    await this.s.run({
      source: "STEP 1: feed\n    WHILE button 'Load More' exists:\n        CLICK the 'Load More' button\n",
    });
    return ((await this.s.readText('#feed')).match(/Feed item/g) ?? []).length;
  }

  async logout() {
    await this.do("CLICK the 'Logout' button ON HEADER");
    return new LoginPage(this.s, this.baseUrl).assertLoaded();
  }
}

export class FormsPage extends BasePage {
  static path = '/forms.html';
  static title = 'Forms Page';

  fillProfile({ name, email, phone, bio }) {
    return this.do(
      `FILL 'Full name' field with '${name}'`,
      `FILL 'Email' field with '${email}'`,
      `TYPE '${phone}' into the 'Phone' field`,
      `FILL 'Bio' field with '${bio}'`,
    );
  }

  async choose({ country, plan, newsletter }) {
    await this.do(`SELECT '${country}' from the 'Country' dropdown`, `CLICK the radio button for '${plan}'`);
    return this.do(`${newsletter ? 'CHECK' : 'UNCHECK'} the checkbox for 'Newsletter'`);
  }

  // Handled by the Delivery Date custom control in src/controls.mjs.
  setDelivery(date) {
    return this.do(`FILL 'Delivery Date' field with '${date}'`);
  }

  async uploadAvatar(path) {
    await this.do(`UPLOAD '${path}' to 'Avatar'`);
    return this.shouldSee(`Uploaded: ${path.split(/[\\/]/).pop()}`);
  }

  async submit() {
    await this.do(
      "VERIFY that 'Submit' is disabled",
      "CHECK the checkbox for 'I accept the Terms'",
      "VERIFY that 'Submit' is enabled",
      "CLICK the 'Submit' button",
    );
    await this.shouldSee('Form submitted');
    // readText: the visible text of a CSS region, for the non-labelled bits.
    return JSON.parse(await this.s.readText('#summary'));
  }
}

export class WidgetsPage extends BasePage {
  static path = '/widgets.html';
  static title = 'Widgets Page';

  waitReady() {
    return this.do("WAIT FOR 'Loading widgets...' to disappear");
  }

  async moveCardToDone() {
    await this.do("DRAG 'Card A' and drop it into 'Done column'");
    return this.shouldSee('Card A moved to Done');
  }

  async sign(signer) {
    // The custom control reads {signer} from the session's variables.
    await this.s.setVars({ signer });
    await this.do("CLICK the 'Signature Pad'");
    return this.shouldSee(`Signed by ${signer}`);
  }

  async rate(stars) {
    await this.do(`SELECT '${stars}' from the 'Star Rating' dropdown`);
    return this.shouldSee(`Rating: ${stars}`);
  }

  async typeInShadow(text) {
    await this.do(`TYPE '${text}' into the 'Shadow Field' field`);
    return this.shouldSee(`Echo: ${text}`);
  }
}

export class CatalogPage extends BasePage {
  static path = '/catalog.html';
  static title = 'Catalog';

  waitLoaded(count) {
    return this.shouldSee(`Loaded ${count} products`);
  }

  async addToCart(product, qty = 1) {
    for (let i = 0; i < qty; i++) await this.do(`CLICK the 'Add to cart' button NEAR '${product}'`);
    return this;
  }

  async cartTotal() {
    return Number(await this.valueOf('Cart total'));
  }

  async checkout() {
    await this.do("CLICK the 'Checkout' button", "VERIFY that 'placed' is present");
    return this.s.readText('#order-status');
  }

  /** MOCK holds for the rest of the session. The path is relative to the
   *  engine's working directory, because a single step has no .hunt file. */
  useMock(mockFile) {
    return this.do(`MOCK GET "/api/products" with '${mockFile}'`, "CLICK the 'Refresh catalog' button");
  }
}
