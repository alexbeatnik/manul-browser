# manul-browser (Node)

**Browser automation in plain English — for humans and LLM agents.**

```bash
npm install manul-browser
```

The engine arrives with it: npm picks the one `@manul-browser/engine-<os>-<cpu>`
package that matches your machine. You need only a system Chrome or Firefox.

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

A launch starts Chromium. Ask for Firefox and the same script runs there, over
WebDriver BiDi instead of CDP:

```js
const s = await Session.launch({ browser: 'firefox', headless: true });
```

`MANUL_BROWSER=firefox` does the same from the environment.

## Page objects

`Page` is a page's name in `pages/` — a page file's `@page:` — and what a
person does there. The engine, not the class, decides whether the page is open:

```js
import { Page, Session } from 'manul-browser';

class Login extends Page {
  static pageName = 'Login';
  signIn(user, password) {
    // SignIn is a block of pages/login.hunt: the same action hunts USE.
    return this.use('SignIn', { user, password });
  }
}

const s = await Session.launch();
await s.setVars({ site: 'http://localhost:8000' });
await (await new Login(s).open()).signIn('ada', 'lovelace');
```

`open()` is `NAVIGATE to 'Login'` then `assertLoaded()` (`VERIFY that the page
is 'Login'`, which waits for the page's address and every `@expect` text);
`use(block, vars)` runs `USE Login.<block>`; `do(...steps)` throws
`StepFailed` with the engine's reason and the closest labels; `shouldSee`,
`shouldNotSee` and `valueOf(label)` are the usual checks. A static `path` with
a `baseUrl` opens a page `pages/` has no address for.

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
beforeAll((ctx) => {
  ctx.set('env', 'staging');        // same as ctx.variables.env = 'staging'
  ctx.metadata.started = Date.now(); // scratch space for later hooks; never reaches a hunt
});
```

`session.runSuite(paths)` runs hunts with these hooks applied and resolves to a
`SuiteResult` whose `vars` are the variables the hooks published. A hunt that a
`beforeGroup` hook refused is `skipped`, and the rest still run.

`session.run({ path })` is one hunt, not a suite: no suite hook fires, and a
hunt with a failing step rejects with `EngineError` (`code === 'step_failed'`)
instead of resolving `ok: false`.

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
