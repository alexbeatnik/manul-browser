# Manul stdio session protocol

Status: **implemented** — `manul serve --stdio`, in `core/pkg/serve`.

This is the wire contract between the `manul` binary and the language bindings
in `bindings/`. It is the only thing a binding is allowed to depend on; bindings
never reimplement scoring, DSL parsing, or CDP.

## Why this exists

The agent commands (`map`, `read`, `run-step`, `schema`) already speak JSON, and
they already externalise browser state — each one attaches to an
*already-running* Chrome over CDP, so the browser survives between processes.
That covers a surprising amount, but three things do not survive a process
boundary:

1. **DSL variables.** `EXTRACT the 'Order total' into {total}` writes into
   runtime state owned by the process. The next `manul run-step` is a fresh
   process and cannot see `{total}`. Any multi-step flow that carries a value
   forward is currently impossible through the agent CLI.
2. **The scorer's cache channel.** Resolution is deliberately cache-first; a
   cold process throws that away and re-probes the DOM every step.
3. **Process + CDP attach cost**, paid per step rather than per session.

`serve` keeps one process alive for the length of a session, so all three become
session-scoped rather than command-scoped.

## Transport

```
manul serve --stdio
```

- **stdout** carries the protocol and nothing else. One JSON object per line,
  newline-delimited (NDJSON), UTF-8, no trailing commas, no pretty-printing.
- **stderr** carries human logs. Never parsed. This matches the existing agent
  command convention ("payload on stdout, logs on stderr").
- The session ends when stdin closes or `close` is acknowledged.

A future `--socket <path>` transport may be added for callers that cannot hold a
child process; the framing below does not change.

## Framing

**Request** — every request carries a caller-chosen `id` unique within the session:

```json
{"id":1,"cmd":"run-step","args":{"step":"Click the 'Login' button"}}
```

**Response** — correlated by `id`:

```json
{"id":1,"ok":true,"result":{"action":"click","reason":"resolved","score":0.91}}
{"id":1,"ok":false,"error":{"code":"not_found","message":"no candidate above threshold","near":[…]}}
```

`ok:false` is a *step* failure, not a protocol failure — the session stays
usable. Protocol-level faults (unparseable line, unknown `cmd`) answer with
`code:"bad_request"` and, if no `id` could be recovered, `"id":null`.

**Event** — unsolicited, no `id`:

```json
{"event":"ready","protocol":"1.0","engine":"0.1.1"}
{"event":"log","level":"warn","message":"tab navigated mid-step"}
```

`ready` is always the first line the server writes. A binding that does not see
it must treat the process as failed rather than hanging.

## Ordering

The server executes commands **serially per session** and replies in the order
received. Callers may pipeline. Correlate by `id` regardless — this rule is a
convenience, not a licence for bindings to assume positional replies.

## Commands

Every agent command keeps the argument names and result shape it already has in
`spec/contracts/MANUL_CLI_CONTRACT.md`; `serve` is a transport, not a redesign.

| `cmd`       | args                                              | result |
|-------------|---------------------------------------------------|--------|
| `schema`    | —                                                 | engine schema (same payload as `manul schema`) |
| `open`      | per-session config overrides (see below)          | `{mode,cdp,url}` — opens the browser session |
| `register`  | `controls?`, `calls?`, `hooks?`                    | `{controls,calls,hooks}` — counts accepted |
| `map`       | `maxPerGroup?`, `includeUnlabeled?`               | page map `{url, groups:[…]}` |
| `read`      | `label?`, `selector?`, `maxChars?`                | `{value,found,reason}` or `{text,selector}` |
| `run-step`  | `step`                                            | `StepOutcome` |
| `run`       | `path` or `source`                                | `RunOutcome` for a whole `.hunt` |
| `run-suite` | `paths` (array of `.hunt` files)                  | `SuiteResult` — per-hunt outcomes with the suite lifecycle applied |
| `vars`      | `set?` (object), `get?` (array of names)          | current variable map |
| `state`     | —                                                 | `{title,url}` |
| `close`     | —                                                 | `{}`, then the server exits |

Calling `open` twice in a session is `already_open`.

Error codes: `bad_request`, `not_open`, `already_open`, `step_failed`,
`internal`.

**A malformed request outranks a missing session.** `map` with a bad argument
answers `bad_request` even before `open` has been called — telling the caller to
open a browser first, only for them to hit the same argument error afterwards,
would waste a browser launch on a request that was never going to work.

