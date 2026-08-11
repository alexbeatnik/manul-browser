# manul-browser (Node)

**Browser automation in plain English — for humans and LLM agents.**

> **Not on npm yet.** The release pipeline that would build the platform
> packages is switched off while the repository is put in order. Until then this
> package finds the engine through `$MANUL_BINARY` or `manul` on `PATH`.

A thin client for the Manul engine. It starts `manul serve --stdio` and speaks
the protocol in [`spec/protocol.md`](../../spec/protocol.md). It contains no
element scoring, no DSL parsing and no CDP — if something is missing here, the
fix is a protocol command in the engine, not an implementation in this package.

Zero runtime dependencies. Node ≥ 22, ESM, typed.

## Use it

```js
import { Session } from 'manul-browser';

const s = await Session.launch({ headless: true });
try {
  await s.step('NAVIGATE to https://example.com');
  await s.step("CLICK the 'Sign in' button");
  console.log((await s.map()).labels());
} finally {
  await s.close();
}
```

`Session` mirrors Go's `agent.Session` and Python's `manul.Session` method for
method — `step`, `run`, `runSuite`, `map`, `read`, `state`, `vars`, `close`.
Same names, same meanings, same results.

Attaching to a Chrome that is already running leaves it open afterwards, because
Manul did not open it:

```js
const s = await Session.attach('http://127.0.0.1:9222');
```

## Extensions

Handlers describe *this process*, so they are registered at module scope and
published to the engine when a session opens.

```js
import { call, customControl, beforeAll } from 'manul-browser';

// Reachable from a hunt as: CALL HOST auth.token into {token}
call('auth.token', async (ctx) => fetchToken(ctx.args[0]));

// Intercepts a step aimed at this element, before DOM resolution.
customControl({ page: '*', target: 'Cookie Consent' }, async (ctx) => {
  await ctx.eval("document.querySelector('#accept')?.click()");
});

// Runs once for a whole suite; what it publishes becomes {placeholders}.
beforeAll((ctx) => { ctx.variables.env = 'staging'; });
```

Inside a handler the engine is paused mid-step, so only `ctx.eval` and
`ctx.url` are available — anything else would re-enter the step that is running.

## Hook scripts

The same handlers work from the CLI, without writing a driver:

```js
// manul_hooks.mjs
import { beforeAll, serveHooks } from 'manul-browser';

beforeAll(async (ctx) => { ctx.variables.token = await getToken(); });

await serveHooks();   // blocks until the run ends; must be last
```

```bash
manul run hunts/ --hooks manul_hooks.mjs
```

The engine spawns the script and calls back into it. **Its stdout is the
protocol** — `console.log` is redirected to stderr for you, so printing is safe.

## Escape hatch

The engine gains protocol commands faster than this package wraps them.
`session.transport.call(cmd, args)` sends one directly; nothing interprets the
result.

## Develop

```bash
npm install
npm test          # against a fake engine — no Chrome, no binary needed
npm run build
```

The tests drive a fake engine that speaks the protocol, so they need neither
Chrome nor a compiled engine and finish in about a second. Anything that
genuinely needs a browser has to be run by hand.
