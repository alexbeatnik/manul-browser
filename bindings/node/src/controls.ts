/**
 * The handler registry: custom controls, CALL handlers and suite hooks.
 *
 * These describe *this process*, not any one session, so they are registered at
 * module scope and published to whichever engine turns up — the one a `Session`
 * spawns, or the one that spawned this process with `--hooks`. That is why the
 * registry is module-level state rather than a field on `Session`.
 *
 * Registration is by name, and matching is case- and whitespace-insensitive, so
 * `'Sign In'` and `'sign in'` are one target. It matches how the engine
 * normalises labels; a binding that disagreed would silently miss handlers.
 */

import type { Json } from './transport.js';

/** Page label that matches any page. */
export const ANY_PAGE = '*';

export const BEFORE_ALL = 'before_all';
export const AFTER_ALL = 'after_all';
export const BEFORE_GROUP = 'before_group';
export const AFTER_GROUP = 'after_group';

/** What a handler can reach while the engine is paused inside its step. */
export interface PagePeer {
  pageEval(js: string): Promise<Json>;
  pageUrl(): Promise<string>;
}

/** Context handed to a custom control. */
export interface ControlContext {
  readonly target: string;
  readonly action: string;
  readonly value: string;
  readonly page: string;
  readonly step: string;
  readonly vars: Record<string, string>;
  /** Evaluate JavaScript in the live page. */
  eval(js: string): Promise<Json>;
  /** The page's current URL. */
  url(): Promise<string>;
}

/** Context handed to a `CALL HOST` handler. */
export interface CallContext {
  readonly name: string;
  readonly args: string[];
  readonly vars: Record<string, string>;
  eval(js: string): Promise<Json>;
  url(): Promise<string>;
}

/**
 * Context handed to a suite hook.
 *
 * Anything written into `variables` is published to every hunt that follows as
 * a `{placeholder}`. `before_all` runs before a browser exists, so `eval` there
 * has nothing to talk to and rejects.
 */
export interface GlobalContext {
  readonly variables: Record<string, string>;
  /** Scratch space shared by the hooks of one slot. Never reaches a hunt. */
  readonly metadata: Record<string, unknown>;
  /** Publish a variable to every hunt in the suite; the value is stringified. */
  set(name: string, value: unknown): void;
  eval(js: string): Promise<Json>;
  url(): Promise<string>;
}

export type ControlHandler = (ctx: ControlContext) => void | Promise<void>;
export type CallHandler = (ctx: CallContext) => Json | Promise<Json>;
export type HookHandler = (ctx: GlobalContext) => void | Promise<void>;

interface ControlEntry {
  page: string;
  target: string;
  handler: ControlHandler;
}

const controls = new Map<string, ControlEntry>();
const calls = new Map<string, CallHandler>();
const hooks = new Map<string, HookHandler[]>();

function normalise(value: string): string {
  return value.trim().toLowerCase().split(/\s+/).join(' ');
}

function controlKey(page: string, target: string): string {
  return `${normalise(page)}\x00${normalise(target)}`;
}

function hookKey(kind: string, tag: string): string {
  return `${normalise(kind)}\x00${normalise(tag)}`;
}

// ── registration ────────────────────────────────────────────────────────────

/**
 * Handle a specific element yourself instead of letting the engine resolve it.
 *
 * The `.hunt` line stays an ordinary CLICK or FILL; this intercepts it before
 * DOM resolution, which is what makes a datepicker or a canvas widget
 * expressible in one readable step.
 */
export function customControl(target: string, handler: ControlHandler): void;
export function customControl(
  spec: { page?: string; target: string },
  handler: ControlHandler,
): void;
export function customControl(
  spec: string | { page?: string; target: string },
  handler: ControlHandler,
): void {
  const page = typeof spec === 'string' ? ANY_PAGE : (spec.page ?? ANY_PAGE);
  const target = typeof spec === 'string' ? spec : spec.target;
  if (!target.trim()) throw new TypeError('a custom control needs a target');
  if (typeof handler !== 'function') throw new TypeError('a custom control needs a handler');
  controls.set(controlKey(page, target), { page, target, handler });
}

/** Register a handler reachable from a hunt as `CALL HOST <name>`. */
export function call(name: string, handler: CallHandler): void {
  if (!name.trim()) throw new TypeError('a CALL handler needs a name');
  if (typeof handler !== 'function') throw new TypeError('a CALL handler needs a handler');
  calls.set(normalise(name), handler);
}

function addHook(kind: string, tag: string, handler: HookHandler): void {
  if (typeof handler !== 'function') throw new TypeError(`${kind} needs a handler`);
  const key = hookKey(kind, tag);
  const list = hooks.get(key);
  if (list) list.push(handler);
  else hooks.set(key, [handler]);
}

/** Runs once, before any hunt in the suite and before any browser exists. */
export function beforeAll(handler: HookHandler): void {
  addHook(BEFORE_ALL, '', handler);
}

/** Runs once after the suite, whatever happened. */
export function afterAll(handler: HookHandler): void {
  addHook(AFTER_ALL, '', handler);
}

/** Runs before each hunt carrying `tag` in its `@tags:` header. */
export function beforeGroup(tag: string, handler: HookHandler): void {
  if (!tag.trim()) throw new TypeError('before_group needs a tag');
  addHook(BEFORE_GROUP, tag, handler);
}

