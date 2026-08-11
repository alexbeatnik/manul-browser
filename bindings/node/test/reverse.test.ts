/**
 * Reverse calls: the engine asking this process to run something.
 *
 * These go through the fake engine rather than calling the dispatcher directly,
 * because the thing worth testing is the whole exchange — the invoke line, the
 * handler, the reply, and the nested page primitives available while the engine
 * is paused.
 */

import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { after, beforeEach, describe, it } from 'node:test';

import { Session, beforeAll, call, customControl, resetRegistry } from '../src/index.js';
import type { Transport } from '../src/index.js';

const FAKE = fileURLToPath(new URL('./fakeEngine.js', import.meta.url));

function openFake(): Promise<Session> {
  return Session.launch({ binary: [process.execPath, FAKE], stderr: 'ignore' });
}

/** The fake's test-only commands, reached through the documented escape hatch. */
function raw(s: Session): Transport {
  return s.transport;
}

beforeEach(() => resetRegistry());

describe('publishing', () => {
  it('tells the engine what this process owns', async () => {
    customControl('Cookie Consent', () => {});
    call('py.upper', (ctx) => ctx.args[0]?.toUpperCase() ?? '');
    beforeAll(() => {});

    const s = await openFake();
    after(() => s.close());
    assert.deepEqual(s.published, { controls: 1, calls: 1, hooks: 1 });
  });

  it('publishes nothing when nothing is registered', async () => {
    const s = await openFake();
    after(() => s.close());
    assert.deepEqual(s.published, { controls: 0, calls: 0, hooks: 0 });
  });
});

describe('custom control', () => {
  it('runs the handler when a step reaches it', async () => {
    let sawAction = '';
    customControl('Cookie Consent', (ctx) => {
      sawAction = ctx.action;
    });

    const s = await openFake();
    after(() => s.close());
    const out = await s.step("CLICK the 'Cookie Consent' banner");
    assert.equal(out.ok, true);
    assert.equal(sawAction, 'click');
  });

  it('can look at the page while the engine is paused', async () => {
    let evaluated: unknown = null;
    let seenUrl = '';
    customControl('Cookie Consent', async (ctx) => {
      evaluated = await ctx.eval('document.title');
      seenUrl = await ctx.url();
    });

    const s = await openFake();
    after(() => s.close());
    await s.step("CLICK the 'Cookie Consent' banner");
    assert.equal(evaluated, 'evaluated:document.title');
    assert.equal(seenUrl, 'https://example.com/');
  });

  it('reports a throwing handler as a failed step, and survives it', async () => {
    customControl('Cookie Consent', () => {
      throw new Error('handler blew up');
    });

    const s = await openFake();
    after(() => s.close());
    await assert.rejects(() => s.step("CLICK the 'Cookie Consent' banner"), /handler blew up/);
    // The session is still healthy: a broken handler is one bad step, not a
    // reason to tear the browser down.
    const out = await s.step("CLICK the 'Sign in' button");
    assert.equal(out.ok, true);
  });
});

describe('CALL handler', () => {
  it('returns a value back into the run', async () => {
    call('py.upper', (ctx) => ctx.args.join('|').toUpperCase());
    const s = await openFake();
    after(() => s.close());
    const result = await raw(s).call('call-host', { name: 'py.upper', args: ['a', 'b'] });
    assert.equal(result, 'A|B');
  });

  it('sees the variables the run carries', async () => {
    let seen: Record<string, string> = {};
    call('probe', (ctx) => {
      seen = ctx.vars;
      return null;
    });
    const s = await openFake();
    after(() => s.close());
    await raw(s).call('call-host', { name: 'probe', args: [] });
    assert.equal(seen['greeting'], 'hello');
  });

  it('fails the step when no handler claims the name', async () => {
    const s = await openFake();
    after(() => s.close());
    await assert.rejects(
      () => raw(s).call('call-host', { name: 'nobody.home', args: [] }),
      /no CALL handler registered/,
    );
  });
});

describe('suite hooks', () => {
  it('publishes variables back into the suite', async () => {
    beforeAll((ctx) => {
      ctx.variables['token'] = 'seeded';
    });
    const s = await openFake();
    after(() => s.close());
    const published = (await raw(s).call('fire-hook', { hook: 'before_all' })) as Record<
      string,
      string
    >;
    assert.equal(published['token'], 'seeded');
  });

  it('runs every handler in a slot, sharing one context', async () => {
    const order: string[] = [];
    beforeAll((ctx) => {
      order.push('first');
      ctx.variables['step'] = '1';
    });
    beforeAll((ctx) => {
      order.push('second');
      // A later hook sees what an earlier one published.
      ctx.variables['step'] = `${ctx.variables['step']}2`;
    });

    const s = await openFake();
    after(() => s.close());
    const published = (await raw(s).call('fire-hook', { hook: 'before_all' })) as Record<
      string,
      string
    >;
    assert.deepEqual(order, ['first', 'second']);
    assert.equal(published['step'], '12');
  });

  it('supports an async handler', async () => {
    beforeAll(async (ctx) => {
      await new Promise((r) => setTimeout(r, 1));
      ctx.variables['async'] = 'yes';
    });
    const s = await openFake();
    after(() => s.close());
    const published = (await raw(s).call('fire-hook', { hook: 'before_all' })) as Record<
      string,
      string
    >;
    assert.equal(published['async'], 'yes');
  });
});
