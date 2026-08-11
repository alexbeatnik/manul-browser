/**
 * The JavaScript face of the Manul engine.
 *
 * `Session` mirrors `pkg/agent.Session` in Go and `manul.Session` in Python,
 * method for method, so the three describe the same thing the same way.
 * Everything here is a thin call over the protocol — no scoring, no DSL
 * parsing, no CDP.
 */

import * as controls from './controls.js';
import type { PagePeer } from './controls.js';
import { Transport, type Json, type TransportOptions } from './transport.js';

export interface MapElement {
  label: string;
  role: string;
  editable: boolean;
}

export interface MapGroup {
  name: string;
  elements: MapElement[];
  truncated: number;
}

/** A landmark-grouped view of the page, budgeted for an LLM's context. */
export interface PageMap {
  url: string;
  groups: MapGroup[];
  /** Every element label on the page, flattened — handy for a quick look. */
  labels(): string[];
}

/**
 * The result of reading one labelled thing off the page.
 *
 * `found: false` is a normal answer, not an error: the label simply is not
 * there right now.
 */
export interface Value {
  value: string;
  found: boolean;
  reason: string;
}

/** What happened when one DSL line ran. */
export interface StepOutcome {
  ok: boolean;
  step: string;
  action: string;
  value: string;
  url: string;
  reason: string;
  error: string;
  score: number;
  near: Array<Record<string, Json>>;
}

/** The aggregate of running a whole .hunt script. */
export interface RunOutcome {
  ok: boolean;
  url: string;
  totalSteps: number;
  passed: number;
  failed: number;
}

/** One hunt's outcome inside a suite. */
export interface SuiteHunt {
  path: string;
  ok: boolean;
  /** True when a before_group hook refused this hunt. The suite carried on. */
  skipped: boolean;
  tags: string[];
  steps: number;
  passed: number;
  failed: number;
  error: string;
}

/** The aggregate of a suite run. */
export interface SuiteResult {
  ok: boolean;
  total: number;
  passed: number;
  failed: number;
  skipped: number;
  hunts: SuiteHunt[];
}

export interface SessionOptions extends TransportOptions {
  /** `'launch'` starts a Chrome this session owns; `'attach'` joins one. */
  mode?: 'launch' | 'attach';
  /** CDP endpoint to dial when attaching. */
  cdp?: string;
  /** Attach to the first tab whose URL contains this substring. */
  tab?: string;
  headless?: boolean;
  port?: number;
  executablePath?: string;
}

function num(v: Json, fallback = 0): number {
  const n = Number(v);
  return Number.isFinite(n) ? n : fallback;
}

function str(v: Json, fallback = ''): string {
  return v === undefined || v === null ? fallback : String(v);
}

/**
 * A live engine and the browser it owns.
 *
 * Create one with `launch` or `attach` rather than calling the constructor:
 * opening is asynchronous, and a half-open session is not a useful object.
 */
export class Session implements PagePeer {
  readonly #t: Transport;
  #opened = false;

  /** Which mode the engine actually resolved: `launch` or `attach`. */
  mode = '';
  /** The CDP endpoint in use, when attaching. */
  cdp = '';
  /** What `register` reported when handlers were published. */
  published: { controls: number; calls: number; hooks: number } = {
    controls: 0,
    calls: 0,
    hooks: 0,
  };

  private constructor(options: SessionOptions) {
    this.#t = new Transport(options);
    this.#t.onInvoke = (msg) => controls.dispatchInvoke(msg as Record<string, Json>, this);
  }

