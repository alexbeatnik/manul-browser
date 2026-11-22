import assert from 'node:assert/strict';
import { beforeEach, describe, it } from 'node:test';

import {
  ANY_PAGE,
  afterAll,
  afterGroup,
  beforeAll,
  beforeGroup,
  call,
  customControl,
  diagnoseCustomControlMiss,
  getCall,
  getCustomControl,
  getHooks,
  listCalls,
  listCustomControls,
  listHooks,
  resetRegistry,
} from '../src/index.js';
import type { CallHandler, ControlContext, ControlHandler, HookHandler } from '../src/index.js';
import { dispatchInvoke, registrationPayload, type PagePeer } from '../src/controls.js';

beforeEach(() => resetRegistry());

describe('custom controls', () => {
  it('matches case- and whitespace-insensitively', () => {
    customControl({ page: 'Checkout Page', target: 'React Datepicker' }, () => {});
    assert.ok(getCustomControl('checkout   page', 'react datepicker'));
    assert.ok(getCustomControl('CHECKOUT PAGE', '  React Datepicker  '));
  });

  it('falls back to an any-page registration', () => {
    customControl('Cookie Consent', () => {});
    assert.ok(getCustomControl('Anything At All', 'Cookie Consent'));
    assert.deepEqual(listCustomControls(), [{ page: ANY_PAGE, target: 'Cookie Consent' }]);
  });

  it('prefers the page-specific registration over the wildcard', () => {
    let picked = '';
    customControl('Accept', () => {
      picked = 'any';
    });
    customControl({ page: 'Login', target: 'Accept' }, () => {
      picked = 'login';
    });
    const handler = getCustomControl('Login', 'Accept');
    assert.ok(handler);
    handler?.({
      target: 'Accept',
      action: 'click',
      value: '',
      page: 'Login',
      step: '',
      vars: {},
      eval: async () => null,
      url: async () => '',
    });
    assert.equal(picked, 'login');
  });

  it('returns null for something nobody registered', () => {
    assert.equal(getCustomControl('*', 'Nothing'), null);
  });

  it('refuses an empty target', () => {
    assert.throws(() => customControl('   ', () => {}), TypeError);
  });

  it('explains a miss in terms of what is registered', () => {
    customControl({ page: 'Checkout', target: 'Datepicker' }, () => {});
    const hint = diagnoseCustomControlMiss('Login', 'Datepicker');
    assert.ok(hint?.includes('Checkout'));
    // Nothing registered under that target at all: no hint to give.
    assert.equal(diagnoseCustomControlMiss('Login', 'Unheard Of'), null);
  });
});

describe('call handlers', () => {
  it('registers and looks up by normalised name', () => {
    call('py.Upper', () => 'x');
    assert.ok(getCall('PY.UPPER'));
    assert.deepEqual(listCalls(), ['py.upper']);
  });

  it('refuses an empty name', () => {
    assert.throws(() => call('', () => null), TypeError);
  });
});

describe('suite hooks', () => {
  it('keeps every handler registered for one slot', () => {
    beforeAll(() => {});
    beforeAll(() => {});
    assert.equal(getHooks('before_all', '').length, 2);
    // One slot, not one per handler — the engine registers a single bridge and
    // this side runs both.
    assert.deepEqual(listHooks(), [{ kind: 'before_all', tag: '' }]);
  });

  it('separates group hooks by tag', () => {
    beforeGroup('smoke', () => {});
    afterGroup('regression', () => {});
    assert.equal(getHooks('before_group', 'smoke').length, 1);
    assert.equal(getHooks('before_group', 'regression').length, 0);
    assert.equal(getHooks('after_group', 'regression').length, 1);
  });

  it('refuses a group hook with no tag', () => {
    assert.throws(() => beforeGroup('  ', () => {}), TypeError);
  });
});

