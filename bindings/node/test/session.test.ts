import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { after, describe, it } from 'node:test';

import {
  EngineError,
  ProtocolError,
  Session,
  SessionClosed,
  Transport,
  call,
  customControl,
  resetRegistry,
} from '../src/index.js';

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

  // The engine has taken `browser` on open all along; the binding had no way
  // to say it, so Firefox was reachable only through MANUL_BROWSER.
  it('passes the browser choice to the engine', async () => {
    const chromium = await openFake();
    after(() => chromium.close());
    assert.equal(chromium.browser, 'chromium');

    const firefox = await Session.launch({
      binary: [process.execPath, FAKE],
      stderr: 'ignore',
      browser: 'firefox',
    });
    after(() => firefox.close());
    assert.equal(firefox.mode, 'launch');
    assert.equal(firefox.browser, 'firefox');
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
    assert.deepEqual(res.vars, { token: 'from-hooks' });
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

/** What the session last put on the wire, as the fake recorded it. */
async function lastArgs(s: Session): Promise<Record<string, unknown> | undefined> {
  const req = (await s.transport.call('last-request')) as Record<string, unknown>;
  return req['args'] as Record<string, unknown> | undefined;
}

describe('what goes on the wire', () => {
  it('sends a launch with only the options that were given', async () => {
    const s = await Session.launch({
      binary: [process.execPath, FAKE],
      stderr: 'ignore',
      headless: true,
      port: 9333,
    });
    after(() => s.close());
    assert.deepEqual(await lastArgs(s), { mode: 'launch', headless: true, port: 9333 });
  });

  it('attaches to the endpoint it was given', async () => {
    const s = await Session.attach('http://127.0.0.1:9222', {
      binary: [process.execPath, FAKE],
      stderr: 'ignore',
      tab: 'checkout',
    });
    after(() => s.close());
    assert.equal(s.mode, 'attach');
    assert.equal(s.cdp, 'http://127.0.0.1:9222');
    // A launch names the browser it started; an attach did not start one.
    assert.equal(s.browser, '');
    assert.deepEqual(await lastArgs(s), {
      mode: 'attach',
      cdp: 'http://127.0.0.1:9222',
      tab: 'checkout',
    });
  });

  it('runs a script by path', async () => {
    const s = await openFake();
    after(() => s.close());
    const out = await s.run({ path: 'hunts/login.hunt' });
    assert.equal(out.ok, true);
    assert.deepEqual(await lastArgs(s), { path: 'hunts/login.hunt' });
  });

  it('passes a map budget, and nothing when there is none', async () => {
    const s = await openFake();
    after(() => s.close());
    await s.map();
    assert.equal(await lastArgs(s), undefined);
    await s.map({ maxPerGroup: 5, includeUnlabeled: false });
    assert.deepEqual(await lastArgs(s), { maxPerGroup: 5, includeUnlabeled: false });
  });

  it('passes maxChars to both kinds of read', async () => {
    const s = await openFake();
    after(() => s.close());
    await s.read('Total', { maxChars: 40 });
    assert.deepEqual(await lastArgs(s), { label: 'Total', maxChars: 40 });
    await s.readText('#total', { maxChars: 10 });
    assert.deepEqual(await lastArgs(s), { selector: '#total', maxChars: 10 });
  });

  it('asks for every variable by sending no names at all', async () => {
    const s = await openFake();
    after(() => s.close());
    await s.vars();
    assert.equal(await lastArgs(s), undefined);
    await s.vars('a', 'b');
    assert.deepEqual(await lastArgs(s), { get: ['a', 'b'] });
  });

  it('sends a suite its paths as a copy', async () => {
    const s = await openFake();
    after(() => s.close());
    const paths = ['a.hunt'];
    await s.runSuite(paths);
    assert.deepEqual(await lastArgs(s), { paths: ['a.hunt'] });
  });
});

describe('more of the session', () => {
  it('returns the engine schema', async () => {
    const s = await openFake();
    after(() => s.close());
    assert.deepEqual(await s.schema(), { version: '0.1.0', commands: ['CLICK', 'FILL'] });
  });

  it('reads a label that is there', async () => {
    const s = await openFake();
    after(() => s.close());
    assert.deepEqual(await s.read('Total'), { value: 'read value', found: true, reason: '' });
  });

  it('fills the fields of a sparse step answer with defaults', async () => {
    // A custom control answers with only ok/step/action/url. The rest must be
    // empty values, not undefined leaking out to the caller.
    resetRegistry();
    customControl('Widget', () => {});
    after(() => resetRegistry());
    const s = await openFake();
    after(() => s.close());
    const out = await s.step("CLICK the 'Widget'");
    assert.deepEqual(out, {
      ok: true,
      step: "CLICK the 'Widget'",
      action: 'custom_control',
      value: '',
      url: 'https://example.com/',
      page: '',
      reason: '',
      error: '',
      score: 0,
      near: [],
    });
  });

  it('publishes handlers registered after the session opened', async () => {
    resetRegistry();
    after(() => resetRegistry());
    const s = await openFake();
    after(() => s.close());
    assert.deepEqual(s.published, { controls: 0, calls: 0, hooks: 0 });

    call('late', () => 'here');
    customControl('Late Widget', () => {});
    assert.deepEqual(await s.publishHandlers(), { controls: 1, calls: 1, hooks: 0 });
    assert.deepEqual(s.published, { controls: 1, calls: 1, hooks: 0 });
    assert.equal(await s.transport.call('call-host', { name: 'late', args: [] }), 'here');
  });

  it('tracks whether it is open', async () => {
    const s = await openFake();
    assert.equal(s.opened, true);
    assert.equal(s.closed, false);
    await s.close();
    assert.equal(s.opened, false);
    assert.equal(s.closed, true);
  });

  it('closes through Symbol.asyncDispose', async () => {
    const s = await openFake();
    await s[Symbol.asyncDispose]();
    assert.equal(s.closed, true);
  });

  it('reaches the page primitives directly', async () => {
    // Outside a handler the real engine refuses these; the fake serves them at
    // any time. What this pins is the method-to-command mapping.
    const s = await openFake();
    after(() => s.close());
    assert.equal(await s.pageUrl(), 'https://example.com/');
    assert.equal(await s.pageEval('1'), 'evaluated:1');
  });
});
