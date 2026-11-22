# Manul Python demo

One project that exercises everything the Python binding and the engine behind
it can do — against a small local shop site, so it runs offline and gives the
same result every time.

```
site/              the application under test (static HTML + a tiny JSON API)
demo/
  app_server.py    serves site/ and /api/* on http://localhost:8765
  controls.py      @manul.custom_control  — date picker, canvas, star widget, wildcard
  calls.py         @manul.call            — CALL HOST / CALL PYTHON handlers
  hooks.py         @before_all / @after_all / @before_group / @after_group
  handlers.py      import this to register all of the above
  pom/             Python page objects over manul.Session
hunts/             .hunt files, one per feature area
  lib/auth.hunt    reusable STEP blocks (@export) — page objects on the DSL side
manual/debug.hunt  PAUSE / DEBUG VARS, for --debug by hand
pages/             page-name registry (pages/<host>.json)
data/              @data: CSV and JSON
mocks/             MOCK response bodies
fixtures/          UPLOAD files
manul.config.json  config file (headless Chromium, timeouts, screenshots …)
manul_hooks.py     hook script for `manul run --hooks`
scripts/           01…09: the binding API, one area per script
tests/             the same page objects and hunts under pytest
run_all.py         everything, in order
```

## Run it

From PyPI — the wheel carries the engine:

```bash
python -m venv .venv && . .venv/bin/activate
pip install -r requirements.txt
python run_all.py
```

From this checkout, against an engine built from `core/`:

```bash
(cd ../../core && go build -o ../examples/python-demo/.bin/manul ./cmd/manul)
```

```bash
python -m venv .venv && MANUL_ALLOW_PURE_WHEEL=1 .venv/bin/pip install -e ../../bindings/python pytest
```

```bash
.venv/bin/python run_all.py --all
```

The scripts pick up `.bin/manul` on their own; anywhere else set
`MANUL_BINARY`. `--firefox`, `--attach` and `--daemon` add the slower parts;
`--all` runs them all. Each script also runs on its own:
`python scripts/03_controls_and_calls.py`.

## What is covered where

### Python binding

| API | where |
|---|---|
| `Session(...)` launch, `browser=`, `headless=`, `cwd=`, `stderr=`, `binary=` | `scripts/_common.py`, `07_firefox.py`, `06_errors.py` |
| `Session(mode="attach", cdp=…)`, `MANUL_BROWSER_MODE` / `MANUL_CDP_ENDPOINT` | `08_attach.py` |
| `open_now=False`, `open()`, `close()`, `closed` | `06_errors.py` |
| `step()` → `StepOutcome` (`ok`, `action`, `score`, `reason`, `near`, `error`) | `01_session_basics.py`, `demo/pom/base.py` |
| `run(source=…)`, `run(path=…)` → `RunOutcome` | `01`, `04_run_hunts.py`, `demo/pom/dashboard.py` |
| `run_suite([...])` → `SuiteResult` / `SuiteHunt` (`skipped`, `vars`) | `05_suite.py` |
| `map(max_per_group=, include_unlabeled=)` → `PageMap` / `MapGroup` / `MapElement` | `01` |
| `read(label)` → `Value`, `read_text(css, max_chars=)` | `01`, `demo/pom/*` |
| `state()`, `vars(*names)`, `set_vars(**kv)`, `schema()` | `01` |
| `engine_version`, `protocol`, `mode`, `browser`, `cdp`, `published` | `01`, `03`, `08` |
| `publish_handlers()` after registering on a live session | `03_controls_and_calls.py` |
| `@custom_control(page=, target=)`, `ANY_PAGE`, `ControlContext.eval/current_url/vars/value/action` | `demo/controls.py` |
| `@call(name)`, `CallContext.args/vars/eval`, scalar / dict / `None` results, raising handlers | `demo/calls.py`, `03` |
| `@before_all`, `@after_all`, `@before_group(tag)`, `@after_group(tag)`, `GlobalContext.set/variables/metadata` | `demo/hooks.py`, `05` |
| `list_custom_controls`, `list_calls`, `list_hooks`, `get_custom_control`, `get_call`, `diagnose_custom_control_miss` | `03` |
| `serve_hooks()` | `manul_hooks.py` |
| `find_binary`, `EngineNotFound`, `ProtocolError`, `EngineError` (`code`), `SessionClosed`, `ManulError` | `06_errors.py` |

### DSL (`hunts/`)

