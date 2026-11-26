// Run the whole demo: every script, then the node:test suite.
//
//   node run-all.mjs               # Chromium only
//   node run-all.mjs --firefox     # + the Firefox (WebDriver BiDi) script
//   node run-all.mjs --attach      # + attach to a running Chrome, agent CLI
//   node run-all.mjs --daemon      # + `manul daemon` (~70s)
//   node run-all.mjs --all

import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const ROOT = fileURLToPath(new URL('.', import.meta.url));
const args = new Set(process.argv.slice(2));
const all = args.has('--all');

const steps = [
  ['scripts/01-session-basics.mjs'],
  ['scripts/02-page-objects.mjs'],
  ['scripts/03-controls-and-calls.mjs'],
  ['scripts/04-run-hunts.mjs'],
  ['scripts/05-suite.mjs'],
  ['scripts/06-errors.mjs'],
];
if (all || args.has('--firefox')) steps.push(['scripts/07-firefox.mjs']);
if (all || args.has('--attach')) steps.push(['scripts/08-attach.mjs']);
steps.push(['scripts/09-cli.mjs', ...(all || args.has('--daemon') ? ['--daemon'] : [])]);
steps.push(['--test', '--test-concurrency=1', 'test/']);

const results = [];
for (const step of steps) {
  const name = step.join(' ');
  console.log(`\n\x1b[1;35m▶ ${name}\x1b[0m`);
  const t0 = Date.now();
  const r = spawnSync(process.execPath, step, { cwd: ROOT, stdio: 'inherit' });
  results.push([name, r.status ?? 1, (Date.now() - t0) / 1000]);
}

console.log('\n\x1b[1mSummary\x1b[0m');
for (const [name, code, secs] of results) {
  console.log(`  ${code === 0 ? '\x1b[32mPASS' : '\x1b[31mFAIL'}\x1b[0m  ${secs.toFixed(1).padStart(5)}s  ${name}`);
}
process.exit(Math.max(...results.map(([, code]) => code)));