**A step that resolves nothing is not a transport failure.** `run-step` answers
`ok: true` carrying a `StepOutcome` whose own `ok` is `false`. Not finding an
element is something an agent reacts to; the session stays healthy.

A leading UTF-8 BOM on a request line is ignored — shell pipelines and Windows
editors add one, and rejecting it would only ever look like a client bug.

### Launch a new browser, or attach to a running one

This is **configuration, not a command**. `open` takes no mode argument of its
own; it reads the resolved config, so a caller that has set the config correctly
just calls `open` with no args. Bindings do not get their own way of expressing
this — there is one answer per session and it comes from config.

Today the decision is spread across two keys that can disagree: `cdp_endpoint`
(set ⇒ attach) and `browser` (`electron` ⇒ attach, `chromium` ⇒ launch). Both
encode the same bit, so `browser: chromium` + `cdp_endpoint: …` is ambiguous.
This protocol requires one explicit key instead:

| key            | env                  | CLI                    | values |
|----------------|----------------------|------------------------|--------|
| `browser_mode` | `MANUL_BROWSER_MODE` | `--attach` / `--launch`| `launch` (default) · `attach` |

- **`launch`** — the engine starts a fresh browser and owns its lifetime; it is
  closed when the session closes. `browser`, `channel`, `executable_path`,
  `browser_args`, `headless` apply.
- **`attach`** — the engine connects to an already-running browser at
  `cdp_endpoint` (default `http://127.0.0.1:9222`) and drives the first existing
  page, or the first tab whose URL contains `tab` if given. The browser is **not**
  closed when the session closes — the engine did not open it. Launch-only keys
  are ignored, and setting them alongside `attach` is a warning, not an error.

`browser: electron` keeps working as an alias for `browser_mode: attach` so
existing configs do not break, but it is deprecated and the engine emits a
`log` event saying so. When both are present, `browser_mode` wins.

`open` may override any of these for one session — `{"cmd":"open","args":{"mode":"attach","cdp":"http://127.0.0.1:9333"}}` — which is
what a binding exposes as constructor arguments. Precedence is the existing
config chain: `open` args › env › JSON config › defaults.

### Which browser, and therefore which protocol

`browser` names the engine: `chromium` (the default) or `firefox`. It is an
`open` argument as well as a config key — `{"cmd":"open","args":{"browser":"firefox","headless":true}}` — and an
unsupported value is answered with `bad_request` rather than a browser the
caller did not ask for.

The engine chooses the wire protocol from the browser, not the other way
round: Chromium is driven over CDP, Firefox over WebDriver BiDi, which is the
only protocol Firefox has spoken since it removed CDP in version 141. In
`attach` mode the endpoint decides instead, by scheme: `http://…` is CDP,
`ws://…` is BiDi. Everything above this layer — every command, every result
shape in this document — is identical either way.

### Sessions and `vars`

Variables written by `EXTRACT` persist across `run-step` calls within one
session. `vars` exposes that state explicitly so an LLM driver can read a value
out mid-flow, or seed one before starting, without inventing a DSL line.

## Reverse calls

Custom controls and `CALL HOST` handlers live in the **client**, not the engine.
When the engine is embedded in Go they are Go funcs; driven through a binding
they are on the far side of the pipe, so the engine has to call back.

The client declares them with `register`, and thereafter the engine may write an
**invoke** line. The `invoke` key marks the direction, so neither side can
mistake one for the other:

```json
{"invoke":1,"kind":"custom_control","page":"Login Page","target":"Username","action":"input","value":"ada","step":"FILL 'Username' field with 'ada'","vars":{}}
{"invoke":2,"kind":"call","name":"compute_total","args":["12","30"],"vars":{}}
{"invoke":3,"kind":"hook","hook":"before_group","tag":"smoke","vars":{}}
```

The client answers with the same id:

```json
{"invoke":1,"ok":true,"result":null}
{"invoke":2,"ok":true,"result":"42"}
{"invoke":2,"ok":false,"error":{"code":"handler_failed","message":"…"}}
```

A handler that fails **fails the step**, not the session. The client stays
usable and the browser stays open.

`register` may be sent before `open`: handlers describe the client, and a
decorator that runs at import time cannot wait for a browser.

### Nesting

The exchange is strictly nested inside a request the client is already waiting
on. The client sends `run-step`; the engine reaches a registered control, writes
the invoke, and blocks; the client — sitting in its own read loop — runs the
handler and answers. No second connection, no background thread on either side.

