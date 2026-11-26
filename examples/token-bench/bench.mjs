// How many tokens it costs an LLM agent to see a page and act on it, with
// Playwright MCP and with Manul, measured on the same Chrome tab.
//
//   npm install
//   node bench.mjs [pages.json] > results.json
//
// MANUL is the manul binary (default: `manul` on PATH); CHROME the Chrome
// binary (default: the macOS install). See README.md and
// core/docs/token-efficiency.md for what is measured and what is not.
import { spawn, execFileSync } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname, extname, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';
import { getEncoding } from 'js-tiktoken';

const here = dirname(fileURLToPath(import.meta.url));
const pagesFile = process.argv[2] || join(here, 'pages.json');
const manul = process.env.MANUL || 'manul';
const chromePath = process.env.CHROME || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

// cl100k_base: the tokenizer the engine's README has always quoted. Claude's
// tokenizer is not public; the ratios, not the absolute counts, are the point.
const enc = getEncoding('cl100k_base');
const tokens = (s) => enc.encode(s).length;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const PORT = 9411;
const CDP = `http://127.0.0.1:${PORT}`;
const scratch = mkdtempSync(join(tmpdir(), 'tokbench-'));

// ── The repository's e2e fixtures, for pages that never change ──────────────
const siteRoot = join(here, '..', '..', 'core', 'e2e', 'testdata', 'site');
const types = { '.html': 'text/html; charset=utf-8', '.txt': 'text/plain' };
const site = createServer((req, res) => {
  const path = normalize(decodeURIComponent(new URL(req.url, 'http://x').pathname)).replace(/^([/\\])+/, '');
  const file = join(siteRoot, path);
  if (!file.startsWith(siteRoot) || !existsSync(file)) { res.writeHead(404).end(); return; }
  // The fixtures name their other origins; these pages use none of them.
  const body = readFileSync(file, 'utf8').replace(/@XSITE@|@CROSS@/g, '');
  res.writeHead(200, { 'Content-Type': types[extname(file)] || 'application/octet-stream' }).end(body);
});
await new Promise((r) => site.listen(0, '127.0.0.1', r));
const LOCAL = `http://127.0.0.1:${site.address().port}`;
const pages = JSON.parse(readFileSync(pagesFile, 'utf8').replaceAll('@LOCAL@', LOCAL));

// ── Chrome ───────────────────────────────────────────────────────────────────
// Its password manager is off, as in a profile Manul launches: left on, it
// shows its own UI after a login, and that UI takes the clicks meant for the
// page — for both tools alike.
const profile = join(scratch, 'profile');
mkdirSync(join(profile, 'Default'), { recursive: true });
writeFileSync(join(profile, 'Default', 'Preferences'), JSON.stringify({
  credentials_enable_service: false, credentials_enable_autosignin: false,
  profile: { password_manager_enabled: false, password_manager_leak_detection: false },
  password_manager: { enabled: false },
}));
const chrome = spawn(chromePath, [
  `--remote-debugging-port=${PORT}`, `--user-data-dir=${profile}`,
  '--headless=new', '--window-size=1366,768', '--lang=en-US', '--accept-lang=en-US,en',
  '--no-first-run', '--no-default-browser-check', '--password-store=basic', 'about:blank',
], { stdio: 'ignore' });
for (let i = 0; i < 100; i++) {
  try { await fetch(`${CDP}/json/version`); break; } catch { await sleep(100); }
}

