# Manul Node demo

One project that exercises everything the Node binding (`manul-browser` on npm)
and the engine behind it can do — against a small local shop site, so it runs
offline and gives the same result every time. It is the twin of
[`../python-demo`](../python-demo/README.md): same site, same hunts, same
handlers, written for Node. The two share nothing at run time.

```
site/               the application under test (static HTML + a tiny JSON API)
src/
  app-server.mjs    serves site/ and /api/* on http://localhost:8766
  controls.mjs      customControl()  — date picker, canvas, star widget, wildcard
  calls.mjs         call()           — CALL HOST / CALL GO handlers
  hooks.mjs         beforeAll / afterAll / beforeGroup / afterGroup
  handlers.mjs      import this to register all of the above
  pom/              page objects over a Session
hunts/              .hunt files, one per feature area
  lib/auth.hunt     reusable STEP blocks (@export) — page objects on the DSL side
manual/debug.hunt   PAUSE / DEBUG VARS, for --debug by hand
pages/              page-name registry (pages/<host>.json)
data/  mocks/  fixtures/
manul.config.json   config file (headless Chromium, timeouts, screenshots …)
manul_hooks.mjs     hook script for `manul run --hooks`
scripts/            01…09: the binding API, one area per script
test/               the same page objects and hunts under node:test
run-all.mjs         everything, in order
```

## Run it

Node 22 or newer. From npm — the engine arrives as a platform package:

```bash
npm install
```

```bash
npm run demo
```

From this checkout, against the engine and binding in this repository:

```bash
(cd ../../core && go build -o ../examples/node-demo/.bin/manul ./cmd/manul)
```

```bash
(cd ../../bindings/node && npm install && npm run build) && npm install --no-save ../../bindings/node
```

```bash
node run-all.mjs --all
```

The scripts pick up `.bin/manul` on their own; anywhere else set
`MANUL_BINARY`. `--firefox`, `--attach` and `--daemon` add the slower parts;
`--all` runs them all. Engine logs are hidden; `DEMO_ENGINE_LOG=1` shows them.
Each script also runs on its own: `node scripts/03-controls-and-calls.mjs`.

## What is covered where

### Node binding

| API | where |
|---|---|
| `Session.launch()`, `Session.open({ browser, headless, cwd, stderr, binary })` | `scripts/_common.mjs`, `07-firefox.mjs`, `06-errors.mjs` |
| `Session.attach(cdp)`, `open({ mode: 'attach', cdp })`, `MANUL_BROWSER_MODE` / `MANUL_CDP_ENDPOINT` | `08-attach.mjs` |
| `close()`, `closed`, `opened`, `await using` | `06-errors.mjs` |
| `step()` → `StepOutcome` (`ok`, `action`, `score`, `reason`, `near`, `error`) | `01-session-basics.mjs`, `src/pom/base.mjs` |
| `run({ source })`, `run({ path })` → `RunOutcome` | `01`, `04-run-hunts.mjs`, `src/pom/pages.mjs` |
| `runSuite([...])` → `SuiteResult` / `SuiteHunt` (`skipped`, `vars`) | `05-suite.mjs` |
| `map({ maxPerGroup, includeUnlabeled })` → `PageMap.labels()` | `01` |
| `read(label)` → `Value`, `readText(css, { maxChars })` | `01`, `src/pom/*` |
| `state()`, `vars(...names)`, `setVars({...})`, `schema()`, `transport.call()` | `01` |
| `engineVersion`, `protocol`, `mode`, `browser`, `cdp`, `published`, `publishHandlers()` | `01`, `03`, `08` |
| `customControl({ page, target }, fn)`, `customControl(target, fn)`, `ANY_PAGE`, `ctx.eval/url/vars/value/action` | `src/controls.mjs` |
| `call(name, fn)`, `ctx.args/vars/eval`, scalar / object / `null` results, throwing handlers | `src/calls.mjs`, `03` |
| `beforeAll`, `afterAll`, `beforeGroup(tag)`, `afterGroup(tag)`, `ctx.set/variables/metadata` | `src/hooks.mjs`, `05` |
| `listCustomControls`, `listCalls`, `listHooks`, `getCustomControl`, `getCall`, `getHooks`, `diagnoseCustomControlMiss` | `03` |
| `serveHooks()` | `manul_hooks.mjs` |
| `findBinary`, `EngineNotFound`, `ProtocolError`, `EngineError` (`code`), `SessionClosed`, `ManulError` | `06-errors.mjs` |

### DSL, config, CLI

The hunts are the Python demo's, feature for feature — see its README for the
table. `CALL GO` stands in for `CALL PYTHON` here; all three spellings are one
command. `scripts/09-cli.mjs` drives the package's own `manul` command
(`npx manul`) with `--hooks manul_hooks.mjs` and every run flag, plus `scan`,
`schema`, `pages`, `controls` and `daemon`; `08-attach.mjs` covers `manul map`,
`manul read` and `manul run-step`.

Not automated, because they wait for a person: `manul record <url>` and
`manul run manual/debug.hunt --debug`.

## Things worth knowing

* **A failing hunt rejects.** `session.run()` rejects with
  `EngineError` (`code === 'step_failed'`) rather than resolving `ok: false`;
  `runSuite` reports failures as data.
* **Engine logs go to this process's stderr by default** (`stderr: 'inherit'`).
  The demo passes `'ignore'`; there is no option to write them to a file.
* **`--hooks` with a `.mjs` script runs `$MANUL_NODE`**, else `node` from `PATH`.
* **MOCK lasts for the whole session**, so `11_mocking.hunt` runs last and
  `node:test` leaves it out.
* **Port 8766**, so this demo and the Python one (8765) can run side by side.
* **Needs engine 0.1.6**, like the Python demo.
