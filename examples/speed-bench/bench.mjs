#!/usr/bin/env node
// The same 26 tests on Manul and on Playwright, against the same local site
// and the same Chrome: ROUNDS rounds (5), each tool once per round, the one
// that goes first swapped every round. Reports the wall time of each whole
// run, launch included, and each test's own time.
//
//   node bench.mjs                    ROUNDS=5 WORKERS=1 MANUL=manul
//
// The site is served from here with caching off, so neither tool gets a
// warm cache from the other's run.

import { spawn, execFileSync } from 'node:child_process';
import fs from 'node:fs';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const siteDir = path.join(here, '..', 'quickstart', 'site');
const rounds = Number(process.env.ROUNDS ?? 5);
const workers = Number(process.env.WORKERS ?? 1);
const manulBin = process.env.MANUL ?? 'manul';
const pwCli = path.join(here, 'playwright', 'node_modules', 'playwright', 'cli.js');

const types = { '.html': 'text/html; charset=utf-8', '.css': 'text/css', '.json': 'application/json' };
const server = http.createServer((req, res) => {
  let p = decodeURIComponent(new URL(req.url, 'http://x').pathname);
  if (p.endsWith('/')) p += 'index.html';
  const file = path.join(siteDir, path.normalize(p));
  if (!file.startsWith(siteDir)) return res.writeHead(403).end();
  fs.readFile(file, (err, data) => {
    if (err) return res.writeHead(404).end();
    res.writeHead(200, { 'content-type': types[path.extname(file)] ?? 'application/octet-stream', 'cache-control': 'no-store' });
    res.end(data);
  });
});
await new Promise((ok) => server.listen(0, '127.0.0.1', ok));
const SITE = `http://127.0.0.1:${server.address().port}`;

function run(cmd, args, cwd, env = {}) {
  return new Promise((resolve) => {
    const start = process.hrtime.bigint();
    const child = spawn(cmd, args, { cwd, env: { ...process.env, ...env } });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (d) => (stdout += d));
    child.stderr.on('data', (d) => (stderr += d));
    child.on('close', (code) => resolve({ ms: Number(process.hrtime.bigint() - start) / 1e6, code, stdout, stderr }));
  });
}

// manul run --json writes one JSON document per hunt, one after another.
function jsonDocuments(text) {
  const docs = [];
  for (let i = 0; i < text.length; ) {
    const start = text.indexOf('{', i);
    if (start < 0) break;
    let depth = 0, inString = false, escaped = false, end = -1;
    for (let j = start; j < text.length; j++) {
      const c = text[j];
      if (inString) {
        if (escaped) escaped = false;
        else if (c === '\\') escaped = true;
        else if (c === '"') inString = false;
      } else if (c === '"') inString = true;
      else if (c === '{') depth++;
      else if (c === '}' && --depth === 0) { end = j; break; }
    }
    if (end < 0) break;
    docs.push(JSON.parse(text.slice(start, end + 1)));
    i = end + 1;
  }
  return docs;
}

async function manul() {
  const r = await run(manulBin, ['run', 'tests/', '--headless', '--workers', String(workers), '--var', `site=${SITE}`, '--json'], path.join(here, 'manul'));
  const tests = {};
  const byCommand = {};
  for (const d of jsonDocuments(r.stdout)) {
    tests[d.title] = { ms: d.total_duration_ms, ok: d.success === true };
    for (const s of d.results ?? []) {
      byCommand[s.command_type] ??= { n: 0, ms: 0 };
      byCommand[s.command_type].n++;
      byCommand[s.command_type].ms += s.duration_ms ?? 0;
    }
  }
  return { wall: r.ms, code: r.code, tests, byCommand, stderr: r.code ? r.stderr.slice(-2000) : '' };
}

async function playwright() {
  const r = await run(process.execPath, [pwCli, 'test', `--workers=${workers}`, '--reporter=json'], path.join(here, 'playwright'), { SITE });
  const tests = {};
  const walk = (suite) => {
    for (const spec of suite.specs ?? []) {
      for (const t of spec.tests ?? []) {
        const res = t.results?.at(-1);
        tests[spec.title] = { ms: res?.duration ?? 0, ok: res?.status === 'passed' };
      }
    }
    for (const s of suite.suites ?? []) walk(s);
  };
  try {
    for (const s of JSON.parse(r.stdout).suites ?? []) walk(s);
  } catch {
    // A run that could not report is a failed run; its code says so.
  }
  return { wall: r.ms, code: r.code, tests, stderr: r.code ? r.stderr.slice(-2000) : '' };
}

