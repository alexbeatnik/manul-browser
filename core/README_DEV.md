<p align="center">
    <img src="images/manul.png" alt="Manul Browser mascot" width="160" />
</p>

# 😼 Manul Browser Engine 0.1.4 — Deterministic Web & Desktop Automation Runtime

> **Developer README.** The user-facing tour lives in [README.md](README.md); this file is the
> engineering manual: project structure, runtime architecture, extension points, configuration,
> testing, and release mechanics.

**Status: Alpha.** Solo-developed, battle-tested against synthetic DOM suites and real sites.
No stability promises; the core claim is transparency — every resolution is explainable.

---

## 📁 Project Structure

```
cmd/manul           CLI entry point → produces the `manul` binary
pkg/agent           Embedding facade: agent.Session (Launch/Attach/Connect,
                    Read/ReadText/Step/Run/Map, Lookup, PageState) + LLM renderers
pkg/cdp             CDP WebSocket transport + domain wrappers (per-frame contexts)
pkg/browser         Browser/Page interfaces, CDP backend, Chrome process lifecycle
pkg/runtime         DSL execution: probe → filter → score → resolve → act;
                    control flow, ScopedVariables, [SETUP]/[TEARDOWN], registries
pkg/worker          Worker / WorkerPool / PortAllocator (parallel directory runs)
pkg/dom             ElementSnapshot (37 normalized fields)
pkg/heuristics      In-page JS probes (snapshot, visible-text, xpath, extract, page-text)
pkg/scorer          Deterministic 4-channel scorer + contextual qualifiers
pkg/dsl             .hunt parser, imports/@script aliases, command AST
pkg/explain         ExecutionResult / HuntResult / candidate explainability types
pkg/report          Per-hunt HTML report + aggregate index.html + run_history.json
pkg/config          Config struct, JSON file + MANUL_* env + defaults
pkg/pages           Page-name registry (pages/<site>.json, auto-populate)
pkg/scan            manul scan: flat + --full landmark-grouped drafts
pkg/daemon          @schedule: watcher (manul daemon)
pkg/record          manul record: interaction recorder
pkg/utils           Semantic logger (Block/Action/Detail), error types
contracts/          Frozen public-surface contracts (MANUL_*_CONTRACT.md ×8 + extension)
docs/               User documentation
examples/           Sample .hunt files
.claude/skills/     Deep-dive engineering guides (scoring, concurrency, DSL, testing)
```

`AGENTS.md` is the long-form internals map for both humans and AI assistants working in this repo.

---

## 🏛️ Architecture — the engine as a runtime

One deterministic pipeline serves every consumer (CLI runs, agent commands, embedding API):

```
.hunt line ──► pkg/dsl (parse) ──► pkg/runtime (dispatch)
                                        │
                          snapshot probe (pkg/heuristics, JS)
                                        │
                          37-field ElementSnapshot (pkg/dom)
                                        │
                          4-channel scorer (pkg/scorer)
                                        │
                     threshold check ──► CDP action (pkg/cdp Input.*)
                                        │
                     ExecutionResult (pkg/explain) ──► report / StepOutcome
```

- **No LLM in the loop.** Resolution is 100% the deterministic scorer; same page + same step ⇒ same result.
- **Native CDP.** One external dependency (`gorilla/websocket`); trusted `Input.*` events at real
  coordinates; per-frame execution contexts for iframes/OOPIF.
- **True concurrency.** No GIL: `pkg/worker` runs whole hunts in parallel goroutines, one
  `Runtime`+`Page`+`ChromeProcess` per worker (`go test -race`-verified).

---

## ✨ Key Features (dev view)

- Full `.hunt` DSL: control flow (`IF/ELIF/ELSE`, `WHILE`≤100, `REPEAT` with `{i}`, `FOR EACH`),
  contextual qualifiers (`NEAR`, `ON HEADER/FOOTER`, `INSIDE`), waits, strict assertions
  (`VERIFY … has value|text|placeholder`), `MOCK`, `PRINT`, `SCREENSHOT`, `OPEN APP`.
- Agent surface: `manul schema | map | read | run-step` emit compact JSON on stdout (logs → stderr),
  attach to a running browser over `--cdp`
  (`failure_reasons`: `ok, not_found, ambiguous, timeout, verify_failed, action_failed`).
- Embedding API `pkg/agent`: `Connect/Launch/Attach → Session.{Step,Run,Read,ReadText,Map,Lookup,PageState}` +
  prompt-ready renderers (`RenderForLLM`, `DescribeForLLM`, `DescribePageChange`).
