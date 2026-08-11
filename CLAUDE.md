# CLAUDE.md

Working notes for this repository. Behavioural coding guidance lives in
[`core/CLAUDE.md`](core/CLAUDE.md); this file is about *Manul specifically*.

## What this is

One engine, written once, in Go. Everything else drives it.

```
core/            The engine. The only implementation of scoring, DSL and CDP.
spec/            Contracts + the stdio session protocol. Source of truth.
bindings/python/ Thin client: ships the binary, speaks the protocol.
conformance/     Fixtures every path must agree on. (not started)
```

The repository exists because the engine was previously written twice, in Go and
Python, and they drifted: eight of nine shared contracts had diverged by the time
they were merged. **Do not reintroduce a second implementation.** If a binding
needs behaviour the engine lacks, add a protocol command in `core/` — never a
local implementation in the binding.

## Commands

```bash
cd core && go build ./... && go test -short ./...      # engine
cd bindings/python && python -m pytest tests/ -q       # binding (no Chrome needed)
```

The Python tests run against a fake engine that speaks the protocol, so they
need neither Chrome nor a compiled binary and finish in about a second. Anything
that genuinely needs a browser has to be run by hand.

## Invariants

**stdout is the protocol.** In `manul serve`, stdout carries NDJSON and nothing
else. Every log goes to stderr. One stray `fmt.Println` breaks every client.

**Contracts describe shipped behaviour.** `spec/contracts/*.md` are not design
documents. Changing what the engine does means changing the contract in the same
commit. Each file embeds a JSON block that must stay parseable — check it.

**Bindings own process lifecycle and typing, nothing else.** No scoring, no DSL
parsing, no CDP framing, no probe JavaScript.

**A declared verb must be executable.** `pkg/runtime` carries a guard test that
walks every `CommandType` and fails on "not yet implemented". `WAIT FOR` and
`HIGHLIGHT` both shipped declared-but-dead; that is what the guard is for.

## Traps that have already cost time

**`EvalJS` is not uniform.** Numbers, booleans and objects come back as JSON;
**strings come back as bare, unquoted bytes**. `json.Unmarshal` on a string
result fails. Swallowing that error turns every string the page produced into
`null`. Use `decodeEvalResult` in `pkg/serve`, or compare the raw bytes.

**The scorer always ranks something.** "A top candidate exists" is not "the
target is present" — a nonsense label still returns a best guess. Anything
asking *is this on the page* must compare against `ThresholdAmbiguous`, the bar
the rest of the engine uses. Without it, a wait for a missing element passes on
the first poll.

**Reverse calls are strictly nested.** While the engine waits for a handler's
reply it serves only `page.eval` and `page.url`. Anything else would re-enter
the step currently executing, so it is refused rather than deadlocking. A
handler must not call `session.map()`.

**Snapshot caching hides change.** The cache makes resolution cheap within a
step. Any polling loop must call `invalidateSnapshot()` each iteration or it
will never see what it is waiting for.

## Local environment

Twelve tests fail on this Windows machine and have since before any of this
work: eleven in `pkg/config` (Go's `TempDir` cleanup hitting a file lock — the
test logic itself passes) and `TestCollectScheduledHunts_Subdirectories` in
`pkg/daemon`, which fails in 0.00s and looks like a genuine path-separator bug
rather than timing. Both predate the merge; confirm against a clean upstream
clone before blaming a change.

The repository is **not** gofmt-clean and never was — roughly 80 files. Run
`gofmt -w` only on files you touched, or the diff drowns in unrelated churn.

## Conventions

- Go ≥ 1.26, single dependency (`gorilla/websocket`). Keep it that way.
- The engine version lives in `cmd/manul/main.go` as `version`, without a `v`
  prefix, and must match the contracts.
- `.hunt` verbs are matched by keyword, not strict grammar. New verbs go in the
  `parseCommand` switch **before** any prefix that would swallow them — that is
  why `WAIT FOR SELECTOR` is checked ahead of `WAIT FOR`.
