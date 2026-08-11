import assert from 'node:assert/strict';
import { beforeEach, describe, it } from 'node:test';

import {
  ANY_PAGE,
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