- Explainability: `--explain` prints top-5 per-channel rankings; debug wire protocol
  (`MANUL_DEBUG_PAUSE`/`MANUL_EXPLAIN_NEXT` NUL-markers) for non-TTY drivers.

### 🧹 [SETUP] / [TEARDOWN] hooks and inline `CALL GO`

```hunt
@script: {db} = testdata.Seed

[SETUP]
    CALL GO {db}.CreateUser "{email}" into {user_id}
[END SETUP]

STEP 1: …
    CALL GO api.FetchOTP "{email}" into {otp}

[TEARDOWN]
    CALL GO {db}.Cleanup "{email}"
[END TEARDOWN]
```

Handlers are **registered in-process before the run** (no filesystem imports):

```go
runtime.RegisterGoCall("api.FetchOTP", func(ctx context.Context, inv runtime.GoCallInvocation) (any, error) {
    return fetchOTP(inv.Args[0])
})
```

`[SETUP]` failure marks the mission `broken` and skips browser steps; `[TEARDOWN]` always runs
after a successful setup. Returned scalars land in the `into {var}` target; returned maps set
multiple variables.

### 📋 `@var:` declarations and `@script:` aliases

Five-level `ScopedVariables` precedence: **row > step > mission > global > import**.
`@script: {alias} = package.Func` aliases a registered `CALL GO` handler path.

### 🏷️ `@tags:` + `--tags` filter

`@tags: smoke, auth` in the header; `manul --tags smoke dir/` runs only matching hunts
(env: `MANUL_TAGS`).

### 🎛️ Custom Controls & the page registry

```go
runtime.RegisterCustomControl("Checkout Page", "React Datepicker",
    func(ctx context.Context, page browser.Page, inv runtime.CustomControlInvocation) error {
        // inv.ActionType / inv.Value / inv.Variables; drive the widget via page.EvalJS(...)
        return nil
    })
```

Page names come from `pages/<site>.json` (auto-populated `Auto: domain/path` placeholders;
longest-prefix site matching). `"*"` registers an any-page control. Registries are
`sync.RWMutex`-guarded package globals — register at process init, **never** while workers run
(`ResetRuntimeRegistries()` is test-only).

### 🐹 Public Go API (`pkg/agent`)

```go
sess, _ := agent.Connect(ctx, agent.Options{Port: 9222}) // attach or launch+own
defer sess.Close()
out, _ := sess.Step(ctx, "Click the 'Login' button")     // StepOutcome{OK, Reason, Score, Near}
res, _ := sess.Run(ctx, huntScript)                      // RunOutcome + per-step results
```

Failures carry a typed `Reason` and `Near` candidates — branch on values, not error strings.
One `Session` = one page = one goroutine; use `pkg/worker` for parallel suites.

### 🧠 Deterministic resolution — no LLM in the loop

Weights are (`cache 2.0 · semantics 0.60 · text 0.45 ·
attributes 0.25 · proximity 0.10`) and frozen by `contracts/MANUL_SCORING_CONTRACT.md` +
golden-number tests. Don't touch weights without bumping the contract.

---

## 💻 System Requirements

- **Go ≥ 1.26** (build only; the artifact is a single static binary)
- **Google Chrome / Chromium** on `PATH` (or `--executable-path`); CDP is Chromium-only by design
- Linux / macOS / Windows

---

## 🛠️ Installation

### From source (dev mode)

```bash
git clone https://github.com/alexbeatnik/manul-browser.git
cd manul-browser/core
make build            # → ./manul
make install          # → ~/.local/bin/manul
make install-system   # → /usr/local/bin/manul
```

### From the module

```bash
go install github.com/alexbeatnik/manul-browser/core/cmd/manul@latest
```

---

## ⚙️ Configuration (`manul.config.json`)

Read from the CWD; layering (highest → lowest): **CLI flags → `MANUL_*` env → JSON file → `config.Default()`**.

