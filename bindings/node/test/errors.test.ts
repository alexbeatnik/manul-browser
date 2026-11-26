import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  EngineError,
  EngineNotFound,
  ManulError,
  ProtocolError,
  SessionClosed,
} from '../src/index.js';

describe('errors', () => {
  it('can all be caught as ManulError, and as Error', () => {
    const all = [
      new EngineNotFound(['somewhere']),
      new ProtocolError('bad line'),
      new EngineError('bad_request', 'nope'),
      new SessionClosed('closed'),
    ];
    for (const err of all) {
      assert.ok(err instanceof ManulError, `${err.name} is not a ManulError`);
      assert.ok(err instanceof Error);
    }
  });

  it('name themselves after their own class', () => {
    // A stack trace that says "Error" for every one of them is no help.
    assert.equal(new ProtocolError('x').name, 'ProtocolError');
    assert.equal(new EngineError('c', 'm').name, 'EngineError');
    assert.equal(new SessionClosed('x').name, 'SessionClosed');
    assert.equal(new EngineNotFound([]).name, 'EngineNotFound');
    assert.equal(new ManulError('x').name, 'ManulError');
  });

  it('keeps the engine code apart from the message', () => {
    const err = new EngineError('not_open', 'no browser yet');
    assert.equal(err.code, 'not_open');
    assert.equal(err.message, 'not_open: no browser yet');
  });

  it('lists each searched place on its own line', () => {
    const err = new EngineNotFound(['/a/manul (explicit path)', 'manul on PATH']);
    assert.deepEqual(err.searched, ['/a/manul (explicit path)', 'manul on PATH']);
    assert.match(err.message, /\n {2}\/a\/manul \(explicit path\)\n {2}manul on PATH\n/);
  });
});