// ── Playwright MCP, over stdio, attached to the same Chrome ─────────────────
const mcp = spawn(process.execPath, [join(here, 'node_modules', '@playwright', 'mcp', 'cli.js'),
  '--cdp-endpoint', CDP, '--output-dir', join(scratch, 'mcp-output')],
{ stdio: ['pipe', 'pipe', 'ignore'], cwd: scratch });
let buf = '';
let nextId = 1;
const waiting = new Map();
mcp.stdout.on('data', (d) => {
  buf += d;
  let nl;
  while ((nl = buf.indexOf('\n')) >= 0) {
    const line = buf.slice(0, nl);
    buf = buf.slice(nl + 1);
    if (!line.trim()) continue;
    const msg = JSON.parse(line);
    if (msg.id && waiting.has(msg.id)) { waiting.get(msg.id)(msg); waiting.delete(msg.id); }
  }
});
const rpc = (method, params) => new Promise((resolve) => {
  const id = nextId++;
  waiting.set(id, resolve);
  mcp.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
});
await rpc('initialize', { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'token-bench', version: '1' } });
mcp.stdin.write(JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' }) + '\n');
// What the model reads of a tool call is its text content.
const tool = async (name, args) => {
  const r = await rpc('tools/call', { name, arguments: args });
  return (r.result?.content || []).filter((c) => c.type === 'text').map((c) => c.text).join('\n');
};

// ── Manul, as an agent calls it: one CLI command per step ───────────────────
// A step that fails answers on stdout too, and exits 1; that answer is what
// the model reads, so it is counted.
const manulRun = (args) => {
  try {
    return execFileSync(manul, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
  } catch (e) {
    return e.stdout || '';
  }
};

async function pageHTML(urlPart) {
  const list = await (await fetch(`${CDP}/json/list`)).json();
  const t = list.find((x) => x.type === 'page' && x.url.includes(urlPart));
  const ws = new WebSocket(t.webSocketDebuggerUrl);
  await new Promise((r) => ws.addEventListener('open', r, { once: true }));
  const reply = new Promise((r) => ws.addEventListener('message', (e) => r(JSON.parse(e.data)), { once: true }));
  ws.send(JSON.stringify({ id: 1, method: 'Runtime.evaluate', params: { expression: 'document.documentElement.outerHTML', returnByValue: true } }));
  const html = (await reply).result.result.value;
  ws.close();
  return html;
}

const escapeRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

// ── Seeing a page, and one click on it ──────────────────────────────────────
const rows = [];
for (const p of pages) {
  await tool('browser_navigate', { url: p.url });
  await sleep(p.settle ?? 1500);
  const tab = p.tab || new URL(p.url).host;
  const snapshot = await tool('browser_snapshot', {});
  const html = await pageHTML(tab);
  // map is a budgeted view: at most 8 elements a group by default, and it
  // reports how many it left out. Its whole, for a like-for-like comparison
  // with a snapshot that leaves nothing out:
  const map = manulRun(['map', '--cdp', CDP, '--tab', tab]);
  const mapFull = manulRun(['map', '--cdp', CDP, '--tab', tab, '--max-per-group', '100000']);
  const row = {
    page: p.name,
    url: p.url,
    rawHTML: tokens(html),
    playwrightSnapshot: tokens(snapshot),
    manulMap: tokens(map),
    manulMapFull: tokens(mapFull),
    manulMapTruncated: /"truncated":\s*[1-9]/.test(map),
  };
  if (p.click) {
    // The same click both ways, each from a freshly loaded page. Playwright
    // needs the element's ref from the snapshot; Manul needs its label.
    const ref = (snapshot.match(new RegExp(`${p.click.role} "${escapeRe(p.click.name)}[^"]*"[^\\n]*\\[ref=([a-z0-9]+)\\]`)) || [])[1];
    const click = ref ? await tool('browser_click', { element: p.click.name, target: ref }) : '';
    await tool('browser_navigate', { url: p.url });
    await sleep(p.settle ?? 1500);
    const step = manulRun(['run-step', `CLICK the '${p.click.name}' ${p.click.role}`, '--cdp', CDP, '--tab', tab]);
    Object.assign(row, {
      clickTarget: `${p.click.role} "${p.click.name}"`,
      playwrightClick: ref ? tokens(click) : null,
      manulStep: tokens(step),
      manulStepOK: /"ok": true/.test(step),
      targetInManulMap: map.toLowerCase().includes(p.click.name.toLowerCase()),
    });
  }
  rows.push(row);
  console.error(`${row.page}: snapshot ${row.playwrightSnapshot}, map ${row.manulMap}`);
}

// ── One task, end to end ────────────────────────────────────────────────────
// Sauce Demo: sign in, put the first product in the cart, open the cart and
// check the product is in it. Each tool gets its cheapest honest route: the
// Playwright agent takes a snapshot only when it needs refs it does not have,
// and checks with browser_wait_for rather than another snapshot; the Manul
// agent maps each new page once and checks with VERIFY. Both must end on the
// cart page with one item in it.
const cartCheck = "() => location.pathname + ' ' + document.querySelectorAll('.cart_item').length";

async function taskPlaywright() {
  const calls = [];
  const call = async (name, args) => { const t = await tool(name, args); calls.push({ tool: name, tokens: tokens(t) }); return t; };
  const ref = (snap, re) => {
    const m = snap.match(re);
    if (!m) throw new Error(`no ref matching ${re}`);
    return m[1];
  };
  await call('browser_navigate', { url: 'https://www.saucedemo.com/' });
  let snap = await call('browser_snapshot', {});
  await call('browser_type', { element: 'Username', target: ref(snap, /textbox "Username" \[ref=(\w+)\]/), text: 'standard_user' });
  await call('browser_type', { element: 'Password', target: ref(snap, /textbox "Password" \[ref=(\w+)\]/), text: 'secret_sauce' });
  await call('browser_click', { element: 'Login', target: ref(snap, /button "Login" \[ref=(\w+)\]/) });
  snap = await call('browser_snapshot', {});
  await call('browser_click', { element: 'Add to cart', target: ref(snap, /button "Add to cart" \[ref=(\w+)\]/) });
  // An action invalidates every ref ("Ref … not found in the current page
  // snapshot. Try capturing new snapshot."), so the next one needs a new
  // snapshot — or the file the action wrote, which costs the same to read.
  snap = await call('browser_snapshot', {});
  await call('browser_click', { element: 'Cart', target: ref(snap, /button "Cart[^"]*" \[ref=(\w+)\]/) });
  await call('browser_wait_for', { text: 'Sauce Labs Backpack' });
  const check = await tool('browser_evaluate', { function: cartCheck });
  return { calls, done: /cart\.html 1/.test(check) };
}

async function taskManul() {
  const calls = [];
  const tab = 'saucedemo';
  const step = (s) => {
    const out = manulRun(['run-step', s, '--cdp', CDP, '--tab', tab]);
    calls.push({ tool: `run-step ${s}`, tokens: tokens(out), ok: /"ok": true/.test(out) });
  };
  const map = () => calls.push({ tool: 'map', tokens: tokens(manulRun(['map', '--cdp', CDP, '--tab', tab])) });
  // Signed out, on the site; not counted — Manul's own NAVIGATE below is.
  await tool('browser_navigate', { url: 'https://www.saucedemo.com/' });
  await tool('browser_evaluate', { function: "() => { localStorage.clear(); sessionStorage.clear(); document.cookie.split(';').forEach((c) => { document.cookie = c.split('=')[0].trim() + '=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/'; }); }" });
  step('NAVIGATE to https://www.saucedemo.com/');
  map();
  step("FILL 'Username' field with 'standard_user'");
  step("FILL 'Password' field with 'secret_sauce'");
  step("CLICK the 'Login' button");
  map();
  step("CLICK the 'Add to cart' button");
  step("CLICK the 'Cart' button");
  step("VERIFY that 'Sauce Labs Backpack' is present");
  const check = await tool('browser_evaluate', { function: cartCheck });
  return { calls, done: calls.every((c) => c.ok !== false) && /cart\.html 1/.test(check) };
}

const task = {};
for (const [name, run] of [['playwright', taskPlaywright], ['manul', taskManul]]) {
  try {
    const t = await run();
    task[name] = { ...t, totalTokens: t.calls.reduce((n, c) => n + c.tokens, 0) };
  } catch (e) {
    task[name] = { error: String(e) };
  }
}

// ── Sent with every request, whatever the page: the tool definitions ───────
const defs = await rpc('tools/list', {});
const overhead = {
  playwrightTools: defs.result.tools.length,
  playwrightToolDefinitions: tokens(JSON.stringify(defs.result.tools)),
};

const versions = {
  date: new Date().toISOString().slice(0, 10),
  playwrightMcp: JSON.parse(readFileSync(join(here, 'node_modules', '@playwright', 'mcp', 'package.json'), 'utf8')).version,
  playwrightCore: JSON.parse(readFileSync(join(here, 'node_modules', 'playwright-core', 'package.json'), 'utf8')).version,
  manul: manulRun(['--version']).trim(),
  chrome: (await (await fetch(`${CDP}/json/version`)).json()).Browser,
  tokenizer: 'cl100k_base (js-tiktoken)',
};

console.log(JSON.stringify({ versions, pages: rows, task, overhead }, null, 2));
mcp.kill();
chrome.kill();
site.close();
process.exit(0);