  /**
   * Start an engine and open a browser session.
   *
   * Registered handlers are published before the browser exists: they describe
   * this process, and a registration at import time must not depend on a
   * session having been created yet.
   */
  static async open(options: SessionOptions = {}): Promise<Session> {
    const s = new Session(options);
    try {
      await s.#t.ready();
      await s.publishHandlers();
      const res = ((await s.#t.call('open', {
        mode: options.mode,
        cdp: options.cdp,
        tab: options.tab,
        headless: options.headless,
        port: options.port,
        executablePath: options.executablePath,
      })) ?? {}) as Record<string, Json>;
      s.#opened = true;
      s.mode = str(res['mode']);
      s.cdp = str(res['cdp']);
      return s;
    } catch (err) {
      await s.#t.close();
      throw err;
    }
  }

  /** Start a Chrome this session owns. */
  static launch(options: Omit<SessionOptions, 'mode'> = {}): Promise<Session> {
    return Session.open({ ...options, mode: 'launch' });
  }

  /**
   * Join a Chrome that is already running.
   *
   * That browser is left open when the session ends — Manul did not open it.
   */
  static attach(cdp: string, options: Omit<SessionOptions, 'mode' | 'cdp'> = {}): Promise<Session> {
    return Session.open({ ...options, mode: 'attach', cdp });
  }

  /**
   * Tell the engine which custom controls, CALL handlers and hooks exist here.
   *
   * Called automatically by `open`. Call it again after registering more
   * handlers on a session that is already running.
   */
  async publishHandlers(): Promise<{ controls: number; calls: number; hooks: number }> {
    const payload = controls.registrationPayload();
    const empty =
      payload.controls.length === 0 && payload.calls.length === 0 && payload.hooks.length === 0;
    if (empty) {
      this.published = { controls: 0, calls: 0, hooks: 0 };
      return this.published;
    }
    const res = ((await this.#t.call('register', payload as unknown as Record<string, Json>)) ??
      {}) as Record<string, Json>;
    this.published = {
      controls: num(res['controls']),
      calls: num(res['calls']),
      hooks: num(res['hooks']),
    };
    return this.published;
  }

  /** End the session and stop the engine. Safe to call twice. */
  async close(): Promise<void> {
    await this.#t.close();
    this.#opened = false;
  }

  /** Enables `await using session = await Session.launch()`. */
  async [Symbol.asyncDispose](): Promise<void> {
    await this.close();
  }

  get closed(): boolean {
    return this.#t.closed;
  }

  get opened(): boolean {
    return this.#opened;
  }

  /**
   * The live protocol connection.
   *
   * An escape hatch, and deliberately a supported one: the engine gains
   * commands faster than this package wraps them, and `session.transport.call`
   * is a better answer than a fork. Nothing here interprets what it returns.
   */
  get transport(): Transport {
    return this.#t;
  }

  get engineVersion(): string {
    return this.#t.engineVersion;
  }

  get protocol(): string {
    return this.#t.protocol;
  }

  // ── page commands ─────────────────────────────────────────────────────────

  /**
   * Run one DSL line.
   *
   * A step that resolves nothing is a normal answer with `ok: false`, not a
   * thrown error — the session stays usable either way.
   */
  async step(instruction: string): Promise<StepOutcome> {
    const raw = ((await this.#t.call('run-step', { step: instruction })) ?? {}) as Record<
      string,
      Json
    >;
    return {
      ok: Boolean(raw['ok']),
      step: str(raw['step']),
      action: str(raw['action']),
      value: str(raw['value']),
      url: str(raw['url']),
      reason: str(raw['reason']),
      error: str(raw['error']),
      score: num(raw['score']),
      near: (raw['near'] ?? []) as Array<Record<string, Json>>,
    };
  }

  /** Run a whole .hunt script, given either its text or a path to it. */
  async run(source: { source: string } | { path: string }): Promise<RunOutcome> {
    const args = 'source' in source ? { source: source.source } : { path: source.path };
    const raw = ((await this.#t.call('run', args)) ?? {}) as Record<string, Json>;
    return {
      ok: Boolean(raw['ok']),
      url: str(raw['url']),
      totalSteps: num(raw['total_steps']),
      passed: num(raw['passed']),
      failed: num(raw['failed']),
    };
  }

  /**
   * Run several .hunt files with the suite lifecycle applied.
   *
   * This is not a loop over `run`: `beforeAll`/`afterAll` bracket the whole
   * set, and the group hooks fire per hunt according to its tags.
   */
  async runSuite(paths: readonly string[]): Promise<SuiteResult> {
    const raw = ((await this.#t.call('run-suite', { paths: [...paths] })) ?? {}) as Record<
      string,
      Json
    >;
    const hunts = ((raw['hunts'] ?? []) as Array<Record<string, Json>>).map((h) => ({
      path: str(h['path']),
      ok: Boolean(h['ok']),
      skipped: Boolean(h['skipped']),
      tags: (h['tags'] ?? []) as string[],
      steps: num(h['steps']),
      passed: num(h['passed']),
      failed: num(h['failed']),
      error: str(h['error']),
    }));
    return {
      ok: Boolean(raw['ok']),
      total: num(raw['total']),
      passed: num(raw['passed']),
      failed: num(raw['failed']),
      skipped: num(raw['skipped']),
      hunts,
    };
  }

  /** A landmark-grouped view of what is on the page. */
  async map(budget: { maxPerGroup?: number; includeUnlabeled?: boolean } = {}): Promise<PageMap> {
    const raw = ((await this.#t.call('map', {
      maxPerGroup: budget.maxPerGroup,
      includeUnlabeled: budget.includeUnlabeled,
    })) ?? {}) as Record<string, Json>;
    const groups = ((raw['groups'] ?? []) as Array<Record<string, Json>>).map((g) => ({
      name: str(g['name']),
      truncated: num(g['truncated']),
      elements: ((g['elements'] ?? []) as Array<Record<string, Json>>).map((e) => ({
        label: str(e['label']),
        role: str(e['role']),
        editable: Boolean(e['editable']),
      })),
    }));
    return {
      url: str(raw['url']),
      groups,
      labels(): string[] {
        return groups.flatMap((g) => g.elements.map((e) => e.label));
      },
    };
  }

  /** Read one labelled value off the page. */
  async read(label: string, options: { maxChars?: number } = {}): Promise<Value> {
    const raw = ((await this.#t.call('read', { label, maxChars: options.maxChars })) ??
      {}) as Record<string, Json>;
    return {
      value: str(raw['value']),
      found: Boolean(raw['found']),
      reason: str(raw['reason']),
    };
  }

  /** Read the text of a CSS selector. */
  async readText(selector: string, options: { maxChars?: number } = {}): Promise<string> {
    const raw = ((await this.#t.call('read', { selector, maxChars: options.maxChars })) ??
      {}) as Record<string, Json>;
    return str(raw['text']);
  }

  /** URL and title of the current page. */
  async state(): Promise<Record<string, string>> {
    return ((await this.#t.call('state')) ?? {}) as Record<string, string>;
  }

  /** Read DSL variables. With no names, every variable. */
  async vars(...names: string[]): Promise<Record<string, string>> {
    return ((await this.#t.call('vars', { get: names.length ? names : undefined })) ??
      {}) as Record<string, string>;
  }

  /** Set DSL variables, and get the full set back. */
  async setVars(values: Record<string, string>): Promise<Record<string, string>> {
    return ((await this.#t.call('vars', { set: values })) ?? {}) as Record<string, string>;
  }

  /** The engine's own description of its DSL and JSON shapes. */
  async schema(): Promise<Record<string, Json>> {
    return ((await this.#t.call('schema')) ?? {}) as Record<string, Json>;
  }

  // ── page primitives ───────────────────────────────────────────────────────
  //
  // Valid only while a handler is running. They exist so a JavaScript handler
  // can inspect and touch the page the way an embedded Go handler does with its
  // browser.Page.

  async pageEval(js: string): Promise<Json> {
    return this.#t.nestedCall('page.eval', { js });
  }

  async pageUrl(): Promise<string> {
    return str(await this.#t.nestedCall('page.url'));
  }
}
