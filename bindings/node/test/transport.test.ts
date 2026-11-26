/**
 * The wire itself: framing, the awkward lines an engine can write, and how the
 * process is brought down. `session.test.ts` covers what the commands mean;
 * this covers what happens when the pipe misbehaves.
 */

import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { after, describe, it } from 'node:test';

import { EngineError, ProtocolError, SessionClosed, Transport } from '../src/index.js';

const FAKE = fileURLToPath(new URL('./fakeEngine.js', import.meta.url));

function fake(mode = 'normal'): Transport {
  return new Transport({
    binary: [process.execPath, FAKE],
    env: { ...process.env, MANUL_FAKE_MODE: mode },
    stderr: 'ignore',
  });
}

async function readyFake(mode = 'normal'): Promise<Transport> {
  const t = fake(mode);
  after(() => t.close(500));
  await t.ready();
  return t;
}

describe('construction', () => {
  it('refuses an empty command array', () => {
    assert.throws(() => new Transport({ binary: [] }), TypeError);
  });

  it('tolerates a BOM ahead of the ready line', async () => {
    const t = await readyFake('bom');
    assert.equal(t.protocol, '1.0');
    assert.equal(t.engineVersion, '0.1.0');
  });

  it('reports a binary that cannot be started', async () => {
    const t = new Transport({ binary: ['/nonexistent/manul-engine'], stderr: 'ignore' });
    await assert.rejects(() => t.ready(), ProtocolError);
  });
});

describe('requests', () => {
  it('drops unset arguments, and the args key with them', async () => {
    const t = await readyFake();

    await t.call('state', { a: undefined, b: null });
    assert.deepEqual(await t.call('last-request'), { id: 1, cmd: 'state' });

    await t.call('state', { keep: 'x', drop: undefined, zero: 0, no: false, empty: '' });
    const req = (await t.call('last-request')) as Record<string, unknown>;
    // Only null and undefined mean "unset"; falsy values are real values.
    assert.deepEqual(req['args'], { keep: 'x', zero: 0, no: false, empty: '' });
  });

  it('numbers requests so replies can be matched', async () => {
    const t = await readyFake();
    await t.call('state');
    await t.call('state');
    const req = (await t.call('last-request')) as Record<string, unknown>;
    assert.equal(req['id'], 2);
  });

  it('steps over replies and events nobody asked for', async () => {
    const t = await readyFake();
    assert.equal(await t.call('stray'), 'after the strays');
    // And the conversation is still in step afterwards.
    assert.deepEqual(await t.call('state'), { url: 'https://example.com/', title: 'Example' });
  });

  it('carries the error code and message of a rejected request', async () => {
    const t = await readyFake();
    await assert.rejects(
      () => t.call('no-such-command'),
      (err: unknown) => {
        assert.ok(err instanceof EngineError);
        assert.equal(err.code, 'bad_request');
        assert.match(err.message, /^bad_request: unknown cmd/);
        return true;
      },
    );
  });
});

describe('a misbehaving engine', () => {
  it('fails the pending request on a line that is not JSON', async () => {
    const t = await readyFake('garbage');
    await assert.rejects(() => t.call('state'), (err: unknown) => {
      assert.ok(err instanceof ProtocolError);
      assert.match(err.message, /unreadable line from the engine: this is not json/);
      return true;
    });
  });

  it('fails the pending request when the engine exits instead of answering', async () => {
    const t = await readyFake('die');
    await assert.rejects(() => t.call('state'), (err: unknown) => {
      assert.ok(err instanceof ProtocolError);
      assert.match(err.message, /exited \(code 5\)/);
      return true;
    });
  });

  it('answers a reverse call with an error when nothing handles it', async () => {
    // A bare Transport has no onInvoke. Leaving the invoke unanswered would
    // hang the engine inside its step; answering it fails the step instead.
    const t = await readyFake();
    assert.equal(t.onInvoke, null);
    await assert.rejects(
      () => t.call('call-host', { name: 'anything', args: [] }),
      /no handler is registered/,
    );
    // The engine is not stuck: the next command goes through.
    assert.deepEqual(await t.call('vars'), { greeting: 'hello' });
  });

  it('passes what onInvoke returns back to the engine', async () => {
    const t = await readyFake();
    t.onInvoke = async (msg) => `handled ${msg.kind}`;
    assert.equal(await t.call('call-host', { name: 'x', args: [] }), 'handled call');
  });

  it('sends null for a handler that returns nothing', async () => {
    const t = await readyFake();
    t.onInvoke = () => undefined;
    assert.equal(await t.call('call-host', { name: 'x', args: [] }), null);
  });
});

describe('close', () => {
  it('kills an engine that ignores close', async () => {
    const t = fake('stubborn');
    await t.ready();
    const started = Date.now();
    await t.close(200);
    assert.equal(t.closed, true);
    // Bounded by the deadline, not by the engine's patience.
    assert.ok(Date.now() - started < 5000);
  });

  it('refuses calls once closed', async () => {
    const t = fake();
    await t.ready();
    await t.close();
    await assert.rejects(() => t.call('state'), SessionClosed);
  });

  it('is safe on an engine that already died', async () => {
    const t = fake('die');
    await t.ready();
    await assert.rejects(() => t.call('state'));
    await t.close(200);
    assert.equal(t.closed, true);
  });
});
