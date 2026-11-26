/**
 * Which engine gets picked.
 *
 * The order is shared with `manul/_binary.py` on purpose, so it is pinned here:
 * two bindings that disagree about which engine they started are very hard to
 * debug. No platform package is installed in a checkout, which is what lets
 * these tests reach the PATH step at all.
 */

import assert from 'node:assert/strict';
import { chmodSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { delimiter, join } from 'node:path';
import { afterEach, beforeEach, describe, it } from 'node:test';

import { BINARY_NAME, EngineNotFound, findBinary, platformPackage } from '../src/index.js';

let dir = '';
let savedPath: string | undefined;
let savedBinary: string | undefined;

/** An empty file that passes for an engine: findBinary only checks it exists. */
function engineAt(...parts: string[]): string {
  const path = join(dir, ...parts);
  mkdirSync(join(path, '..'), { recursive: true });
  writeFileSync(path, '');
  chmodSync(path, 0o755);
  return path;
}

beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), 'manul-binary-'));
  savedPath = process.env['PATH'];
  savedBinary = process.env['MANUL_BINARY'];
  // Start from nothing, so a developer's own `manul` cannot leak into a test.
  process.env['PATH'] = join(dir, 'empty');
  delete process.env['MANUL_BINARY'];
});

afterEach(() => {
  if (savedPath === undefined) delete process.env['PATH'];
  else process.env['PATH'] = savedPath;
  if (savedBinary === undefined) delete process.env['MANUL_BINARY'];
  else process.env['MANUL_BINARY'] = savedBinary;
  rmSync(dir, { recursive: true, force: true });
});

describe('findBinary', () => {
  it('prefers an explicit path over everything else', () => {
    const explicit = engineAt('explicit', BINARY_NAME);
    process.env['MANUL_BINARY'] = engineAt('env', BINARY_NAME);
    process.env['PATH'] = join(engineAt('path', BINARY_NAME), '..');
    assert.equal(findBinary(explicit), explicit);
  });

  it('uses MANUL_BINARY ahead of PATH', () => {
    const env = engineAt('env', BINARY_NAME);
    process.env['MANUL_BINARY'] = env;
    process.env['PATH'] = join(engineAt('path', BINARY_NAME), '..');
    assert.equal(findBinary(), env);
  });

  it('falls through an explicit path that does not exist', () => {
    const env = engineAt('env', BINARY_NAME);
    process.env['MANUL_BINARY'] = env;
    assert.equal(findBinary(join(dir, 'missing', BINARY_NAME)), env);
  });

  it('finds manul on PATH, in the first directory that has it', () => {
    const first = engineAt('one', BINARY_NAME);
    engineAt('two', BINARY_NAME);
    process.env['PATH'] = [join(dir, 'nothing-here'), join(dir, 'one'), join(dir, 'two')].join(
      delimiter,
    );
    assert.equal(findBinary(), first);
  });

  it('skips a directory on PATH that is named like the engine', () => {
    mkdirSync(join(dir, 'decoy', BINARY_NAME), { recursive: true });
    const real = engineAt('real', BINARY_NAME);
    process.env['PATH'] = [join(dir, 'decoy'), join(dir, 'real')].join(delimiter);
    assert.equal(findBinary(), real);
  });

  it('lists every place it looked when nothing is found', () => {
    const explicit = join(dir, 'gone', BINARY_NAME);
    process.env['MANUL_BINARY'] = join(dir, 'also-gone', BINARY_NAME);
    assert.throws(
      () => findBinary(explicit),
      (err: unknown) => {
        assert.ok(err instanceof EngineNotFound);
        assert.deepEqual(err.searched, [
          `${explicit} (explicit path)`,
          `${process.env['MANUL_BINARY']} (MANUL_BINARY)`,
          `${platformPackage()} (not installed)`,
          `${BINARY_NAME} on PATH`,
        ]);
        for (const place of err.searched) assert.ok(err.message.includes(place));
        assert.match(err.message, /Set MANUL_BINARY/);
        return true;
      },
    );
  });

  it('does not list sources that were never offered', () => {
    assert.throws(
      () => findBinary(),
      (err: unknown) => {
        assert.ok(err instanceof EngineNotFound);
        assert.deepEqual(err.searched, [
          `${platformPackage()} (not installed)`,
          `${BINARY_NAME} on PATH`,
        ]);
        return true;
      },
    );
  });
});

describe('platformPackage', () => {
  it('names the package for this host', () => {
    assert.equal(platformPackage(), `@manul-browser/engine-${process.platform}-${process.arch}`);
  });

  it('names the binary for this host', () => {
    assert.equal(BINARY_NAME, process.platform === 'win32' ? 'manul.exe' : 'manul');
  });
});