| Key | Default | Env | Description |
|---|---|---|---|
| `headless` | `false` | `MANUL_HEADLESS` | Hide the browser window. |
| `browser` | `"chromium"` | `MANUL_BROWSER` | `chromium` (launch) or `electron` (attach over CDP). |
| `browser_args` | `[]` | `MANUL_BROWSER_ARGS` | Extra Chrome launch flags. |
| `channel` | — | `MANUL_CHANNEL` | Binary channel: `chrome`, `msedge`, `chromium`, … |
| `executable_path` | — | `MANUL_EXECUTABLE_PATH` | Explicit Chrome/Electron binary. |
| `cdp_endpoint` | — | `MANUL_CDP_ENDPOINT` | Attach to a running Chrome instead of launching. |
| `timeout` | `5000` | `MANUL_TIMEOUT` | Action timeout (ms). |
| `nav_timeout` | `30000` | `MANUL_NAV_TIMEOUT` | Navigation timeout (ms). |
| `disable_cache` | `false` | `MANUL_DISABLE_CACHE` (inverse alias: `MANUL_SEMANTIC_CACHE_ENABLED`) | Disable in-session DOM snapshot cache. |
| `workers` | `1` | `MANUL_WORKERS` | Parallel hunt files (worker pool). |
| `retries` | `0` | `MANUL_RETRIES` | Retry failed steps N times. |
| `verify_max_retries` | `15` | `MANUL_VERIFY_MAX_RETRIES` | VERIFY re-poll budget. |
| `screenshot` | `"on-fail"` | `MANUL_SCREENSHOT` | `none` / `on-fail` / `always`. |
| `html_report` | `false` | `MANUL_HTML_REPORT` | Generate HTML report. |
| `explain_mode` | `false` | `MANUL_EXPLAIN` | Per-channel scoring output. |
| `debug_mode` | `false` | `MANUL_DEBUG` | Pause before every step. |
| `break_lines` | `[]` | — | 1-based breakpoint lines (with debug). |
| `tags` | `[]` | `MANUL_TAGS` | Tag filter. |
| `tests_home` | `"tests"` | `MANUL_TESTS_HOME` | Default output dir for new hunts/scans. |

---

## 🖥️ CLI Usage

```bash
manul file.hunt | dir/ | .            # run (implicit `run` subcommand); `-` reads stdin
manul --headless --html-report dir/   # CI mode
manul --tags smoke --retries 2 dir/   # filter + retry
manul --workers 4 dir/                # parallel pool
manul --debug --break-lines 12,20 f.hunt
manul --explain f.hunt
manul --cdp http://127.0.0.1:9222 --target 'url=app.local' f.hunt

manul scan <URL> [--full] [--output draft.hunt]
manul record <URL> [output.hunt]
manul daemon dir/ --headless          # @schedule: watcher
manul pages [list|migrate]
manul controls list

# agent commands (JSON on stdout, logs on stderr)
manul schema
manul map        [--cdp …] [--tab s] [--max-per-group 8] [--include-unlabeled]
manul read 'Lbl' [--cdp …] [--selector css] [--max-chars n]
manul run-step "Click the 'Login' button" [--cdp …] [--compact]
```

Flags may appear before or after positionals (interleaved parsing, same as the Python CLI).
Exit codes: `0` success, non-zero on any failed hunt/step.

---

## 🧪 Tests

```bash
go test ./...                 # full unit + synthetic suite (21 packages)
go test -race ./pkg/worker/   # concurrency contract
go vet ./...
```

Scoring golden numbers live in `pkg/scorer` tests and must stay identical to the Python
engine's (`scoring_math`). Deep-dive guides: [.claude/skills/](.claude/skills/) —
scoring-heuristics, concurrency-rules, adding-dsl-commands, extensions-and-go-calls,
testing-manul-browser, hunt-authoring.

**Adding a DSL command:** parser case in `pkg/dsl/parser.go` (+`CommandType`), dispatch in
`pkg/runtime/runtime.go:executeCommand`, tests in both packages, and — if it's public surface —
the DSL contract + `manul schema` verbs list.

---

## 🔖 Version Bump

`const version` in `cmd/manul/main.go` is the single source of truth (reported by
`manul --version` and the agent schema, **no `v` prefix**). Bump it together with:
the git tag (`v0.1.4` — Go needs the prefix), README badges/notes, and the
`"version"` field in every `contracts/MANUL_*_CONTRACT.md`. Keep it in lockstep with the
binding versions in `bindings/python/manul/__init__.py` and `bindings/node/package.json`.

Pushing the bump to `main` releases everything. `release.yml` builds the binaries,
creates the GitHub Release and tags `core/vX.Y.Z`; `publish.yml` builds the PyPI and npm
packages with `bindings/build-packages.sh`, smoke-tests each on its own platform, and
uploads them. **That upload cannot be undone** — bump the version when the code on `main`
is the code to ship. A push that leaves the version alone publishes nothing.

---

## 📜 Release Notes: 0.1.4

Two changes, to how Chromium is started and to how the binaries are built. Nothing about
hunts or their results moves.

