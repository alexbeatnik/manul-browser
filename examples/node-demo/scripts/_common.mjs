// Shared setup for the demo scripts: engine binary, app server, assertions.

import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { Session } from 'manul-browser';
import { startAppServer } from '../src/app-server.mjs';

export const ROOT = fileURLToPath(new URL('..', import.meta.url));

// A source checkout builds the engine into .bin/ (see README). An npm install
// brings its own through the @manul-browser/engine-<os>-<cpu> package.
const localEngine = join(ROOT, '.bin', process.platform === 'win32' ? 'manul.exe' : 'manul');
if (!process.env.MANUL_BINARY && existsSync(localEngine)) process.env.MANUL_BINARY = localEngine;

export function banner(title) {
  console.log(`\n\x1b[1;36m══ ${title} \x1b[0m`);
}

/** A demo assertion that says what it proved. */
export function check(cond, what) {
  if (!cond) {
    console.error(`\x1b[31m✗ ${what}\x1b[0m`);
    process.exit(1);
  }
  console.log(`  \x1b[32m✓\x1b[0m ${what}`);
}

/**
 * Start the site and a session whose working directory is the demo root.
 *
 * The engine reads manul.config.json, pages/ and relative MOCK/UPLOAD paths
 * from its working directory, so `cwd` matters. Engine logs (stderr) are
 * hidden unless DEMO_ENGINE_LOG=1.
 */
export async function withAppAndSession(options, fn) {
  const app = await startAppServer();
  let s;
  try {
    s = await Session.open({
      cwd: ROOT,
      stderr: process.env.DEMO_ENGINE_LOG ? 'inherit' : 'ignore',
      ...options,
    });
    return await fn(app, s);
  } finally {
    await s?.close();
    await app.close();
  }
}
