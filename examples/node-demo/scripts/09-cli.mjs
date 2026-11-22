// The `manul` command line, with the same Node handlers via --hooks.
//
//   node scripts/09-cli.mjs              # everything except the daemon
//   node scripts/09-cli.mjs --daemon     # also run `manul daemon` for ~70s

import { spawn } from 'node:child_process';
import { appendFileSync, existsSync, mkdirSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { ROOT, banner, check } from './_common.mjs';
import { startAppServer } from '../src/app-server.mjs';

// The package's own `manul` command (what `npx manul` runs), honouring $MANUL_BINARY.
const CLI = fileURLToPath(new URL('./cli.js', import.meta.resolve('manul-browser')));
const HOOKS = ['--hooks', 'manul_hooks.mjs'];
// The engine runs a .mjs hook script with $MANUL_NODE, else `node` on PATH.
process.env.MANUL_NODE ??= process.execPath;

mkdirSync(join(ROOT, 'reports'), { recursive: true });

// Asynchronous on purpose: the app server lives in this process, and a
// spawnSync would block the event loop it answers on — the hooks' fetch() to
// the backend would then never get a reply.
function manul(args, { input, env } = {}) {
  console.log(`  $ manul ${args.join(' ')}`);
  return new Promise((resolve) => {
    const child = spawn(process.execPath, [CLI, ...args], { cwd: ROOT, env: { ...process.env, ...env } });
    let stdout = '';
    let stderr = '';
    child.stdout.setEncoding('utf8').on('data', (d) => (stdout += d));
    child.stderr.setEncoding('utf8').on('data', (d) => (stderr += d));
    const timer = setTimeout(() => child.kill(), 600_000);
    child.on('close', (code) => {
      clearTimeout(timer);
      appendFileSync(join(ROOT, 'reports', 'cli.log'), `\n$ manul ${args.join(' ')}\n${stdout}\n${stderr}`);
      resolve({ code, stdout, stderr });
    });
    child.stdin.end(input ?? '');
  });
}

const app = await startAppServer();
process.env.DEMO_BASE_URL = app.baseUrl; // read by the beforeAll hook in the hook process
try {
  banner('Whole directory, Node hooks spawned by the engine, HTML report');
  // --tags picks every hunt but the quarantined one: on the CLI a hunt that a
  // beforeGroup hook refuses is BROKEN, and BROKEN exits 1.
  let r = await manul(['run', 'hunts/', ...HOOKS, '--tags', 'smoke,regression,monitoring', '--headless', '--html-report', '--screenshot', 'on-fail']);
  console.log(r.stdout.trim().split('\n').slice(-3).map((l) => '    ' + l).join('\n'));
  check(r.code === 0, 'every selected hunt passed');
  const reports = readdirSync(join(ROOT, 'reports')).filter((f) => /^report_.*\.html$/.test(f));
  check(reports.length > 0, `${reports.length} HTML report(s), e.g. ${reports.at(-1)}`);
  check(existsSync(join(ROOT, 'reports', 'run_history.json')), 'reports/run_history.json appended');

  banner('--tags smoke --jsonl: one JSON object per step, then a result per hunt');
  r = await manul(['run', 'hunts/', ...HOOKS, '--tags', 'smoke', '--jsonl']);
  const lines = r.stdout.split('\n').filter((l) => l.startsWith('{')).map((l) => JSON.parse(l));
  const kinds = [...new Set(lines.map((l) => l.event))].sort();
  check(r.code === 0 && kinds.includes('step') && kinds.includes('result'), `${lines.length} JSON lines, events ${JSON.stringify(kinds)}`);

  banner('--workers 2 --retries 1 --json: parallel browsers');
  r = await manul(['run', 'hunts/', ...HOOKS, '--tags', 'forms,widgets,data', '--workers', '2', '--retries', '1', '--json']);
  check(r.code === 0, 'parallel run passed');
  // --json prints one result document per hunt (and per @data row), back to back.
  const docs = [];
  let rest = r.stdout.trim();
  while (rest) {
    let depth = 0, end = 0, inStr = false, esc = false;
    for (; end < rest.length; end++) {
      const ch = rest[end];
      if (inStr) { esc = !esc && ch === '\\'; if (!esc && ch === '"') inStr = false; else if (ch !== '\\') esc = false; continue; }
      if (ch === '"') inStr = true;
      else if (ch === '{') depth++;
      else if (ch === '}' && --depth === 0) break;
    }
    docs.push(JSON.parse(rest.slice(0, end + 1)));
    rest = rest.slice(end + 1).trim();
  }
  console.log('    ' + docs.map((d) => `${d.title}=${d.passed}/${d.total_steps}`).join(', '));

  banner('--explain --disable-cache: per-channel score breakdown');
  r = await manul(['run', 'hunts/02_forms.hunt', ...HOOKS, '--explain', '--disable-cache', '--screenshot', 'none']);
  const breakdowns = (r.stdout + r.stderr).split('Breakdown:').length - 1;
  check(r.code === 0 && breakdowns > 0, `${breakdowns} breakdown lines`);

  banner('Auto-annotate: page names come from pages/localhost_8766.json');
  r = await manul(['run', 'hunts/06_data_driven.hunt', ...HOOKS], { env: { MANUL_AUTO_ANNOTATE: 'true' } });
  const navs = [...new Set((r.stdout + r.stderr).split('\n').filter((l) => l.includes('Auto-Nav:')).map((l) => l.split('Auto-Nav:')[1].trim()))];
  check(r.code === 0 && navs.includes('Login Page'), `Auto-Nav page names: ${JSON.stringify(navs)}`);

  banner('A hunt from stdin');
  r = await manul(['run', '-', '--headless'], { input: `STEP 1: stdin\n    NAVIGATE to ${app.baseUrl}/index.html\n    VERIFY that 'Sign in' is present\nDONE.\n` });
  check(r.code === 0, 'manul run - (stdin)');

  banner('A hunt refused by beforeGroup is BROKEN; a failing hunt is FAIL — both exit 1');
  r = await manul(['run', 'hunts/09_quarantine.hunt', ...HOOKS]);
  check(r.code === 1 && (r.stdout + r.stderr).includes('quarantined'), 'beforeGroup refused it');
  r = await manul(['run', 'hunts/09_quarantine.hunt']);
  check(r.code === 1, 'without the hooks it runs — no {base_url} global, no such text — and fails');

  banner('scan / schema / pages / controls');
  r = await manul(['scan', `${app.baseUrl}/forms.html`, '--output', 'reports/scanned_forms.hunt', '--headless']);
  check(r.code === 0 && existsSync(join(ROOT, 'reports', 'scanned_forms.hunt')), 'scan wrote a draft hunt');
  r = await manul(['schema']);
  check(JSON.parse(r.stdout).verbs, 'schema is JSON with verbs');
  r = await manul(['pages', 'list']); // the subcommand, not the pages/ directory beside it
  check(r.code === 0 && r.stdout.includes('pages'), r.stdout.trim().split('\n')[0]);
  r = await manul(['controls', 'list']);
  console.log(`    ${r.stdout.trim()}  (lists Go-registered controls; Node ones arrive at runtime via --hooks)`);

  if (process.argv.includes('--daemon')) {
    banner('daemon: runs hunts with @schedule: (08_scheduled.hunt, every 1 minute)');
    console.log('  $ manul daemon hunts/ --headless   (waits for the first scheduled run, ~60s)');
    const d = spawn(process.execPath, [CLI, 'daemon', 'hunts/', '--headless'], { cwd: ROOT });
    let seen = '';
    await new Promise((resolve) => {
      const onData = (b) => {
        seen += b;
        if (seen.includes('finished')) resolve();
      };
      d.stdout.on('data', onData);
      d.stderr.on('data', onData);
      d.on('exit', resolve);
      setTimeout(resolve, 120_000);
    });
    d.kill();
    check(seen.includes('BLOCK PASS'), 'the scheduled hunt ran and passed');
  }

  console.log('\n  not automated here (interactive): `manul record <url>`, `manul run manual/debug.hunt --debug`, `--break-lines 9`');
} finally {
  await app.close();
}
