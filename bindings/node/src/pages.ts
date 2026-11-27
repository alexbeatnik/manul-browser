// Page objects over a Session.
//
// Manul needs no selectors, so a page object is not a bag of locators. It is a
// page's name and a vocabulary: methods that say what a person does there, each
// a few plain-English lines, or one of the actions the page's own page file
// declares (pages/login.hunt: `@page: Login`, `STEP SignIn:`). The engine
// still resolves every element, and it — not this class — decides whether the
// page is open: `VERIFY that the page is 'Login'` asks for the page's name in
// pages/, its title, and what its page file @expects.
//
//   class LoginPage extends Page {
//     static pageName = 'Login';
//     signIn(user: string, password: string) {
//       return this.use('SignIn', { user, password });
//     }
//   }
//   await new LoginPage(session).open().then((p) => p.signIn('ada', 'lovelace'));

import type { RunOutcome, Session, StepOutcome } from './session.js';

/** A step a page object ran did not pass. Carries the engine's outcome. */
export class StepFailed extends Error {
  readonly outcome: StepOutcome | RunOutcome;

  constructor(outcome: StepOutcome | RunOutcome, step = '') {
    const what = step || ('step' in outcome ? outcome.step : '') || 'the steps';
    const reason = ('reason' in outcome && outcome.reason) || 'failed';
    const error = 'error' in outcome && outcome.error ? ` — ${outcome.error}` : '';
    const near =
      'near' in outcome
        ? outcome.near
            .slice(0, 3)
            .map((c) => String(c['label'] ?? c['text'] ?? ''))
            .filter(Boolean)
            .join(', ')
        : '';
    super(`${JSON.stringify(what)} failed: ${reason}${error}${near ? ` (closest: ${near})` : ''}`);
    this.name = 'StepFailed';
    this.outcome = outcome;
  }
}

/** The part of a Session a page object uses. */
export type PageSession = Pick<Session, 'step' | 'run' | 'setVars' | 'read'>;

/**
 * A page of the app under test. `pageName` is the page's name in pages/ — a
 * page file's `@page`, or a name a JSON fragment gives; `path` is where the
 * page is under `baseUrl`, for a page pages/ has no address for.
 */
export class Page {
  /** The page's name in pages/. */
  static pageName = '';
  /** Where the page is under baseUrl, when pages/ does not know. */
  static path = '';

  readonly s: PageSession;
  readonly baseUrl: string;

  constructor(session: PageSession, baseUrl = '') {
    this.s = session;
    this.baseUrl = baseUrl.replace(/\/$/, '');
  }

  /** The page's name in pages/. */
  get pageName(): string {
    const name = (this.constructor as typeof Page).pageName;
    if (!name) {
      throw new Error(`${this.constructor.name} has no pageName: set it to the page's name in pages/`);
    }
    return name;
  }

  /**
   * Run DSL lines in order; a failed step throws StepFailed. The session
   * returns a failure rather than throwing — for a test it is fatal, and a
   * page object is where that is decided.
   */
  async do(...steps: string[]): Promise<this> {
    for (const step of steps) {
      const outcome = await this.s.step(step);
      if (!outcome.ok) throw new StepFailed(outcome, step);
    }
    return this;
  }

  /**
   * Go to the page — by its address under baseUrl when `path` is set, by its
   * name in pages/ otherwise — and wait until it is open.
   */
  async open(): Promise<this> {
    const path = (this.constructor as typeof Page).path;
    if (path && this.baseUrl) await this.do(`NAVIGATE to ${this.baseUrl}${path}`);
    else await this.do(`NAVIGATE to '${this.pageName}'`);
    return this.assertLoaded();
  }

  /**
   * Wait, as long as a step waits, until the engine says this page is open:
   * its name in pages/ or its title, and every text its page file @expects.
   */
  assertLoaded(): Promise<this> {
    return this.do(`VERIFY that the page is '${this.pageName}'`);
  }

  /** Run one of the page file's actions — `USE <name>.<block>` — with the given variables set first. */
  async use(block: string, variables: Record<string, string | number> = {}): Promise<this> {
    const vars = Object.fromEntries(Object.entries(variables).map(([k, v]) => [k, String(v)]));
    if (Object.keys(vars).length > 0) await this.s.setVars(vars);
    const step = `USE ${this.pageName}.${block}`;
    const outcome = await this.s.run({ source: step });
    if (!outcome.ok) throw new StepFailed(outcome, step);
    return this;
  }

  shouldSee(...texts: string[]): Promise<this> {
    return this.do(...texts.map((t) => `VERIFY that '${t}' is present`));
  }

  shouldNotSee(...texts: string[]): Promise<this> {
    return this.do(...texts.map((t) => `VERIFY that '${t}' is NOT present`));
  }

  /** Read one labelled value without a full page scan. */
  async valueOf(label: string): Promise<string> {
    const v = await this.s.read(label);
    if (!v.found) throw new Error(`${JSON.stringify(label)} is not on ${this.pageName}: ${v.reason}`);
    return v.value;
  }
}
