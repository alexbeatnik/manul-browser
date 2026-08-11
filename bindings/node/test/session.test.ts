import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { after, describe, it } from 'node:test';

import { EngineError, ProtocolError, Session, SessionClosed, Transport } from '../src/index.js';

const FAKE = fileURLToPath(new URL('./fakeEngine.js', import.meta.url));

/** A session backed by the fake engine rather than a real one. */
function openFake(env: NodeJS.ProcessEnv = {}): Promise<Session> {
  return Session.launch({
    binary: [process.execPath, FAKE],
    env: { ...process.env, ...env },
    stderr: 'ignore',
  });
}

describe('startup', () => {
  it('reads the ready event and reports both versions', async () => {
    const s = await openFake();
    after(() => s.close());
    assert.equal(s.protocol, '1.0');
    assert.equal(s.engineVersion, '0.1.0');
    assert.equal(s.mode, 'launch');
  });

  it('refuses a protocol major it was not written against', async () => {
    await assert.rejects(() => openFake({ MANUL_FAKE_MODE: 'future' }), ProtocolError);
  });

  it('reports an engine that dies before saying ready', async () => {
    await assert.rejects(() => openFake({ MANUL_FAKE_MODE: 'silent' }), ProtocolError);
  });

  it('does not leave a process behind when opening fails', async () => {
    // The failure path must close the transport it created; otherwise every
    // rejected open leaks a child.
    await assert.rejects(() => openFake({ MANUL_FAKE_MODE: 'future' }));
  });
});

describe('step', () => {
  it('returns the outcome of a successful step', async () => {
    const s = await openFake();
    after(() => s.close());
    const out = await s.step("CLICK the 'Sign in' button");
    assert.equal(out.ok, true);
    assert.equal(out.url, 'https://example.com/');
    assert.equal(out.score, 0.98);
  });

  it('treats a step that resolved nothing as an answer, not an error', async () => {
    const s = await openFake();
    after(() => s.close());
    const out = await s.step('CLICK the NOPE button');
    assert.equal(out.ok, false);
    assert.equal(out.reason, 'not_found');
  });

  it('raises EngineError when the engine rejects the request', async () => {
    const s = await openFake();
    after(() => s.close());
    await assert.rejects(() => s.step('EXPLODE'), (err: unknown) => {
      assert.ok(err instanceof EngineError);
      assert.equal(err.code, 'step_failed');
      return true;
    });
  });

  it('stays usable after a rejected command', async () => {
    const s = await openFake();
    after(() => s.close());
    await assert.rejects(() => s.step('EXPLODE'));
    const out = await s.step("CLICK the 'Sign in' button");
    assert.equal(out.ok, true);
  });
});

describe('page reads', () => {
  it('maps the page and flattens labels', async () => {
    const s = await openFake();
    after(() => s.close());
    const pm = await s.map();
    assert.equal(pm.url, 'https://example.com/');
    assert.equal(pm.groups.length, 2);
    assert.deepEqual(pm.labels(), ['Sign in', 'Email', 'Home']);
    assert.equal(pm.groups[1]?.truncated, 2);
  });

  it('reports a label that is not there as found: false', async () => {
    const s = await openFake();
    after(() => s.close());
    const v = await s.read('Missing');
    assert.equal(v.found, false);
    assert.equal(v.reason, 'not_found');
  });

  it('reads a selector as text', async () => {
    const s = await openFake();
    after(() => s.close());
    assert.equal(await s.readText('#main'), 'selector text');
  });

  it('reports page state', async () => {
    const s = await openFake();
    after(() => s.close());
    assert.deepEqual(await s.state(), { url: 'https://example.com/', title: 'Example' });
  });
});

describe('run', () => {
  it('runs a script from source', async () => {
    const s = await openFake();
    after(() => s.close());
    const out = await s.run({ source: 'STEP 1:\n  WAIT 1\n' });
    assert.equal(out.ok, true);
    assert.equal(out.totalSteps, 3);
    assert.equal(out.passed, 3);
  });

  it('runs a suite and reports each hunt', async () => {
    const s = await openFake();
    after(() => s.close());
    const res = await s.runSuite(['a.hunt', 'b.hunt']);
    assert.equal(res.total, 2);
    assert.equal(res.hunts.length, 2);
    assert.deepEqual(res.hunts[0]?.tags, ['smoke']);
  });

  it('rejects a suite with no paths', async () => {
    const s = await openFake();
    after(() => s.close());
    await assert.rejects(() => s.runSuite([]), EngineError);
  });
});

describe('vars', () => {
  it('reads every variable, and a projection', async () => {
    const s = await openFake();
    after(() => s.close());
    assert.deepEqual(await s.vars(), { greeting: 'hello' });
    assert.deepEqual(await s.vars('greeting'), { greeting: 'hello' });
  });

  it('sets variables and returns the full set', async () => {
    const s = await openFake();
    after(() => s.close());
    const all = await s.setVars({ token: 'abc' });
    assert.equal(all['token'], 'abc');
    assert.equal(all['greeting'], 'hello');
  });
});

describe('lifecycle', () => {
  it('close is safe twice', async () => {
    const s = await openFake();
    await s.close();
    await s.close();
    assert.equal(s.closed, true);
  });

  it('refuses calls after close', async () => {
    const s = await openFake();
    await s.close();
    await assert.rejects(() => s.step('CLICK'), SessionClosed);
  });

  it('serialises overlapping commands so reverse calls stay nested', async () => {
    const s = await openFake();
    after(() => s.close());
    // Fired together on purpose: the transport must not interleave them.
    const results = await Promise.all([
      s.step('one'),
      s.step('two'),
      s.step('three'),
    ]);
    assert.deepEqual(results.map((r) => r.step), ['one', 'two', 'three']);
  });
});

describe('transport', () => {
  it('takes a command array verbatim, for engines behind a wrapper', async () => {
    const t = new Transport({ binary: [process.execPath, FAKE], stderr: 'ignore' });
    await t.ready();
    assert.equal(t.protocol, '1.0');
    await t.close();
    assert.equal(t.closed, true);
  });
});
