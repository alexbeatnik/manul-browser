// End-to-end check that an installed package actually drives Chrome.
//
// Deliberately not part of `npm test`: everything else in test/ runs against a
// fake engine precisely so it needs no browser, and this needs a real one. It
// is the Node counterpart of bindings/python/tests/smoke_release.py and checks
// the same thing — that what was packed installs, finds its engine in the
// platform package, and can drive a page.
//
// It imports `manul-browser` by name, so it has to run from a project that has
// the tarballs installed; the publish workflow copies it into one:
//
//     MANUL_CHROME=/path/to/chrome node smoke-release.mjs
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

import { Session, findBinary } from 'manul-browser';

const PAGE = `<!doctype html>
<meta charset="utf-8">
<title>Manul smoke</title>
<h1>Checkout</h1>
<label for="email">Email address</label>
<input id="email">
<button id="signin">Sign in</button>
`;

const engine = findBinary();
console.log(`engine  ${engine}`);
// The whole point of the platform packages: the engine must come from the
// install, not from a `manul` that happens to be on this machine's PATH.
if (!engine.includes('node_modules')) {
  console.log('       the engine was not resolved from the installed platform package');
  process.exit(1);
}

const dir = mkdtempSync(join(tmpdir(), 'manul-smoke-'));
let failed = false;
try {
  const page = join(dir, 'smoke.html');
  writeFileSync(page, PAGE, 'utf8');

  const s = await Session.launch({
    headless: true,
    executablePath: process.env.MANUL_CHROME || undefined,
  });
  try {
    console.log(`package engine ${s.engineVersion}, protocol ${s.protocol}`);
    for (const step of [
      `NAVIGATE to ${pathToFileURL(page).href}`,
      "FILL 'Email address' field with 'ada@example.com'",
      "CLICK the 'Sign in' button",
    ]) {
      const out = await s.step(step);
      console.log(`  ${out.ok ? 'ok ' : 'FAIL'} ${step}`);
      if (!out.ok) {
        console.log(`       reason: ${out.reason}  near: ${JSON.stringify(out.near)}`);
        failed = true;
        break;
      }
    }
    if (!failed) {
      const labels = (await s.map()).labels();
      console.log(`  map labels: ${JSON.stringify(labels)}`);
      if (labels.length === 0) {
        console.log('       map returned nothing at all');
        failed = true;
      }
    }
  } finally {
    await s.close();
  }
} finally {
  rmSync(dir, { recursive: true, force: true });
}

console.log(failed ? 'smoke FAILED' : 'smoke ok');
process.exit(failed ? 1 : 0);
