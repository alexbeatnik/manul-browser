# Manul stdio session protocol

Status: **draft** — not yet implemented in `core/`.

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
{"event":"ready","protocol":"1.0","engine":"0.1.0"}
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
| `map`       | `maxPerGroup?`, `includeUnlabeled?`               | page map `{url, groups:[…]}` |
| `read`      | `label?`, `selector?`, `maxChars?`                | `{value,found,reason}` or `{text,selector}` |
| `run-step`  | `step`, `compact?`                                | `StepOutcome` |
| `run`       | `path` or `source`                                | full `ExecutionResult` for a whole `.hunt` |
| `vars`      | `set?` (object), `get?` (array of names)          | current variable map |
| `close`     | —                                                 | `{}`, then the server exits |

Calling `open` twice in a session is `bad_request`.

### Launch a new Chrome, or attach to a running one

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

- **`launch`** — the engine starts a fresh Chrome and owns its lifetime; it is
  closed when the session closes. `browser`, `channel`, `executable_path`,
  `browser_args`, `headless` apply.
- **`attach`** — the engine connects to an already-running Chrome at
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

### Sessions and `vars`

Variables written by `EXTRACT` persist across `run-step` calls within one
session. `vars` exposes that state explicitly so an LLM driver can read a value
out mid-flow, or seed one before starting, without inventing a DSL line.

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