| feature | hunt |
|---|---|
| NAVIGATE, FILL, TYPE, CLICK, SELECT, CHECK, UNCHECK, radio, UPLOAD | `02_forms` |
| VERIFY present / NOT present / checked / enabled / disabled, `has value` / `has placeholder` | `02_forms`, `01_auth` |
| DRAG, HOVER, DOUBLE CLICK, RIGHT CLICK, PRESS (Enter, Escape, `Control+A ON`) | `03_widgets` |
| WAIT, WAIT FOR … to disappear / be visible, WAIT FOR SELECTOR | `03_widgets` |
| SCROLL DOWN / UP, SCROLL DOWN inside, shadow DOM, JS `alert()` | `03_widgets` |
| custom controls from a hunt (FILL / CLICK / SELECT on a widget) | `02_forms`, `03_widgets` |
| WAIT FOR RESPONSE, NEAR, FOR EACH, CALL HOST maths, backend check | `04_catalog` |
| MOCK | `11_mocking` |
| IF / ELIF / ELSE (text, `button … exists`, `==`, `contains`), WHILE, REPEAT, REPEAT … as, SET, PRINT, VERIFY SOFTLY, DEBUG VARS | `05_control_flow` |
| `@data:` CSV, `@data:` JSON | `06_data_driven`, `10_data_json` |
| ON HEADER, ON FOOTER, INSIDE, INSIDE … row with, HIGHLIGHT, SCREENSHOT, FULL SCAN, SCAN PAGE into, VERIFY VISUAL | `07_diagnostics` |
| `@import` (named, `as` alias), `@export`, USE, `@var`, `@script` aliases, `@tags`, `@context`, `@title` | `01_auth`, `lib/auth.hunt` |
| `[SETUP]` / `[TEARDOWN]`, CALL HOST / CALL PYTHON, `into {var}` | `01_auth` |
| `@schedule:` | `08_scheduled` (for `manul daemon`) |
| PAUSE, DEBUG VARS under `--debug` / `--break-lines` | `manual/debug.hunt` |

### Config, globals, page registry

* `manul.config.json` — read from the engine's working directory, which is why
  every session passes `cwd=ROOT`. Precedence: `Session(...)` › `MANUL_*` env ›
  the file › defaults.
* Global variables — `before_all` publishes `{base_url}`, `{signer}`,
  `{api_token}` to every hunt of a suite. A single `run()` is not a suite, so
  scripts seed the same names with `set_vars`.
* `pages/localhost_8765.json` — names pages for `auto_annotate`. Custom controls
  are **not** looked up through it: their `page` is the page's `document.title`.

### CLI (`scripts/09_cli.py`)

`manul run hunts/ --hooks manul_hooks.py` with `--tags`, `--headless`,
`--html-report`, `--screenshot`, `--jsonl`, `--json`, `--workers`, `--retries`,
`--explain`, `--disable-cache`; `manul run -` (stdin); `MANUL_AUTO_ANNOTATE`;
exit codes for FAIL and BROKEN; `manul scan`, `manul schema`, `manul pages`,
`manul controls list`, `manul daemon`. `08_attach.py` covers `manul map`,
`manul read` and `manul run-step` against a running Chrome.

Not automated, because they wait for a person: `manul record <url>` and
`manul run manual/debug.hunt --debug`. `OPEN APP` needs an Electron app and is
not part of the demo.

## Things worth knowing

These are what the demo ran into; each hunt or script says so where it matters.

* **A failing hunt raises.** `Session.run()` raises `EngineError(code="step_failed")`
  rather than returning `ok=False`; `run_suite` reports failures as data.
* **MOCK lasts for the whole session**, across hunts. `11_mocking.hunt` runs last
  for that reason, and pytest leaves it out.
* **WAIT FOR RESPONSE only sees responses that arrive after it starts.** The
  catalog API is deliberately slower than a CLICK step.
* **EXTRACT reads a control's value, or the text that contains the label.**
  Totals are readonly inputs with a `<label>`, so `EXTRACT the 'Order total'`
  gives `142.50` rather than `Order total: 142.50`.
* **`--hooks` with a `.py` script runs `$MANUL_PYTHON`**, else `python`/`python3`
  from `PATH` — set it to the virtualenv's interpreter.
* **On the CLI a hunt refused by `before_group` is BROKEN and exits 1.** Under
  `run_suite` it is `skipped`.
* **Needs engine 0.1.6.** Four things this demo exercises were broken in
  0.1.5: `INSIDE 'X' row with 'Y'`, `NEAR` a plain-text heading,
  `manul pages list` beside a `pages/` directory, and a Firefox left running
  after a launch its updater interrupted.