const tools = { manul, playwright };
const runs = { manul: [], playwright: [] };
for (let round = 1; round <= rounds; round++) {
  const order = round % 2 ? ['manul', 'playwright'] : ['playwright', 'manul'];
  for (const name of order) {
    const r = await tools[name]();
    runs[name].push(r);
    const passed = Object.values(r.tests).filter((t) => t.ok).length;
    console.error(`round ${round} ${name.padEnd(10)} ${(r.wall / 1000).toFixed(1).padStart(6)}s  ${passed}/${Object.keys(r.tests).length} passed${r.code ? `  exit ${r.code}` : ''}`);
    if (r.code) console.error(r.stderr);
  }
}
server.close();

const median = (xs) => {
  const s = [...xs].sort((a, b) => a - b);
  const m = s.length >> 1;
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
};
const sec = (ms) => (ms / 1000).toFixed(1) + 's';

const titles = Object.keys(runs.manul[0].tests).sort();
const perTest = titles.map((title) => {
  const m = median(runs.manul.map((r) => r.tests[title]?.ms ?? NaN));
  const p = median(runs.playwright.map((r) => r.tests[title]?.ms ?? NaN));
  return { title, manul: m, playwright: p, ratio: m / p };
});

console.log(`\n${rounds} rounds, ${workers} worker(s), ${titles.length} tests, ${SITE}\n`);
console.log('| | Manul | Playwright | Manul / Playwright |');
console.log('|---|---:|---:|---:|');
const wall = { manul: runs.manul.map((r) => r.wall), playwright: runs.playwright.map((r) => r.wall) };
console.log(`| whole run, median | ${sec(median(wall.manul))} | ${sec(median(wall.playwright))} | ${(median(wall.manul) / median(wall.playwright)).toFixed(1)}× |`);
console.log(`| whole run, fastest | ${sec(Math.min(...wall.manul))} | ${sec(Math.min(...wall.playwright))} | |`);
console.log(`| whole run, slowest | ${sec(Math.max(...wall.manul))} | ${sec(Math.max(...wall.playwright))} | |`);
const sum = (k) => perTest.reduce((a, t) => a + t[k], 0);
console.log(`| sum of test medians | ${sec(sum('manul'))} | ${sec(sum('playwright'))} | ${(sum('manul') / sum('playwright')).toFixed(1)}× |`);

console.log('\n| test | Manul | Playwright | Manul / Playwright |');
console.log('|---|---:|---:|---:|');
for (const t of perTest) console.log(`| ${t.title} | ${(t.manul / 1000).toFixed(2)}s | ${(t.playwright / 1000).toFixed(2)}s | ${t.ratio.toFixed(1)}× |`);

const commands = {};
for (const r of runs.manul) {
  for (const [k, v] of Object.entries(r.byCommand)) {
    commands[k] ??= { n: 0, ms: 0 };
    commands[k].n += v.n;
    commands[k].ms += v.ms;
  }
}
console.log('\nManul, by command (all rounds):\n');
console.log('| command | count per run | mean | share of time |');
console.log('|---|---:|---:|---:|');
const total = Object.values(commands).reduce((a, c) => a + c.ms, 0);
for (const [k, c] of Object.entries(commands).sort((a, b) => b[1].ms - a[1].ms)) {
  console.log(`| ${k} | ${c.n / rounds} | ${Math.round(c.ms / c.n)} ms | ${((100 * c.ms) / total).toFixed(0)}% |`);
}

const version = (cmd, args) => {
  try {
    return execFileSync(cmd, args, { encoding: 'utf8' }).trim();
  } catch {
    return 'unknown';
  }
};
const out = {
  date: new Date().toISOString(),
  rounds,
  workers,
  machine: `${os.type()} ${os.release()} ${os.arch()}, ${os.cpus()[0]?.model}, ${os.cpus().length} cores`,
  node: process.version,
  manul: version(manulBin, ['--version']),
  playwright: JSON.parse(fs.readFileSync(path.join(here, 'playwright', 'node_modules', 'playwright', 'package.json'), 'utf8')).version,
  runs,
  perTest,
};
fs.mkdirSync(path.join(here, 'results'), { recursive: true });
const file = path.join(here, 'results', `${out.date.slice(0, 10)}-w${workers}.json`);
fs.writeFileSync(file, JSON.stringify(out, null, 1));
console.error(`\nraw results: ${path.relative(process.cwd(), file)}`);