- **Chromium picks its own debugging port.** A launch used to pass
  `--remote-debugging-port=9222`; it now passes port `0`, Chrome takes a free port, and
  the engine reads the one it chose from `DevToolsActivePort` in the profile directory.
  Security tools flag a browser started on a fixed debugging port with a separate
  profile — it is how cookies get stolen over CDP — and one quarantined `manul.exe` for
  exactly that. A launch no longer has that shape. A port that is set explicitly
  (`agent.Options.Port`, the `port` argument of the protocol's `open`) is still passed as
  given.
- **Parallel Chromium workers** each ask for a free port too, instead of taking
  9222, 9223, … from the allocator.
- **Firefox is unchanged**: 9222 by default, and the allocated port for each parallel
  worker.
- **`manul run`** reports the endpoint the browser came up on once it is up
  (`Launched chromium (http://127.0.0.1:41873, …)`) instead of announcing a port
  beforehand.
- **Release binaries keep their symbol table.** They were built with `-ldflags "-s -w"`;
  one VirusTotal engine called the stripped Windows binary malware, and the same code
  built without those flags scans clean. Each binary is about 3 MB larger for it.

**Changes to expect:** a launched Chromium is no longer at `http://127.0.0.1:9222`, so
anything that assumed that address has to take the endpoint from the session, or set the
port explicitly. `agent.Connect` with no port still probes 9222 for a running Chrome
before launching, and will not find one that an earlier `Connect` launched.

## 📜 Release Notes: 0.1.3

A release from driving the engine against real sites in both browsers: values can be
read back out of fields, two ways to hang a session are gone, and several more steps
that passed over the wrong thing now fail.

- **A field can be read back.** `EXTRACT` and `read` looked at text only, so a filled
  input came back as its own label. A target that names a form control — by label,
  placeholder, aria-label, name, id or test id, or by a caption just before it — now
  reads what the control holds; a `<select>` answers with the option it shows. An empty
  field is found and empty. `VERIFY … has value` / `has placeholder` no longer resolve
  the `<label>` in place of the field, and can see a disabled one.
- **JavaScript dialogs are accepted**, on both backends. An `alert` used to hang a
  Chromium session for good; Firefox dismissed dialogs, so `confirm` answered Cancel.
- **A covered element is not clicked.** A click goes to coordinates; when something else
  is on top there — an open date picker, a modal backdrop — CLICK, DOUBLE CLICK and RIGHT
  CLICK wait briefly for it to clear and then fail the step naming it.
- **`MOCK` is request interception** (Fetch domain over CDP, network intercepts over
  BiDi) instead of a patch over `window.fetch`: the pattern matches the end of the URL,
  `*` is a wildcard, any kind of request is covered, and a rule survives navigation.
- **`nav_timeout` is applied.** It was declared and read by nothing. `NAVIGATE` fails
  when it runs out on a document that is not parsed yet, and carries on with a warning
  when it is; `WAIT FOR RESPONSE` waits `nav_timeout` rather than the step timeout.
  `NAVIGATE` to a host that does not resolve now fails in Chromium as it did in Firefox.
- **`UPLOAD` works over CDP**, takes a path relative to the hunt, and reports a missing
  file itself. A relative path used to make Chrome stop answering.
- **Also fixed:** `WAIT FOR … to disappear` when another sentence shares a word with the
  target; `CHECK` through a caption not bound to its box, and its retry ticking
  unrelated boxes; `FOR EACH` over a collection declared with `@var:`; `read` with a
  selector that matches nothing returning the whole page.
- **`map`, `FULL SCAN` and `SCAN PAGE`** name a control after its bound `<label>`, not
  its `name` attribute.
- **Browsers:** headless Chrome gets a 1366×768 window, as headless Firefox has, and
  treats its page as focused. On Windows a launched browser dies with the engine however
  the engine ends.
- **Debugging:** a paused run answers `vars` with the hunt's variables (debug contract
  0.2.1).

**Changes to expect:** hunts that passed by clicking a covered element or waiting on a
partial word match now fail with an explanation; `confirm` answers OK in Firefox; headless
Chrome lays pages out at desktop width, which invalidates `VERIFY VISUAL` baselines.

## 📜 Release Notes: 0.1.2

A correctness release: steps that passed over the wrong thing now fail, and
several documented features that never ran now do.

- **A target that is not on the page is `not_found`.** A type hint and a matching tag
  outscored the confidence bar with no text in common, so `CLICK the 'Delete account'
  button` clicked whichever button came first. Action commands now require the winner to
  match the target somewhere (`scorer.MatchesQuery`); a partial match still resolves.
  The same holds for a custom dropdown's option, DRAG, HIGHLIGHT and `VERIFY … has value`.
