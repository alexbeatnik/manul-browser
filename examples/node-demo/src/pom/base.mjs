// Page objects over a manul Session.
//
// Manul needs no selectors, so a page object here is not a bag of locators. It
// is a vocabulary: methods that name what a user does on that page, each a few
// plain-English DSL lines. The engine still resolves every element.

/** A step returned ok: false. Carries the engine's reason and nearby candidates. */
export class StepFailed extends Error {
  constructor(outcome) {
    const near = outcome.near.slice(0, 3).map((c) => c.text).join(', ');
    super(
      `${JSON.stringify(outcome.step)} failed: ${outcome.reason}` +
        (outcome.error ? ` — ${outcome.error}` : '') +
        (near ? ` (nearby: ${near})` : ''),
    );
    this.name = 'StepFailed';
    this.outcome = outcome;
  }
}

export class BasePage {
  /** Path under the site root. */
  static path = '/';
  /** document.title — also the page label custom controls are registered under. */
  static title = '';

  constructor(session, baseUrl) {
    this.s = session;
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  get title() {
    return this.constructor.title;
  }

  /**
   * Run DSL lines in order; a failed step throws StepFailed.
   *
   * The binding deliberately returns failures instead of throwing; a page
   * object is the right place to decide that, for a test, a miss is fatal.
   */
  async do(...steps) {
    for (const step of steps) {
      const outcome = await this.s.step(step);
      if (!outcome.ok) throw new StepFailed(outcome);
    }
    return this;
  }

  async open() {
    await this.do(`NAVIGATE to ${this.baseUrl}${this.constructor.path}`);
    return this.assertLoaded();
  }

  async assertLoaded() {
    const state = await this.s.state();
    if (this.title && state.title !== this.title) {
      throw new Error(`expected page ${JSON.stringify(this.title)}, on ${JSON.stringify(state.title)} (${state.url})`);
    }
    return this;
  }

  shouldSee(...texts) {
    return this.do(...texts.map((t) => `VERIFY that '${t}' is present`));
  }

  shouldNotSee(...texts) {
    return this.do(...texts.map((t) => `VERIFY that '${t}' is NOT present`));
  }

  /** Read one labelled value without a full page scan. */
  async valueOf(label) {
    const v = await this.s.read(label);
    if (!v.found) throw new Error(`${JSON.stringify(label)} not on ${this.title}: ${v.reason}`);
    return v.value;
  }
}
