/**
 * Locating the `manul` engine binary.
 *
 * A published install normally carries the binary in a platform package, but a
 * developer working on the engine wants their own build to win without
 * reinstalling anything. Hence the order below: an installed package is
 * preferred over whatever PATH happens to hold, because an install should be
 * self-contained and predictable, and MANUL_BINARY exists for the developer who
 * wants to override that.
 *
 * The order matches `manul/_binary.py` deliberately. Two bindings that disagree
 * about which engine they picked would be very hard to debug.
 */

import { accessSync, constants, chmodSync, statSync } from 'node:fs';
import { createRequire } from 'node:module';
import { delimiter, join } from 'node:path';

import { EngineNotFound } from './errors.js';

export const BINARY_NAME = process.platform === 'win32' ? 'manul.exe' : 'manul';

/**
 * The platform package that would carry the engine for this host.
 *
 * `manul-browser` lists one of these per target as `optionalDependencies`, so
 * npm installs exactly the one that matches. `scripts/pack.mjs` builds them and
 * adds that list at pack time. In a checkout none is installed, so resolution
 * failing here is the expected case there, not an error.
 */
export function platformPackage(): string {
  return `@manul-browser/engine-${process.platform}-${process.arch}`;
}

function isFile(path: string): boolean {
  try {
    return statSync(path).isFile();
  } catch {
    return false;
  }
}

/**
 * Restore the executable bit if something ate it.
 *
 * npm preserves modes, so this is normally a no-op. It is not always: a tarball
 * repacked by a mirror, or an install onto a filesystem with no permission
 * model, can arrive without it. A read-only install directory is not an error
 * here — the spawn attempt reports the real problem in a moment.
 */
function ensureExecutable(path: string): void {
  if (process.platform === 'win32') return;
  try {
    accessSync(path, constants.X_OK);
    return;
  } catch {
    /* fall through and try to fix it */
  }
  try {
    chmodSync(path, statSync(path).mode | 0o111);
  } catch {
    /* reported by the spawn, not here */
  }
}

/** Look for `manul` on PATH, honouring PATHEXT on Windows. */
function onPath(): string | null {
  const dirs = (process.env['PATH'] ?? '').split(delimiter).filter(Boolean);
  for (const dir of dirs) {
    const candidate = join(dir, BINARY_NAME);
    if (isFile(candidate)) return candidate;
  }
  return null;
}

/**
 * Return the path to the engine binary.
 *
 * Resolution order:
 *
 * 1. `explicit`, when the caller passed one.
 * 2. `$MANUL_BINARY`.
 * 3. The platform package for this host.
 * 4. `manul` on `PATH`.
 *
 * @throws {EngineNotFound} listing everything that was tried.
 */
export function findBinary(explicit?: string): string {
  const searched: string[] = [];

  if (explicit) {
    if (isFile(explicit)) return explicit;
    searched.push(`${explicit} (explicit path)`);
  }

  const env = process.env['MANUL_BINARY'];
  if (env) {
    if (isFile(env)) return env;
    searched.push(`${env} (MANUL_BINARY)`);
  }

  const pkg = platformPackage();
  try {
    const require = createRequire(import.meta.url);
    const bundled = join(require.resolve(`${pkg}/package.json`), '..', 'bin', BINARY_NAME);
    if (isFile(bundled)) {
      ensureExecutable(bundled);
      return bundled;
    }
    searched.push(`${bundled} (${pkg})`);
  } catch {
    searched.push(`${pkg} (not installed)`);
  }

  const found = onPath();
  if (found) return found;
  searched.push(`${BINARY_NAME} on PATH`);

  throw new EngineNotFound(searched);
}
