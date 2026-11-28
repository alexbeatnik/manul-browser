<p align="center">
    <img src="images/manul.png" alt="Manul Browser mascot" width="180" />
</p>

# Manul Browser — Engine

[![Status: Alpha](https://img.shields.io/badge/status-alpha-d97706)](#project-status)
[![Go](https://img.shields.io/badge/go-%3E%3D1.26-blue)](https://go.dev/)
[![License](https://img.shields.io/badge/license-Apache%202.0-green)](LICENSE)

**A deterministic automation runtime for both humans and LLM agents — the Go engine behind Manul Browser, and its only implementation.** Write (or generate) `.hunt` files in plain English; the engine resolves every element with deterministic DOM heuristics and drives the browser directly — Chrome over the Chrome DevTools Protocol (CDP), Firefox over WebDriver BiDi — with no Playwright, no selectors, no cloud APIs and no AI required.

The same runtime serves two drivers from one artifact:

- **Humans** author readable `.hunt` steps (`Click the 'Login' button`) — QA tests, RPA, synthetic monitors. No selectors to maintain.
- **LLM agents** drive it through JSON CLI commands (`manul map` / `run-step` / `read` / `schema`) that target elements by human label, never CSS/XPath.

This is the engine itself, and the only implementation of it: the `.hunt` DSL, the deterministic scorer, and the agent JSON shapes — built on a single dependency (`gorilla/websocket`), shipping as one static binary, with true goroutine-level parallelism and an embeddable in-process API (`pkg/agent`). Every other language drives this rather than reimplementing it.

### Built for agents — and it's measurably cheaper on tokens

An agent has to *see* a page before it can act. Playwright MCP shows it an accessibility snapshot of the whole page, and acts on elements through refs taken from it — refs that every action invalidates, so the next action needs a fresh snapshot. `manul map` emits a compact, landmark-grouped view of the labelled, interactive elements, and a step acts on a label, which does not go stale. `manul mcp` serves the same to Claude Desktop, Claude Code and any MCP client. Measured on the same Chrome with `cl100k_base` (2026-11-28, the task 2026-11-26; `@playwright/mcp` 0.0.86):

| | Playwright MCP | `manul mcp` | |
| --- | ---: | ---: | --- |
| Seeing a page — 8 pages, from a local fixture to Wikipedia and MDN | 258 – 60,583 | 29 – 3,290 | **1.7–18× fewer** (1.7–8.9× with no budget on `map`) |
| A whole task — sign in, add to cart, check the cart on Sauce Demo | 4,314 (10 calls) | 570 (9 calls) | **7.6× fewer** |
| Tool definitions and instructions, sent with every request | 4,484 | 1,322 | **3.4× fewer** |

A snapshot and a map do not hold the same thing — the snapshot also carries the page's text — and the method, the caveats and a script to rerun it all are in [Token efficiency](docs/token-efficiency.md).

### And it runs a suite faster than Playwright

The same 26 tests, 185 steps, written as `.hunt` files and as Playwright Test, against a local site in the same Chrome (2026-11-28, Playwright 1.66):

| | Playwright Test | `manul run` | |
| --- | ---: | ---: | --- |
| One worker, median of 5 | 40.9 s | 32.2 s | **0.8×** |
| Four workers, median of 3 | 16.8 s | 12.0 s | **0.7×** |

On a page that answers at once the two are about even; on one whose content arrives late Manul notices sooner. Until the actions stopped sleeping a fixed 150–500 ms after acting, the same suite took Manul 128 s. The method and the caveats are in [Speed](docs/speed.md).

Authoring is also leaner and far more durable: the same flow written as a `.hunt` file carries **zero CSS/XPath selectors** to break when the markup shifts.

> **Status: Alpha.** Solo-developed, actively battle-tested. Bugs are expected, APIs may evolve, and there are no promises about stability or production readiness. The core claim is transparency: when a step works, you can see exactly why; when it fails, you get the scoring breakdown to diagnose it.

> **📖 Full Documentation:** [Overview](docs/overview.md) · [Installation](docs/installation.md) · [Getting Started](docs/getting-started.md) · [DSL Syntax](docs/dsl-syntax.md) · [DSL for LLMs](docs/dsl-for-llms.md) · [Reports & Explainability](docs/reports.md) · [Integration](docs/integration.md) · [Extensions](docs/extensions.md) · [Loops & Page Objects](docs/loops-and-pages.md) · [Token Efficiency](docs/token-efficiency.md) · [Speed](docs/speed.md)

---

## Syntax First

Manul Browser runs `.hunt` files — plain-English automation scripts that read like manual QA steps. Here is the DSL in action.

### A complete flow

```text
@context: Smoke test for a login page
@title: Login Smoke
@var: {email} = admin@example.com
@var: {password} = secret123

STEP 1: Open the app
    NAVIGATE to https://example.com/login
    VERIFY that 'Sign In' is present

STEP 2: Authenticate
    FILL 'Email' field with '{email}'
    VERIFY "Email" field has value "{email}"
    FILL 'Password' field with '{password}'
    CLICK the 'Sign In' button
    VERIFY that 'Dashboard' is present

DONE.
```

Run it:

```bash
manul path/to/login.hunt
```

Every `@var:` is declared up front — never hardcode test data inside steps. `VERIFY` confirms state after every significant action. `DONE.` closes the mission.

> **Case-insensitive keywords.** All DSL keywords are case-insensitive at runtime — `CLICK`, `Click`, and `click` all work. The canonical form used in documentation and generated files is ALL UPPERCASE.
>
> **Element type hints are optional.** Words like `button`, `link`, `field`, `dropdown` placed after the target outside quotes are not required, but they provide a strong heuristic signal that boosts scoring accuracy. `CLICK the 'Login' button` and `CLICK the 'Login'` both work — the former is more precise.

### Conditional branching

Branch test logic with `IF` / `ELIF` / `ELSE` based on what the page actually contains. Nesting is supported.

```text
STEP 1: Adaptive login
    IF button 'SSO Login' exists:
        CLICK the 'SSO Login' button
        VERIFY that 'SSO Portal' is present
    ELIF text 'Sign In' is present:
        FILL 'Username' field with '{username}'
        CLICK the 'Sign In' button
    ELSE:
        CLICK the 'Create Account' link
```

Conditions can check element existence, visible text, variable equality, substring containment, or simple truthiness — all evaluated against the live page.

### Loops

Repeat actions with `REPEAT`, iterate data with `FOR EACH`, or poll dynamic state with `WHILE`. Loops nest freely with conditionals.

```text
@var: {products} = Laptop, Headphones, Mouse

STEP 1: Add products to cart
    FOR EACH {product} IN {products}:
        FILL 'Search' field with '{product}'
        PRESS Enter
        CLICK the 'Add to Cart' button NEAR '{product}'
        VERIFY that 'Added to cart' is present

STEP 2: Load all reviews
    WHILE button 'Load More' exists:
        CLICK the 'Load More' button
        WAIT 2

STEP 3: Retry checkout
    REPEAT 3 TIMES:
        CLICK the 'Place Order' button
        IF text 'Success' is present:
            VERIFY that 'Order confirmed' is present
```

`REPEAT N TIMES:` runs a fixed count. `FOR EACH {var} IN {collection}:` iterates comma-separated values. `WHILE <condition>:` repeats until the condition is false (safety limit: 100 iterations). `{i}` counter is auto-set on every iteration.

### Contextual navigation

When a page has repeating controls — multiple "Delete" buttons, "Edit" links in every row — use contextual qualifiers instead of brittle selectors.

```text
CLICK the 'Edit' button NEAR 'John Doe'
CLICK the 'Login' button ON HEADER
CLICK the 'Privacy Policy' link ON FOOTER
CLICK the 'Delete' button INSIDE 'Actions' row with 'John Doe'
```

`NEAR` ranks by pixel distance. `ON HEADER` / `ON FOOTER` scopes to viewport zones. `INSIDE` restricts scoring to a resolved row or container subtree.

### Variables, data-driven runs, and backend hooks

```text
@var: {email} = admin@example.com
@var: {password} = secret123
@script: {db} = db.Helpers
@data: users.csv
@tags: smoke, auth

[SETUP]
    CALL GO {db}.SeedUser "{email}" "{password}"
[END SETUP]

STEP 1: Login
    NAVIGATE to https://example.com/login
    FILL 'Email' field with '{email}'
    FILL 'Password' field with '{password}'
    CLICK the 'Sign In' button
    VERIFY that 'Dashboard' is present

STEP 2: Fetch and use an OTP
    CLICK the 'Send OTP' button
    CALL GO api.FetchOTP "{email}" into {otp}
    FILL 'OTP' field with '{otp}'
    CLICK the 'Verify' button
    VERIFY that 'Welcome' is present

[TEARDOWN]
    CALL GO {db}.CleanDatabase "{email}"
[END TEARDOWN]
```

`@data:` loops the entire mission over each row in a CSV or JSON file. `@tags:` lets you filter runs with `manul --tags smoke`. `[SETUP]` / `[TEARDOWN]` run Go outside the browser for data seeding and cleanup. `CALL GO ... into {var}` captures return values mid-test — ideal for OTPs, tokens, and backend state. `CALL GO` invokes Go functions registered in-process via `RegisterGoCall` (see [Embedding](#embedding-go-api)); `@script:` aliases a handler path.

### Explicit waits and strict assertions

```text
WAIT FOR 'Submit' to be visible
WAIT FOR 'Loading...' to disappear
WAIT FOR RESPONSE "/api/checkout"

VERIFY "Email" field has value "{email}"
VERIFY "Save" button has text "Save Changes"
VERIFY "Search" input has placeholder "Type to search..."
```

Actions wait on their own: a `CLICK`, `FILL`, `SELECT` or `EXTRACT` whose target is not on the page yet — products loaded after the page, a form that appears after a click — looks again until it is there or `timeout` (default 5s) passes, and a step that has already typed or clicked is never run twice. `WAIT FOR` is for waiting on something else, such as a spinner going away while the old content is still showing; like `VERIFY`, it polls the page on both engines. `WAIT FOR RESPONSE` blocks until a matching network response arrives. Strict assertions resolve the element through heuristics and compare exact text, value, or placeholder with `==`.

### Frames and tabs

```text
FILL 'Card number' field with '4242 4242 4242 4242'
CLICK the 'Pay now' button
CLICK the 'Open the product' link
CLICK the 'Add to cart' button
SWITCH TO TAB 1
```

Nothing to switch into for an iframe: same-origin, cross-origin and cross-site frames — a payment form, an embedded example, a widget in a shadow root — are read, filled, clicked, uploaded to and verified like the page itself. A tab a step opens (`target="_blank"`, `window.open`) is where the next step runs, and a popup that closes itself hands back to the tab that opened it. `SWITCH TO TAB <n>` or `SWITCH TO TAB '<title or URL>'` goes back to a tab that is still open.

### Page scanning for LLM agents

A full-page scan groups every interactive control on the page by its nearest semantic landmark (form, nav, header, dialog, section …) including Shadow DOM, and writes an annotated draft `.hunt`:

```bash
manul scan https://example.com --full
```

Designed for LLM-driven authoring — the grouped output is easy for a model (or a human) to read and trim into a real hunt. For live agentic perception, prefer `manul map` (below), which emits the same structure as token-lean JSON without writing a file.

### MCP server — Claude Desktop, Claude Code

```bash
claude mcp add manul-browser -- manul mcp  # Claude Code
```

```json
{ "mcpServers": { "manul-browser": { "command": "manul", "args": ["mcp"] } } }
```

The second is Claude Desktop's `claude_desktop_config.json`. `manul mcp` gives an MCP client six tools over stdio — `step`, `map`, `read`, `read_text`, `run_hunt`, and `suite_guide`, which tells Claude how to build a test suite on Manul — and a `build_test_suite` prompt. It opens the browser with the first call that needs it and closes it when the client goes away. Flags such as `--browser firefox`, `--headless`, `--user-data-dir` and `--cdp` go after `mcp`. See [Integration](docs/integration.md#mcp-server-claude-desktop-claude-code) and [`MANUL_MCP_CONTRACT.md`](../spec/contracts/MANUL_MCP_CONTRACT.md).

### Agent commands — drive the engine from an external LLM

For agentic use, Manul Browser exposes a small set of **JSON-emitting CLI commands** (the CLI face of the embeddable [`pkg/agent`](#embedding-go-api) API). They attach to an **already-running Chrome over CDP**, so an external model keeps one browser open and issues stateless calls against it. The JSON payload goes to **stdout**; all engine logs go to **stderr**, so a driver can pipe the output straight into a prompt.

```bash
# 1. start Chrome once with remote debugging
google-chrome --remote-debugging-port=9222 &

# 2. let the model see the page, act, and read — by human label, never CSS/XPath
manul schema                                       # DSL grammar + agent JSON shapes (no browser)
manul map --cdp http://127.0.0.1:9222              # compact landmark-grouped page map → JSON
manul run-step "Click the 'Login' button" --compact # run one instruction → step-outcome JSON
manul read 'Order total' --cdp http://127.0.0.1:9222 # read one labelled value → {value, found, reason}
manul read --selector '#cart' --cdp ...            # sanitized region text → {text, selector}
```

Shared flags: `--cdp <url>` (default `http://127.0.0.1:9222`) and `--tab <url-substr>` to pick a tab. `run-step --compact` returns a non-zero exit code when the step fails, and surfaces `near` candidates (with `0.0–1.0` scores) on a failed or low-confidence match so the agent can retarget without a re-scan. `manul schema` is the machine-readable contract a driver pins instead of stuffing full docs into every prompt.

### Shared libraries and scheduling

```text
@import: Login, Logout from lib/auth.hunt
@export: Checkout
@schedule: every 5 minutes

STEP 1: Setup
    USE Login

STEP 2: Purchase flow
    CLICK the 'Buy Now' button
    VERIFY that 'Order Confirmed' is present

STEP 3: Cleanup
    USE Logout

DONE.
```

`@import:` / `USE` reuses named STEP blocks across files. `@export:` controls visibility. `@schedule:` plus `manul daemon` turns any hunt into a recurring monitor or RPA job.

### CLI

```bash
manul path/to/hunts/                             # run all .hunt files in a directory
manul --headless path/to/file.hunt               # headless single file
manul --tags smoke path/to/hunts/                # filter by tags
manul --html-report --screenshot on-fail path/   # reports + failure screenshots
manul --explain path/to/file.hunt                # per-step scoring breakdown
manul --debug path/to/file.hunt                  # pause before every step
manul --workers 4 path/to/hunts/                 # run files in parallel
manul --browser firefox --locale uk-UA file.hunt # Firefox, asking pages for Ukrainian
manul scan https://example.com                   # scan a page → draft.hunt
manul daemon path/to/hunts/ --headless           # run scheduled hunts
```

---

## Philosophy

### Determinism, not prompt variance

The primary resolver is not an LLM. It is a weighted heuristic scorer (`pkg/scorer`) backed by an in-page JavaScript probe. Scores are normalized on a `0.0–1.0` confidence scale across four channels: `text`, `id`, `semantics`, and `proximity` (with an in-session cache signal). The result is repeatable: same page state plus same step text equals same resolution — no prompt variance, no cloud dependency.

When `--explain` is enabled, every resolved step prints the top-5 candidates with a per-channel breakdown so you can see exactly which signals drove the decision and which lost.

### Native CDP, no Playwright

Manul Browser talks to the browser through its **own** Chrome DevTools Protocol client — a thin WebSocket transport in [`pkg/cdp`](pkg/cdp) with a single external dependency (`gorilla/websocket`). There is no Playwright, no Selenium, no Node.js, and no bundled browser download: the engine launches the Chrome/Chromium you already have on `PATH` (or attaches to a running one) and drives it directly.

Why own the protocol layer:

- **One small dependency, fully inspectable.** The whole browser driver is a handful of readable Go packages (`pkg/cdp` transport, `pkg/browser` launcher, `pkg/dom` element model) rather than a large vendored toolchain. What the engine sends to Chrome is exactly what you can read.
- **Trusted input by default.** Clicks and keystrokes are dispatched via CDP `Input.*` events at real coordinates, and form values go through the native value setter so React/Vue/Angular state updates fire — no `force` hacks needed. `PRESS Enter` sends a real character event (`text:"\r"`) so forms actually submit.
- **Per-frame execution contexts.** Every element is resolved and acted on inside its own frame's execution context — same-origin, cross-origin, or a cross-site frame Chrome runs in a process of its own (attached as a session of its own); Firefox gets pointer actions and keys routed into the frame's context, because a click it dispatches from the top never reaches a cross-site frame. Frames are first-class, not an afterthought.
- **Two protocols, one engine.** CDP is Chrome's, so Firefox is driven over **WebDriver BiDi** instead (`--browser firefox`) — Firefox deprecated CDP in 129 and removed it, along with the `remote.active-protocols` preference, in 141. The scorer, the DSL and the in-page JavaScript (`pkg/pagejs`) are shared; only the transport differs, and `--cdp` picks it from the endpoint scheme (`http://…` CDP, `ws://…/session` BiDi). WebKit/Safari are not supported. Pick the concrete binary with `--channel` (`chrome`, `msedge`, `chromium`, `firefox-esr`, …) or `--executable-path`.

### True concurrency (goroutines)

Because there is no GIL and no single-threaded driver process, Manul Browser runs dozens of hunts in parallel using native goroutines. The `pkg/worker` package provides a `WorkerPool` with per-worker Chrome isolation, a `PortAllocator` for debug-port management, and race-detector-safe CDP transport — each worker owns its own `Runtime`, `Page`, and `ChromeProcess`. Run directory suites concurrently with `--workers N`, or embed the pool directly (see [Parallel Execution](#parallel-execution-go-api)).

### Dual-persona workflow

Manual QA writes plain-English `.hunt` steps — no code required. SDETs extend the same files with Go hooks (`[SETUP]` / `[TEARDOWN]`, `CALL GO`) and `RegisterCustomControl` handlers for complex widgets. Both personas work on the same artifact; the Go extensions are registered in-process before a run.

### No AI in the loop — fully deterministic

Manul Browser has **no LLM inside it**. Element resolution is 100% the deterministic scorer — same page state + same step ⇒ same result, every run, with no model to install, no temperature to pin, and no network calls. The *intelligence* lives in the external agent that drives the engine via the [agent commands](#agent-commands--drive-the-engine-from-an-external-llm) (`map` / `run-step` / `read` / `schema`); the runtime itself stays a predictable execution layer.

---

## Four Automation Pillars

The same runtime and the same DSL serve four use cases:

| Pillar | How |
|---|---|
| **QA / E2E testing** | Write plain-English flows, verify outcomes, attach HTML reports and screenshots. No selectors in the test source. |
| **RPA workflows** | Log into portals, fill forms, extract values, hand off to Go for backend or filesystem steps. |
| **Synthetic monitoring** | Pair `.hunt` files with `@schedule:` and `manul daemon` for recurring health checks. |
| **AI agent targets** | Constrained DSL execution is safer than raw CDP/scripting for external agents — the runtime still owns scoring, retries, and validation. |

---

## Key Features

- **Conditional branching & loops** — `IF` / `ELIF` / `ELSE` for adaptive flows; `REPEAT`, `FOR EACH`, `WHILE` for iterating data, retrying actions, and polling dynamic state. Full nesting support (`WHILE` capped at 100 iterations; `REPEAT` exposes a `{i}` 0-based counter).
- **Deterministic targeting** — 4-channel heuristic scorer ranks every candidate with explicit signal breakdowns; contextual qualifiers (`NEAR`, `ON HEADER/FOOTER`, `INSIDE`) restrict candidates spatially and structurally. Shadow DOM, 3-pass proximity resolution, and anti-phantom guards included.
- **Frames and tabs** — iframes of any origin, frames inside shadow roots, new tabs and popups followed automatically, `SWITCH TO TAB` to go back. See [Frames and tabs](#frames-and-tabs).
- **Waits for what is still loading** — an action whose target has not arrived yet looks again until `timeout`, never acting twice.
- **The same page on both engines** — Chrome and Firefox ask for the same language (`locale`, default `en-US`), and every hunt in `e2e/` runs on both in CI.
- **Explainability** — `--explain` prints top-5 candidate rankings with per-channel score breakdowns, and `explain-next` previews the scoring of a step before it runs.
- **Interactive debugger** — `--debug` pauses before every step with a browser modal UI; breakpoint lines (`--break-lines`); `explain-next` previews the scoring for the upcoming (or an overridden) step without executing it.
- **MCP server** — `manul mcp` serves the engine to Claude Desktop, Claude Code and any MCP client: six tools, plain-language answers, under a third of Playwright MCP's per-request overhead, and a guide that lets Claude build a whole test suite.
- **Embeddable agent API (`pkg/agent`)** — `agent.Session` owns Chrome and exposes compact, agent-friendly calls: `Read` (zero-scan), `ReadText`, `Step`/`Run` (typed `Reason` + `Near` candidates), `Map` (budgeted scan). CLI `read` / `run-step --compact` are thin wrappers over it.
  ```go
  import "github.com/alexbeatnik/manul-browser/core/pkg/agent"

  sess, _ := agent.Launch(ctx, agent.Options{Headless: true})
  defer sess.Close()
  sess.Step(ctx, "Click the 'Login' button")
  sess.Read(ctx, "Order total")
  ```
- **LLM-friendly rendering** — `PageMap.RenderForLLM` lists quoted, copy-pastable labels with the DSL verb per role (`'Search' [textbox — FILL]`); `DescribeForLLM` / `RenderForLLM` translate failures into plain-language advice; `DescribePageChange` says explicitly when the page did NOT change. Tuned so even small local models stay grounded.
- **Desktop / Electron** — Set `--executable-path` and use `OPEN APP` instead of `NAVIGATE` to drive Electron apps with the same DSL.
- **Custom controls** — `RegisterCustomControl(page, target, handler)` lets you handle complex widgets (datepickers, virtual tables, canvas elements) with raw CDP while the hunt file keeps a single readable step.
- **Page-name registry** — `pages/<site>.json` maps URLs to human-readable labels for reports; unknown URLs auto-populate as `Auto: domain/path` placeholders. See [docs/loops-and-pages.md](docs/loops-and-pages.md).
- **Reliable keyboard input** — `PRESS` dispatches real character events (Enter carries `text:"\r"` so forms submit); key names are normalized (`enter`/`esc`/`space`/`down` → DOM key values).
- **HTML reports** — `--html-report` writes one styled report per hunt (per data row for `@data:`). The Go API can also write an aggregate `index.html` (`report.GenerateIndex`); the CLI does not.
- **Single static binary** — `go build` produces one self-contained `manul` executable. No runtime, no interpreter, no `node_modules`; ideal for CI images and `scratch`/distroless containers.

---

## Architecture

```
cmd/manul           CLI entry point → produces `manul` binary
pkg/mcp             MCP server over stdio (`manul mcp`): six tools over pkg/agent, the suite guide
pkg/agent           Embedding facade: agent.Session over runtime/cdp/scorer —
                    Launch/Attach/Connect (owns Chrome), Read/ReadText/Step/Run/Map
pkg/cdp             Low-level CDP WebSocket transport and domain wrappers (Chromium)
pkg/bidi            Low-level WebDriver BiDi transport and command wrappers (Firefox)
pkg/pagejs          The in-page JavaScript both backends inject — written once
pkg/browser         Abstract browser/page interfaces + both backends + process lifecycle
pkg/runtime         Targeting pipeline: probe → filter → score → resolve;
                    DSL execution, control flow, variable management
pkg/worker          Worker / WorkerPool / PortAllocator for parallel execution
pkg/dom             Normalized DOM element model (ElementSnapshot with 37 fields)
pkg/heuristics      In-page JS probes (SnapshotProbe, VisibleTextProbe, ExtractDataProbe)
pkg/scorer          Deterministic 4-channel [0.0–1.0] scoring and ranking
pkg/dsl             .hunt file parser, import resolver, command AST with block nesting
pkg/explain         Structured execution results and explainability types
pkg/report          Styled per-hunt HTML reports; GenerateIndex for an aggregate
pkg/config          Runtime configuration
pkg/core            Shared enums (e.g. ScrollStrategy)
pkg/pages           Page-name registry: URL → human-readable label
pkg/scan            DOM scanner → draft .hunt; ScanPage (flat) + ScanPageFull (grouped)
pkg/utils           Semantic logging, ANSI stripping, error types
examples/           Sample .hunt files
docs/               Documentation
```

See [docs/overview.md](docs/overview.md) for the deep-dive architecture walkthrough.

---

## Quickstart

### Install

Requires Go ≥ 1.26 and a system-installed Google Chrome / Chromium on `PATH`.

```bash
# build a local binary
make build                  # or: go build -o manul ./cmd/manul

# install on PATH
make install                # user-local (~/.local/bin)
make install-system         # system-wide (/usr/local/bin)

# or straight from the module
go install github.com/alexbeatnik/manul-browser/core/cmd/manul@latest
```

### Configure

Create `manul.config.json` in the workspace root. All keys are optional:

```json
{
  "browser": "chromium",
  "headless": false
}
```

This is the minimal recommended config — fully heuristics-only, no AI dependency.

### Run

```bash
manul examples/login.hunt                        # single file
manul examples/                                  # all hunts in a directory
manul --headless --html-report examples/         # CI mode with reports
```

### Configuration reference

| Key | Default | Description |
|---|---|---|
| `headless` | `false` | Hide the browser window. |
| `browser` | `"chromium"` | `chromium` (launch system Chrome, CDP), `firefox` (launch system Firefox, WebDriver BiDi), or `electron` (deprecated spelling of `browser_mode: attach`). |
| `disable_cache` | `false` | Disable the in-session DOM snapshot cache (env: `MANUL_DISABLE_CACHE`, or the inverse `MANUL_SEMANTIC_CACHE_ENABLED`). |
| `timeout` | `5000` | How long a step waits, milliseconds: for a target that is not on the page yet, and for `VERIFY`. `0` turns waiting for a target off. |
| `nav_timeout` | `30000` | Navigation timeout, milliseconds. |
| `workers` | `1` | Max parallel hunt files. |
| `channel` | `null` | Chrome/Chromium binary to launch (`chrome`, `msedge`, `chromium`, …). |
| `executable_path` | `null` | Explicit path to a Chrome/Chromium (or Electron) executable. |
| `locale` | `"en-US"` | Language the launched browser uses and asks pages for (`navigator.language`, `Accept-Language`), the same on both engines. |
| `browser_args` | `[]` | Extra flags for the browser, after the engine's own: `["--no-sandbox"]` in a container, `["--window-size=1920,1080"]` for Chrome, `["-width", "1920", "-height", "1080"]` for Firefox. `MANUL_BROWSER_ARGS` takes them separated by spaces. |
| `retries` | `0` | Run a failed hunt file again, up to N times; one that passes on a retry is reported as flaky. |
| `screenshot` | `"on-fail"` | `none`, `on-fail`, or `always`. |
| `html_report` | `false` | Generate an HTML report. |
| `explain_mode` | `false` | Per-channel scoring breakdown in output. |
| `debug_mode` | `false` | Pause before every step for interactive stepping. |

Environment variables (`MANUL_HEADLESS`, `MANUL_BROWSER`, `MANUL_CHANNEL`, `MANUL_LOCALE`, `MANUL_WORKERS`, etc.) override JSON config; CLI flags override everything:

```
CLI flags  >  MANUL_* env vars  >  manul.config.json  >  config.Default()
```

### Binary distribution

`go build` yields a single static `manul` binary — no interpreter, no `node_modules`, no bundled browser. Drop it into a CI image (or a `scratch`/distroless container with system Chrome) and run headless:

```bash
go build -o manul ./cmd/manul
./manul --headless --html-report --screenshot on-fail hunts/
```

---

## Embedding (Go API)

To drive a browser from a Go program — an assistant, an agent, a custom tool — embed `pkg/agent`. Manul Browser owns the entire browser lifecycle; the consumer just calls a small, compact API and never touches CDP or the runtime directly:

```go
import "github.com/alexbeatnik/manul-browser/core/pkg/agent"

sess, err := agent.Connect(ctx, agent.Options{Port: 9222}) // attach if Chrome is up, else launch & own it
// or: agent.Launch(ctx, agent.Options{Headless: true})    // always spawn & own Chrome
// or: agent.Attach(ctx, "http://127.0.0.1:9222", "", ...) // use a specific running Chrome
defer sess.Close()                                          // reaps Chrome when this session launched it

out, _ := sess.Step(ctx, "Click the 'Login' button")  // compact: ok, reason, score, near[]
total, _ := sess.Read(ctx, "Order total")             // zero-scan targeted text
text, _ := sess.ReadText(ctx, "#answer")              // sanitized region/page text
pm, _ := sess.Map(ctx, agent.MapBudget{MaxPerGroup: 8}) // budgeted landmark map
res, _ := sess.Run(ctx, huntScript)                   // whole .hunt, compact aggregate
ps, _ := sess.PageState(ctx)                          // {Title, URL} snapshot
ans, _ := sess.Lookup(ctx, url, 3*time.Second, "")    // background-tab read (no UI disruption)

// Prompt-ready presentation — an embedding app needs zero browser code:
prompt := pm.RenderForLLM(5)            // page map: quoted labels + per-role DSL verb (CLICK/FILL/SELECT/CHECK)
report := res.RenderForLLM()            // whole run in plain language, with corrective advice per failed step
line := out.DescribeForLLM()            // one step outcome: what ran, why it failed, what to do next
diff := agent.DescribePageChange(before, ps) // ALWAYS speaks — incl. explicit "the page did NOT change"
```

Failures carry a machine-readable `Reason` (`not_found` / `ambiguous` / `timeout` / `verify_failed` / `action_failed`) and, for resolution problems, the top candidates in `Near` — so a caller branches on a typed reason instead of parsing error strings, and corrects a weak match without a follow-up scan.

`Session.Lookup` opens a URL in a **background tab** (the active page is never switched away from), waits for it to settle, runs an optional extractor JS, then closes the tab — so a consumer needs no CDP code to do unobtrusive web lookups in the user's logged-in profile. `agent.Connect` is the one-call lifecycle entry point: it attaches to a Chrome already listening on the debug port, or launches (and owns) one otherwise.

### Parallel Execution (Go API)

The `manul` CLI runs single-threaded unless you pass `--workers`. For full control over parallelism, embed the worker pool directly:

```go
import (
    "context"
    "github.com/alexbeatnik/manul-browser/core/pkg/config"
    "github.com/alexbeatnik/manul-browser/core/pkg/dsl"
    "github.com/alexbeatnik/manul-browser/core/pkg/report"
    "github.com/alexbeatnik/manul-browser/core/pkg/worker"
)

func runSuite(ctx context.Context, hunts []*dsl.Hunt) error {
    cfg := config.Default()
    // The last argument is the suite's GlobalContext, for suite hooks; nil without them.
    results, err := worker.RunHuntsInParallel(ctx, cfg, hunts, 4, logger, nil)
    if err != nil {
        return err
    }
    summaries := make([]report.RunSummary, len(results))
    for i, r := range results {
        summaries[i] = report.RunSummary{Result: r.Result, WorkerID: r.WorkerID}
    }
    _, _ = report.GenerateIndex(summaries, "reports")
    return nil
}
```

**Concurrency contract:** one `Runtime`, `Page`, and `ChromeProcess` per worker. Sharing them across goroutines is a data race — verified by `go test -race` in CI. Register all `CALL GO` handlers and custom controls **before** spawning the pool.

---

## Ecosystem

| Component | Role | Links |
|-----------|------|-------|
| **Python binding** | Thin client over this engine — ships the binary, speaks the stdio protocol. Not a second implementation. | [`bindings/python`](../bindings/python) · `manul-browser` on PyPI |
| **Node binding** | The same, for Node ≥ 22 — npm picks the engine package for the platform. | [`bindings/node`](../bindings/node) · `manul-browser` on npm |
| **Manul Browser** | This project — the Go runtime. Single static binary, goroutine parallelism, embeddable `pkg/agent`. | [GitHub](https://github.com/alexbeatnik/manul-browser) |

### Contributing and running tests

```bash
git clone https://github.com/alexbeatnik/manul-browser.git
cd manul-browser/core
# Requires Go ≥ 1.26 and a system-installed Google Chrome / Chromium on PATH.

go build ./...                                   # build everything
go test ./...                                    # full unit + synthetic suite
go test -race ./pkg/worker/...                   # concurrency safety
go test -tags e2e ./e2e/                         # every hunt in e2e/ on real Chrome and Firefox
```

### Development guides

- [**Scoring & Heuristics**](.claude/skills/scoring-heuristics/SKILL.md)
- [**Concurrency Rules**](.claude/skills/concurrency-rules/SKILL.md)
- [**Adding DSL Commands**](.claude/skills/adding-dsl-commands/SKILL.md)
- [**Go Calls & Extensions**](.claude/skills/extensions-and-go-calls/SKILL.md)
- [**Hunt Authoring**](.claude/skills/hunt-authoring/SKILL.md)

---

## Get Involved

Manul Browser is alpha-stage and solo-developed. If deterministic, explainable browser automation interests you:

- Build it: `go install github.com/alexbeatnik/manul-browser/core/cmd/manul@latest` (needs system Chrome/Chromium)
- File issues: [github.com/alexbeatnik/manul-browser/issues](https://github.com/alexbeatnik/manul-browser/issues)

---

## Project Status

**Alpha.** The core engine covers:

- 32+ DSL commands, full control flow (`IF`/`ELIF`/`ELSE`, `WHILE`, `REPEAT N TIMES`, `FOR EACH`)
- 4-channel deterministic scoring with contextual qualifiers (`NEAR`, `ON HEADER/FOOTER`, `INSIDE`)
- Shadow DOM support, 3-pass proximity resolution, anti-phantom guards
- Iframes of any origin, new tabs and popups, `SWITCH TO TAB`; actions that wait for a target still loading
- Chrome over CDP and Firefox over WebDriver BiDi, with a real-browser e2e suite for both in CI
- `[SETUP]` / `[TEARDOWN]` hook blocks with fail-fast setup and guaranteed teardown; `@script:` aliases for `CALL GO` handlers
- Import system (`@import:`, `USE`/`CALL` expansion) and the page-name registry (`pages/<site>.json`)
- HTML reporting, screenshots, debug mode, explain mode
- Native `WorkerPool` for parallel execution with per-worker Chrome isolation
- Embeddable `pkg/agent` API (`Read`/`ReadText`/`Step`/`Run`/`Map`) with typed failure reasons and plain-language rendering for LLM drivers
- Strongly-typed extension API (`CALL GO`, `RegisterCustomControl`); race-detector-safe CDP transport

**Version:** `0.1.6` — `manul --version` and the contracts report `0.1.6` (no prefix); the git module tag carries the `v` prefix Go requires: `go get github.com/alexbeatnik/manul-browser/core@v0.1.6`. The engine is a module in a subdirectory, so that request resolves to the tag `core/v0.1.6`, not `v0.1.6` — the plain `v0.1.6` tag belongs to the binary and wheel release.

## License

Apache-2.0.
