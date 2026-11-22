// The application under test: a small local site with a JSON API.
//
// Everything the demo automates lives in site/. It is served from one origin so
// the hunts are deterministic and need no network, and so MOCK and
// WAIT FOR RESPONSE have a real API to intercept.
//
//   node src/app-server.mjs        # serve on http://localhost:8766

import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, join, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';

export const SITE_DIR = fileURLToPath(new URL('../site/', import.meta.url));
export const DEFAULT_PORT = Number(process.env.DEMO_PORT ?? 8766);

const PRODUCTS = [
  { name: 'Manul Plush', price: 24.5 },
  { name: 'Steppe Map', price: 12.0 },
  { name: 'Snow Boots', price: 89.99 },
];
const TYPES = { '.html': 'text/html', '.css': 'text/css', '.js': 'text/javascript', '.png': 'image/png' };

// A tiny in-memory backend. CALL HOST handlers reset and query it over HTTP,
// the way a real suite seeds and checks a test database.
const orders = [];

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function json(res, status, payload) {
  const body = JSON.stringify(payload);
  res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' });
  res.end(body);
}

async function handle(req, res) {
  const path = (req.url ?? '/').split('?')[0];
  if (req.method === 'GET' && path === '/api/products') {
    await sleep(1500); // slower than a CLICK step, so WAIT FOR RESPONSE sees it arrive
    return json(res, 200, PRODUCTS);
  }
  if (req.method === 'GET' && path === '/api/health') return json(res, 200, { status: 'ok' });
  if (req.method === 'GET' && path === '/api/orders') return json(res, 200, orders);
  if (req.method === 'POST') {
    let raw = '';
    for await (const chunk of req) raw += chunk;
    if (path === '/api/orders') {
      orders.push(JSON.parse(raw || '{}'));
      return json(res, 201, { id: orders.length });
    }
    if (path === '/api/reset') {
      orders.length = 0;
      return json(res, 200, { reset: true });
    }
    return json(res, 404, { error: 'not found' });
  }
  const file = normalize(join(SITE_DIR, path === '/' ? 'index.html' : path));
  if (!file.startsWith(SITE_DIR)) return json(res, 403, { error: 'forbidden' });
  try {
    const body = await readFile(file);
    res.writeHead(200, { 'Content-Type': TYPES[extname(file)] ?? 'application/octet-stream' });
    res.end(body);
  } catch {
    json(res, 404, { error: 'not found' });
  }
}

/** Start the site. Resolves to { baseUrl, close() }. */
export async function startAppServer(port = DEFAULT_PORT) {
  const server = createServer((req, res) => {
    handle(req, res).catch((err) => json(res, 500, { error: String(err) }));
  });
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', resolve);
  });
  return {
    baseUrl: `http://localhost:${server.address().port}`,
    close: () => new Promise((resolve) => {
      server.closeAllConnections();
      server.close(resolve);
    }),
  };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const app = await startAppServer();
  console.log(`serving ${SITE_DIR} on ${app.baseUrl} — Ctrl+C to stop`);
}