describe('dispatchInvoke', () => {
  const peer: PagePeer = {
    pageEval: async (js) => `eval:${js}`,
    pageUrl: async () => 'https://example.com/',
  };

  it('refuses a callback kind it does not know', async () => {
    await assert.rejects(() => dispatchInvoke({ kind: 'teleport' }, peer), /unknown callback kind "teleport"/);
  });

  it('names the page when no control matches at all', async () => {
    await assert.rejects(
      () => dispatchInvoke({ kind: 'custom_control', page: 'Login', target: 'Widget' }, peer),
      /no custom control for "Widget" on page "Login"$/,
    );
  });

  it('points at the page a control was registered for', async () => {
    customControl({ page: 'Checkout', target: 'Widget' }, () => {});
    await assert.rejects(
      () => dispatchInvoke({ kind: 'custom_control', page: 'Login', target: 'Widget' }, peer),
      /registered for "Checkout"/,
    );
  });

  it('sends nothing back from a control but its success', async () => {
    customControl('Widget', () => 'ignored' as unknown as void);
    const out = await dispatchInvoke({ kind: 'custom_control', page: 'Any', target: 'Widget' }, peer);
    assert.equal(out, null);
  });

  it('hands a control everything the step carried, with string defaults', async () => {
    let seen: ControlContext | null = null;
    customControl('Widget', (ctx) => {
      seen = ctx;
    });
    await dispatchInvoke(
      { kind: 'custom_control', page: 'Shop', target: 'Widget', action: 'fill', value: 'x', step: 'FILL', vars: { a: '1' } },
      peer,
    );
    assert.ok(seen);
    const ctx = seen as ControlContext;
    assert.deepEqual(
      { target: ctx.target, action: ctx.action, value: ctx.value, page: ctx.page, step: ctx.step, vars: ctx.vars },
      { target: 'Widget', action: 'fill', value: 'x', page: 'Shop', step: 'FILL', vars: { a: '1' } },
    );
    assert.equal(await ctx.eval('1+1'), 'eval:1+1');
    assert.equal(await ctx.url(), 'https://example.com/');

    // Missing fields arrive as empty strings, never as "undefined".
    await dispatchInvoke({ kind: 'custom_control', target: 'Widget' }, peer);
    const bare = seen as ControlContext;
    assert.equal(bare.action, '');
    assert.equal(bare.value, '');
    assert.equal(bare.step, '');
    assert.deepEqual(bare.vars, {});
  });

  it('does not let a handler change the variables it was handed', async () => {
    const vars = { a: '1' };
    call('mutate', (ctx) => {
      ctx.vars['a'] = 'changed';
      return null;
    });
    await dispatchInvoke({ kind: 'call', name: 'mutate', args: [], vars }, peer);
    assert.equal(vars.a, '1');
  });

  it('turns a CALL handler that returns nothing into null', async () => {
    call('quiet', () => undefined);
    assert.equal(await dispatchInvoke({ kind: 'call', name: 'quiet' }, peer), null);
  });

  it('copies the CALL arguments, defaulting to none', async () => {
    let args: string[] = ['sentinel'];
    call('args', (ctx) => {
      args = ctx.args;
      return null;
    });
    await dispatchInvoke({ kind: 'call', name: 'ARGS' }, peer);
    assert.deepEqual(args, []);
  });

  it('names the tag when a group hook is missing', async () => {
    await assert.rejects(
      () => dispatchInvoke({ kind: 'hook', hook: 'before_group', tag: 'smoke' }, peer),
      /no "before_group" hook registered for tag "smoke"/,
    );
    await assert.rejects(
      () => dispatchInvoke({ kind: 'hook', hook: 'after_all', tag: '' }, peer),
      /no "after_all" hook registered$/,
    );
  });

  it('stops a slot at the first handler that throws', async () => {
    const ran: string[] = [];
    afterAll(() => {
      ran.push('first');
      throw new Error('stop');
    });
    afterAll(() => {
      ran.push('second');
    });
    await assert.rejects(() => dispatchInvoke({ kind: 'hook', hook: 'after_all', tag: '' }, peer), /stop/);
    assert.deepEqual(ran, ['first']);
  });

  it('stringifies what set() publishes', async () => {
    beforeAll((ctx) => {
      ctx.set('n', 42);
      ctx.set('flag', true);
    });
    const out = await dispatchInvoke({ kind: 'hook', hook: 'before_all', tag: '' }, peer);
    assert.deepEqual(out, { n: '42', flag: 'true' });
  });
});

describe('registrationPayload', () => {
  it('is empty with nothing registered', () => {
    assert.deepEqual(registrationPayload(), { controls: [], calls: [], hooks: [] });
  });

  it('lists calls sorted, and keeps controls as registered', () => {
    call('zeta', () => null);
    call('Alpha', () => null);
    customControl({ page: 'Checkout Page', target: 'Datepicker' }, () => {});
    const payload = registrationPayload();
    assert.deepEqual(payload.calls, ['alpha', 'zeta']);
    // The engine gets the label as written; normalising is for lookup only.
    assert.deepEqual(payload.controls, [{ page: 'Checkout Page', target: 'Datepicker' }]);
  });

  it('replaces a control registered twice under one name', () => {
    customControl('Widget', () => {});
    customControl('  widget ', () => {});
    assert.equal(listCustomControls().length, 1);
  });
});

describe('registration guards', () => {
  it('refuses a handler that is not a function', () => {
    assert.throws(() => customControl('X', null as unknown as ControlHandler), TypeError);
    assert.throws(() => call('x', 'nope' as unknown as CallHandler), TypeError);
    assert.throws(() => beforeAll(undefined as unknown as HookHandler), TypeError);
  });

  it('refuses after_group with no tag', () => {
    assert.throws(() => afterGroup('', () => {}), TypeError);
  });

  it('empties every registry on reset', () => {
    customControl('X', () => {});
    call('x', () => null);
    afterAll(() => {});
    resetRegistry();
    assert.deepEqual(registrationPayload(), { controls: [], calls: [], hooks: [] });
  });
});