/** Runs after each hunt carrying `tag`. */
export function afterGroup(tag: string, handler: HookHandler): void {
  if (!tag.trim()) throw new TypeError('after_group needs a tag');
  addHook(AFTER_GROUP, tag, handler);
}

// ── lookup ──────────────────────────────────────────────────────────────────

export function getCustomControl(page: string, target: string): ControlHandler | null {
  return (
    controls.get(controlKey(page, target))?.handler ??
    controls.get(controlKey(ANY_PAGE, target))?.handler ??
    null
  );
}

export function getCall(name: string): CallHandler | null {
  return calls.get(normalise(name)) ?? null;
}

export function getHooks(kind: string, tag: string): HookHandler[] {
  return hooks.get(hookKey(kind, tag)) ?? [];
}

export function listCustomControls(): Array<{ page: string; target: string }> {
  return [...controls.values()].map(({ page, target }) => ({ page, target }));
}

export function listCalls(): string[] {
  return [...calls.keys()].sort();
}

export function listHooks(): Array<{ kind: string; tag: string }> {
  return [...hooks.keys()].map((key) => {
    const [kind = '', tag = ''] = key.split('\x00');
    return { kind, tag };
  });
}

/** Drop every registration. For tests. */
export function resetRegistry(): void {
  controls.clear();
  calls.clear();
  hooks.clear();
}

/**
 * Explain a custom-control miss in terms of what *is* registered.
 *
 * A wrong page label looks identical to a missing handler from the engine's
 * side, and that is the mistake people actually make.
 */
export function diagnoseCustomControlMiss(page: string, target: string): string | null {
  const siblings = [...controls.values()]
    .filter((c) => normalise(c.target) === normalise(target))
    .map((c) => c.page);
  if (siblings.length === 0) return null;
  return (
    `no custom control for ${JSON.stringify(target)} on page ${JSON.stringify(page)}, ` +
    `but one is registered for ${siblings.map((p) => JSON.stringify(p)).join(', ')}. ` +
    `Check the page label, or register with page: '*' to match any page.`
  );
}

/** What gets published to the engine as a `register` command. */
export function registrationPayload(): {
  controls: Array<{ page: string; target: string }>;
  calls: string[];
  hooks: Array<{ kind: string; tag: string }>;
} {
  return { controls: listCustomControls(), calls: listCalls(), hooks: listHooks() };
}

// ── dispatch ────────────────────────────────────────────────────────────────

/**
 * Route one engine callback to the handler that claimed it.
 *
 * `peer` is whatever owns the pipe the callback arrived on: a `Session` when
 * this process drives the engine, the hook host when the engine drives this
 * process. The callbacks are identical either way, so they are dispatched in
 * one place rather than once per direction.
 */
export async function dispatchInvoke(
  msg: Record<string, Json>,
  peer: PagePeer,
): Promise<Json> {
  const kind = msg['kind'];
  const vars = (msg['vars'] ?? {}) as Record<string, string>;
  const evalFn = (js: string) => peer.pageEval(js);
  const urlFn = () => peer.pageUrl();

  if (kind === 'custom_control') {
    const page = String(msg['page'] ?? '');
    const target = String(msg['target'] ?? '');
    const handler = getCustomControl(page, target);
    if (!handler) {
      throw new Error(
        diagnoseCustomControlMiss(page, target) ??
          `no custom control for ${JSON.stringify(target)} on page ${JSON.stringify(page)}`,
      );
    }
    await handler({
      target,
      action: String(msg['action'] ?? ''),
      value: String(msg['value'] ?? ''),
      page,
      step: String(msg['step'] ?? ''),
      vars: { ...vars },
      eval: evalFn,
      url: urlFn,
    });
    // A control's job is the side effect; only failure travels back.
    return null;
  }

  if (kind === 'hook') {
    const hookName = String(msg['hook'] ?? '');
    const tag = String(msg['tag'] ?? '');
    const handlers = getHooks(hookName, tag);
    if (handlers.length === 0) {
      throw new Error(
        `no ${JSON.stringify(hookName)} hook registered` + (tag ? ` for tag ${JSON.stringify(tag)}` : ''),
      );
    }
    // Every handler in a slot runs, and they share one context so a later hook
    // sees what an earlier one published.
    const variables: Record<string, string> = { ...vars };
    const ctx: GlobalContext = {
      variables,
      metadata: {},
      set(name: string, value: unknown) {
        variables[String(name)] = String(value);
      },
      eval: evalFn,
      url: urlFn,
    };
    for (const handler of handlers) await handler(ctx);
    // Only the variables travel back; anything else is scratch space that never
    // reaches a hunt.
    return ctx.variables;
  }

  if (kind === 'call') {
    const name = String(msg['name'] ?? '');
    const handler = getCall(name);
    if (!handler) throw new Error(`no CALL handler registered for ${JSON.stringify(name)}`);
    const result = await handler({
      name,
      args: [...((msg['args'] ?? []) as string[])],
      vars: { ...vars },
      eval: evalFn,
      url: urlFn,
    });
    return result ?? null;
  }

  throw new Error(`unknown callback kind ${JSON.stringify(kind)}`);
}
