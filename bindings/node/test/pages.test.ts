import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { Page, StepFailed, type PageSession } from '../src/index.js';
import type { RunOutcome, StepOutcome, Value } from '../src/session.js';

// Stands in for a Session: records what it is asked, fails a step that names 'Missing'.
function recorder() {
  const calls: Array<[string, unknown]> = [];
  const s: PageSession = {
    async step(instruction: string): Promise<StepOutcome> {
      calls.push(['step', instruction]);
      const ok = !instruction.includes('Missing');
      return {
        ok, step: instruction, action: '', value: '', url: '', page: '',
        reason: ok ? 'ok' : 'not_found', error: '', score: 0,
        near: ok ? [] : [{ label: 'Sign in', score: 0.4 }],
      };
    },
    async run(source: { source: string } | { path: string }): Promise<RunOutcome> {
      const text = 'source' in source ? source.source : source.path;
      calls.push(['run', text]);
      return { ok: !text.includes('Missing'), url: '', page: '', totalSteps: 1, passed: 1, failed: 0 };
    },
    async setVars(values: Record<string, string>) {
      calls.push(['setVars', values]);
      return values;
    },
    async read(label: string): Promise<Value> {
      calls.push(['read', label]);
      return { value: '24.50', found: true, reason: 'ok' } as Value;
    },
  };
  return { s, calls };
}

class Login extends Page {
  static override pageName = 'Login';
}

describe('Page', () => {
  it('opens by name and asks the engine whether the page is open', async () => {
    const { s, calls } = recorder();
    await new Login(s).open();
    assert.deepEqual(calls, [
      ['step', "NAVIGATE to 'Login'"],
      ['step', "VERIFY that the page is 'Login'"],
    ]);
  });

  it('opens by path when one is given', async () => {
    class Shop extends Page {
      static override pageName = 'Shop';
      static override path = '/shop.html';
    }
    const { s, calls } = recorder();
    await new Shop(s, 'http://localhost:8000/').open();
    assert.deepEqual(calls[0], ['step', 'NAVIGATE to http://localhost:8000/shop.html']);
  });

  it("runs the page file's action with its variables", async () => {
    const { s, calls } = recorder();
    await new Login(s).use('SignIn', { user: 'ada', password: 'lovelace' });
    assert.deepEqual(calls, [
      ['setVars', { user: 'ada', password: 'lovelace' }],
      ['run', 'USE Login.SignIn'],
    ]);
  });

  it('throws StepFailed with the reason and the closest labels', async () => {
    const { s } = recorder();
    await assert.rejects(new Login(s).do("CLICK the 'Missing' button"), (e: unknown) => {
      assert.ok(e instanceof StepFailed);
      assert.match(e.message, /not_found/);
      assert.match(e.message, /Sign in/);
      return true;
    });
  });

  it('says so when a page has no name', async () => {
    const { s } = recorder();
    await assert.rejects(new Page(s).open(), /has no pageName/);
  });

  it('reads one value', async () => {
    const { s } = recorder();
    assert.equal(await new Login(s).valueOf('Price'), '24.50');
  });
});