While an invoke is outstanding the engine also serves two **page primitives**,
so a handler can look at and touch the page the way an embedded Go handler does
with its `browser.Page`:

| `cmd` | args | result |
|---|---|---|
| `page.eval` | `js` | the evaluated value |
| `page.url` | — | current URL |

Everything else is refused with `bad_request` while a handler is running:
`run-step`, `map` and friends would re-enter the step that is currently
executing, so refusing is the alternative to deadlocking.

`page.eval` returns strings as strings, numbers as numbers and objects as
objects. (The engine's own `EvalJS` is not uniform here — string results arrive
unquoted — so the server normalises before replying.)

### Handler results

The result of a `call` is handed to the runtime exactly as an embedded Go
handler's would be:

- an **object** becomes runtime variables,
- a **scalar** goes into `into {var}` when the step named one,
- `null` sets nothing.

A `custom_control` result is ignored: its job is the side effect.

A `hook` result that is an object is merged into the suite's global variables,
which is how `before_all` seeds a token for every hunt that follows. Anything
else publishes nothing.

## Suite hooks

`register` accepts `hooks` as an array of `{kind, tag}`, where kind is
`before_all`, `after_all`, `before_group` or `after_group`; `tag` is required
for the group kinds and ignored otherwise.

Declare **one slot per (kind, tag)**, not one per handler. The engine registers
a single bridge per slot and the client runs every handler it holds for that
slot; declaring twice makes the engine call back twice and each handler run
twice.

## Hook peers: the same protocol, spawned the other way

Everything above assumes the client started the engine. `manul run --hooks
<script>` inverts that — the engine starts the script — and reuses this
protocol unchanged rather than inventing a second one. A peer is a client that
never drives a session; it only declares handlers and answers callbacks.

The peer's **stdout is the protocol** and carries nothing else, exactly as the
engine's does in serve mode. Its stderr is passed through, so a hook may print
and log freely.

**Handshake.** The peer speaks first. It writes `register` (zero or more times),
then `ready`; the engine answers each and then begins the run. Only those two
commands are legal here — anything else is refused with `bad_request` and the
run aborts, because there is no session yet to serve it:

```
→ {"id":1,"cmd":"register","args":{"calls":["py.upper"],"hooks":[{"kind":"before_all"}]}}
← {"id":1,"ok":true,"result":{"controls":0,"calls":1,"hooks":1}}
→ {"id":2,"cmd":"ready"}
← {"id":2,"ok":true,"result":{}}
```

The handshake must complete before the first hunt starts. Worker goroutines read
the extension registries without synchronising against late arrivals, so a
handler declared afterwards may or may not be seen.

**Then it inverts.** Once ready, every line the engine writes is an `invoke` and
every line the peer writes is a reply — or a nested `page.eval` / `page.url`,
under the same rules as above.

**Concurrency.** Under `--workers > 1` group hooks fire from several goroutines
at once, and there is one pipe. The engine serialises the exchanges: a peer will
never be asked to handle a second invocation before it has answered the first,
and hooks therefore do **not** run in parallel even when the hunts they bracket
do. A peer needs no locking of its own.

**Shutdown.** The engine closes the peer's stdin after the last `after_all`
hook. The peer should read that EOF as "the run is over" and exit; one that does
not is killed after ten seconds and its exit status reported.

Failure behaviour is not uniform, deliberately:

| Hook | On failure |
|---|---|
| `before_all` | the suite aborts — no hunt runs. `after_all` still fires. |
| `before_group` | the hunts in that group are skipped; the rest of the suite runs. |
| `after_all`, `after_group` | reported, changes no result. Every remaining hook still runs. |

Cleanup that stops halfway leaves more behind than it saves, which is why the
`after_*` hooks never short-circuit.

`before_all` runs before any browser exists, so `page.eval` is unavailable
inside it — the engine answers that there is no page rather than inventing one.

## Versioning

`protocol` in the `ready` event is `major.minor`:

- **minor** bumps add commands, args, or result fields. Bindings must ignore
  unknown fields.
- **major** bumps change or remove existing shapes. A binding refuses to run
  against a `major` it does not know.

The engine version is reported separately and is not tied to the protocol
version.

## Conformance

`conformance/` drives every command in this table through both the one-shot CLI
and `serve`, against local fixtures, and requires identical result payloads. A
divergence between the two paths is a bug in `serve`, not an acceptable
variation.
