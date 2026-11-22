// Attach to a Chrome that is already running, then drive it — from Node and
// from the one-shot agent CLI (`manul map`, `manul read`, `manul run-step`).
//
// In attach mode the browser is NOT closed when the session ends: Manul did
// not open it. The script starts and stops this Chrome itself.

import { spawn, spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Session, findBinary } from 'manul-browser';
import { ROOT, banner, check, withAppAndSession } from './_common.mjs';

const candidates = [
  process.env.CHROME_PATH,
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser',
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
];
const chrome = candidates.find((c) => c && existsSync(c));
if (!chrome) {
  console.error('no Chrome found; set CHROME_PATH');
  process.exit(1);
}

const PORT = 9334;
const endpoint = `http://127.0.0.1:${PORT}`;
const profile = mkdtempSync(join(tmpdir(), 'manul-attach-'));
const proc = spawn(chrome, [`--remote-debugging-port=${PORT}`, `--user-data-dir=${profile}`, '--headless=new',
  '--no-first-run', '--no-default-browser-check', 'about:blank'], { stdio: 'ignore' });

const up = async () => {
  for (let i = 0; i < 50; i++) {
    try {
      if ((await fetch(`${endpoint}/json/version`)).ok) return true;
    } catch {}
    await new Promise((r) => setTimeout(r, 200));
  }
  return false;
};

try {
  check(await up(), `Chrome listening on ${endpoint}`);
  await withAppAndSession({ mode: 'attach', cdp: endpoint }, async (app, s) => {
    banner(`Attached: mode=${s.mode}, cdp=${s.cdp}`);
    check(s.mode === 'attach', 'attach mode');
    await s.step(`NAVIGATE to ${app.baseUrl}/dashboard.html`);
    check((await s.state()).title === 'Dashboard', 'drove the existing tab');

    banner('One-shot agent CLI against the same browser');
    const manul = (...args) => spawnSync(findBinary(), args, { cwd: ROOT, encoding: 'utf8' }).stdout;
    let out = manul('map', '--cdp', endpoint, '--tab', 'dashboard', '--max-per-group', '3');
    check(out.includes('"groups"'), `manul map → ${out.slice(0, 80).replace(/\s+/g, ' ')}…`);
    out = manul('read', 'Order total', '--cdp', endpoint);
    check(out.includes('"142.50"'), `manul read → ${out.replace(/\s+/g, ' ').trim()}`);
    out = manul('run-step', "CLICK the 'Load More' button", '--cdp', endpoint, '--compact');
    check(out.replace(/\s/g, '').includes('"ok":true'), `manul run-step → ${out.replace(/\s+/g, ' ').slice(0, 80)}`);
  });

  banner('Session closed — the browser it attached to is still up');
  check((await fetch(`${endpoint}/json/version`)).ok, 'Chrome survived the session');

  // Session.attach(), and the same from the environment instead of arguments:
  const a = await Session.attach(endpoint, { cwd: ROOT, stderr: 'ignore' });
  check(a.mode === 'attach', 'Session.attach(endpoint)');
  await a.close();
  process.env.MANUL_BROWSER_MODE = 'attach';
  process.env.MANUL_CDP_ENDPOINT = endpoint;
  const e = await Session.open({ cwd: ROOT, stderr: 'ignore' });
  check(e.mode === 'attach', 'MANUL_BROWSER_MODE=attach + MANUL_CDP_ENDPOINT');
  await e.close();
} finally {
  proc.kill();
  await new Promise((r) => setTimeout(r, 500));
  rmSync(profile, { recursive: true, force: true });
}
