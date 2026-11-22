/**
 * `serveHooks`: the one place where the engine drives this process.
 *
 * Each test spawns `hookScript.ts` and plays the engine against it, line by
 * line, the way `manul run --hooks` does. That is the whole contract: what the
 * script writes, in what order, and what it does with what it is sent.
 */

import assert from 'node:assert/strict';
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { createInterface } from 'node:readline';
import { fileURLToPath } from 'node:url';
import { after, describe, it } from 'node:test';

const SCRIPT = fileURLToPath(new URL('./hookScript.js', import.meta.url));

type Msg = Record<string, unknown>;

/** The engine's half of the conversation. */
class Engine {
  readonly proc: ChildProcessWithoutNullStreams;
  readonly #lines: AsyncIterableIterator<string>;
  stderr = '';
  readonly exited: Promise<number | null>;

  constructor(mode: string) {
    this.proc = spawn(process.execPath, [SCRIPT], {
      env: { ...process.env, HOOK_MODE: mode },
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    this.proc.stderr.setEncoding('utf8');
    this.proc.stderr.on('data', (chunk: string) => {
      this.stderr += chunk;
    });
    this.#lines = createInterface({ input: this.proc.stdout })[Symbol.asyncIterator]();
    this.exited = new Promise((resolve) => this.proc.once('close', (code) => resolve(code)));
    after(() => {
      this.proc.kill();
    });
  }

  /** The next line the script wrote, parsed. Any non-JSON line fails the test. */
  async next(): Promise<Msg> {
    const { value, done } = await this.#lines.next();
    assert.ok(!done, `script closed stdout; stderr:\n${this.stderr}`);
    return JSON.parse(String(value)) as Msg;
  }

  send(msg: Msg, prefix = ''): void {
    this.proc.stdin.write(prefix + JSON.stringify(msg) + '\n');
  }

  /** Accept the script's handshake: optional `register`, then `ready`. */
  async handshake(): Promise<Msg | null> {
    let registered: Msg | null = null;
    let msg = await this.next();
    if (msg['cmd'] === 'register') {
      registered = msg;
      this.send({ id: msg['id'], ok: true, result: { controls: 1, calls: 3, hooks: 2 } });
      msg = await this.next();
    }
    assert.equal(msg['cmd'], 'ready');
    this.send({ id: msg['id'], ok: true, result: {} });
    return registered;
  }

  /** Send one invocation and return the script's answer to it. */
  async invoke(n: number, body: Msg): Promise<Msg> {
    this.send({ invoke: n, ...body });
    const reply = await this.next();
    assert.equal(reply['invoke'], n);
    return reply;
  }
}

describe('serveHooks handshake', () => {
  it('publishes its handlers, then says ready', async () => {
    const e = new Engine('full');
    const reg = await e.handshake();
    assert.ok(reg, 'expected a register command');
    assert.equal(reg['id'], 1);
    const args = reg['args'] as Msg;
    assert.deepEqual(args['controls'], [{ page: 'Checkout', target: 'Datepicker' }]);
    assert.deepEqual(args['calls'], ['bigint', 'boom', 'echo']);
    assert.deepEqual(args['hooks'], [
      { kind: 'before_all', tag: '' },
      { kind: 'before_group', tag: 'smoke' },
    ]);
  });

  it('skips register when it has nothing to publish', async () => {
    const e = new Engine('empty');
    const first = await e.next();
    assert.equal(first['cmd'], 'ready');
    assert.equal(first['id'], 1);
    assert.equal('args' in first, false);
  });

  it('exits cleanly when the engine closes its input', async () => {
    const e = new Engine('empty');
    await e.handshake();
    e.proc.stdin.end();
    assert.equal(await e.exited, 0);
  });

  it('dies loudly if the engine rejects the registration', async () => {
    const e = new Engine('full');
    const reg = await e.next();
    e.send({ id: reg['id'], ok: false, error: { code: 'bad_request', message: 'no thanks' } });
    assert.notEqual(await e.exited, 0);
    assert.match(e.stderr, /register: no thanks/);
  });
});

describe('serveHooks invocations', () => {
  it('runs a hook and returns what it published', async () => {
    const e = new Engine('full');
    await e.handshake();
    const reply = await e.invoke(1, {
      kind: 'hook',
      hook: 'before_all',
      tag: '',
      vars: { existing: 'kept' },
    });
    assert.equal(reply['ok'], true);
    assert.deepEqual(reply['result'], { existing: 'kept', token: 'seeded' });
  });

  it('sends console output to stderr, never into the protocol', async () => {
    const e = new Engine('full');
    await e.handshake();
    // If either line had gone to stdout, next() inside invoke() would have
    // tried to parse it and failed.
    await e.invoke(1, { kind: 'hook', hook: 'before_all', tag: '', vars: {} });
    assert.match(e.stderr, /log from a hook/);
    assert.match(e.stderr, /info from a hook/);
  });

  it('lets a hook reach the page while the engine waits', async () => {
    const e = new Engine('full');
    await e.handshake();
    e.send({ invoke: 1, kind: 'hook', hook: 'before_group', tag: 'smoke', vars: {} });

    const evalReq = await e.next();
    assert.equal(evalReq['cmd'], 'page.eval');
    assert.deepEqual(evalReq['args'], { js: 'document.title' });
    e.send({ id: evalReq['id'], ok: true, result: 'Shop' });

    const urlReq = await e.next();
    assert.equal(urlReq['cmd'], 'page.url');
    assert.equal('args' in urlReq, false);
    e.send({ id: urlReq['id'], ok: true, result: 'https://shop.test/' });

    const reply = await e.next();
    assert.equal(reply['invoke'], 1);
    assert.deepEqual(reply['result'], { title: 'Shop', url: 'https://shop.test/' });
  });

  it('reports a page call the engine refused as a failed hook', async () => {
    const e = new Engine('full');
    await e.handshake();
    e.send({ invoke: 1, kind: 'hook', hook: 'before_group', tag: 'smoke', vars: {} });
    const evalReq = await e.next();
    e.send({ id: evalReq['id'], ok: false, error: { code: 'not_open', message: 'no browser' } });
    const reply = await e.next();
    assert.equal(reply['ok'], false);
    assert.match(String((reply['error'] as Msg)['message']), /page\.eval: no browser/);
  });

  it('answers a throwing handler with handler_failed, and keeps serving', async () => {
    const e = new Engine('full');
    await e.handshake();
    const failed = await e.invoke(1, { kind: 'call', name: 'boom', args: [], vars: {} });
    assert.equal(failed['ok'], false);
    assert.deepEqual(failed['error'], {
      code: 'handler_failed',
      message: 'RangeError: deliberate',
    });

    const next = await e.invoke(2, { kind: 'call', name: 'echo', args: ['a', 'b'], vars: {} });
    assert.deepEqual(next, { invoke: 2, ok: true, result: 'a,b' });
  });

  it('survives a result that has no JSON form', async () => {
    const e = new Engine('full');
    await e.handshake();
    const failed = await e.invoke(1, { kind: 'call', name: 'bigint', args: [], vars: {} });
    assert.equal(failed['ok'], false);
    assert.equal((failed['error'] as Msg)['code'], 'handler_failed');
    // Still alive: the next invocation is answered.
    const next = await e.invoke(2, { kind: 'call', name: 'echo', args: ['x'], vars: {} });
    assert.equal(next['result'], 'x');
  });

  it('explains a custom control registered for another page', async () => {
    const e = new Engine('full');
    await e.handshake();
    const reply = await e.invoke(1, {
      kind: 'custom_control',
      page: 'Login',
      target: 'Datepicker',
      action: 'click',
      vars: {},
    });
    assert.equal(reply['ok'], false);
    assert.match(String((reply['error'] as Msg)['message']), /registered for "Checkout"/);
  });

  it('accepts a line with a BOM in front', async () => {
    const e = new Engine('full');
    await e.handshake();
    e.send({ invoke: 1, kind: 'call', name: 'echo', args: ['bom'], vars: {} }, '﻿');
    const reply = await e.next();
    assert.equal(reply['result'], 'bom');
  });

  it('refuses anything that is not an invocation', async () => {
    const e = new Engine('empty');
    await e.handshake();
    e.send({ id: 42, ok: true, result: 'unexpected' });
    assert.notEqual(await e.exited, 0);
    assert.match(e.stderr, /expected an invocation/);
  });
});