- **Quoted labels are opaque to the parser.** `near`, `on`, `inside`, `with`, `into`,
  `from`, `for`, `to be`, `and`, `is not`, `has text` and the type-hint words no longer
  split a command when they appear inside the quotes (`FILL 'Pay with card' field with …`).
- **Now working as documented:** named `@import` + `USE`; `ELSE`/`ELIF` inside `[SETUP]` /
  `[TEARDOWN]`; state checks on a disabled element (`is disabled`, a read-only checkbox);
  `PRESS Control+A` and `PRESS … ON '<target>'`; `IF … is NOT present`; `REPEAT N TIMES as
  {n}`; trailing `# comments`; numbered lines; `DEBUG` and `CHOOSE`; FILL into a
  `contenteditable`; `--tags`; `run-step --tab`.
- **New:** `--retries` (a pass on retry is `flaky`: result fields `flaky`/`attempts`,
  `run_history.json` status `flaky`); `--screenshot on-fail|always|none` (PNG files under
  `screenshots/`, path in `screenshot_path` — **`on-fail` is the default**, so failed
  steps now leave a file); `VERIFY VISUAL '<element>'` against a baseline in
  `visual_baselines/`; `@data:` in parallel runs, sessions and the daemon;
  `agent.Session.RunFile`; `browser` on `open`, in its result, and in both bindings.
- **Runs:** an empty `@data:` file no longer means zero runs and a pass; parallel workers
  start each hunt on a fresh runtime and see what `before_group` published; a CLI flag
  overrides config and env only when it was actually passed; `--debug` forces one worker.
- **Firefox:** focus events fire in a launched browser (the startup tab is replaced by one
  whose content holds focus).
- **Daemon:** weekly schedules fire on the day they name (they ran a day early); scheduled
  hunts get their imports expanded; overlapping runs no longer share one browser.
- **Values:** numbers from JSON data files and handler results keep their spelling
  (`1234567`, not `1.234567e+06`); `$i` no longer rewrites `$items`.
- **Bindings:** Python hook scripts read UTF-8 on Windows; a handler result JSON cannot
  carry fails its own step instead of hanging the session.
- **Packaging:** `bindings/build-packages.sh` builds the wheels, the sdist and the npm
  packages (`manul-browser` plus one `@manul-browser/engine-<os>-<cpu>` per target);
  `publish.yml` smoke-tests and uploads them when a new version reaches `main`. An install from the
  sdist now works — without an engine, as documented — instead of failing to build.

## 📜 Release Notes: 0.1.1

- **Firefox**, driven over **WebDriver BiDi** (`--browser firefox`, `MANUL_BROWSER=firefox`,
  or `browser` in the `open` protocol args). Not CDP: Firefox deprecated that in 129 and
  removed it, and `remote.active-protocols` with it, in 141. New packages `pkg/bidi`
  (protocol client) and `pkg/browser/bidi_backend.go` (the `Page` implementation);
  `--cdp ws://…` attaches to a Firefox somebody else started.
- **`pkg/pagejs`** holds the in-page JavaScript both backends inject, so FILL, CHECK,
  SCROLL, HIGHLIGHT and the coordinate probes cannot drift apart per protocol.
- `browser.Launch`/`browser.Connect` pick engine and protocol; an unsupported `--browser`
  value now fails loudly instead of silently launching Chrome.
- **Breaking:** the JSON config file is `manul.config.json`. The old
  The old `*_configuration.json` name is not read — rename yours.
- The project is **Manul Browser** throughout; the old engine name is retired.
  `EXTENSION_ENGINE_CONTRACT.md` went with it — the VS Code extension is a separate
  product, and the debug protocol it used is specified in `MANUL_DEBUG_CONTRACT.md`.

## 📜 Release Notes: 0.1.0

- **Rebrand:** module path now `github.com/alexbeatnik/manul-browser/core`.
- **DSL surface**: (`PRINT`, `SCREENSHOT`,
  `OPEN APP`, END-terminators), identical agent JSON (`schema`/`map`/`read`/`run-step`,
  `failure_reasons`, `step_outcome.score`, `editable` in `map`), identical CLI flags
  (`--workers`, `--channel`, `--html-report` default off, `--disable-cache`, `--target`),
  `run_history.json` byte-shape, full contracts set (9 files) vendored in-repo.
- `VERIFY '<label>' has value|text|placeholder` attribute form implemented at runtime.
- Interleaved CLI flag parsing (flags before or after positionals) across all subcommands.
- Chrome password-manager/leak-detection UI disabled at launch (clean automation runs).

Apache-2.0.
