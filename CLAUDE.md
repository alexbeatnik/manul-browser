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

Neither predecessor is in the working tree, and nothing outside this paragraph
refers to them. Their history is still an ancestor of `main`, so a file can be
read back without a network round-trip: `git show 9249843:legacy/python/...`
(`9249843` grafted it in; `b5d85d7` is the final upstream Python commit).

## Commands

```bash
cd core && go build ./... && go test -short ./...      # engine
cd core && gofmt -l . && go vet ./...                  # both are CI gates
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

**Two protocols, one engine.** Chromium is driven over CDP (`pkg/cdp`), Firefox
over WebDriver BiDi (`pkg/bidi`) — Firefox removed CDP in version 141, so this
is not a preference. Everything above the transport is shared, and the in-page
JavaScript lives in `pkg/pagejs` precisely so the two backends cannot grow
their own dialects of FILL or CHECK. A behaviour that belongs to the page goes
there; only framing belongs in `pkg/cdp` or `pkg/bidi`.

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

**Firefox is not a CDP browser, whatever `--remote-debugging-port` suggests.**
The flag survives; it starts the BiDi agent. `/json/version` and `/json/list`
are gone with the rest of CDP, so endpoint discovery reads the startup banner
on **stderr** instead — and that banner names a bare origin
(`ws://127.0.0.1:9222`) while the WebSocket upgrade only succeeds at
`/session`. `bidi.NormalizeWebSocketURL` is what closes that gap; without it
the handshake fails with nothing but `bad handshake` to go on.

**One BiDi session per Firefox.** `session.new` on a second socket fails
rather than opening a second view, so every page of one endpoint shares a
single connection (`sharedConn` in `pkg/browser/bidi_backend.go`) and
identifies itself by browsing-context id. Consequently `BiDiPage.Close` does
**not** close the socket — doing so would cut off every sibling page,
including the caller's own tab during a background `Lookup`.

**BiDi sends no events until asked.** `Conn.Subscribe` only opens a local
channel; without `session.subscribe` for the event name, nothing arrives.
Subscribe locally *first*, then ask the browser — the other order drops
anything that happens in between, which for `WAIT FOR RESPONSE` means waiting
out the timeout for a response that already came.

## Local environment

The suite is green on Windows. It was not for a long time — twelve tests failed,
and both causes turned out to be in the tests rather than the engine:

- eleven in `pkg/config` called `os.Chdir` into their own `t.TempDir()`. Windows
  refuses to delete a directory that is a process's working directory, so the
  cleanup failed after the assertions had already passed. `t.Chdir` restores the
  old directory on cleanup, and its cleanup runs before `TempDir`'s because
  cleanups are LIFO. Linux allows deleting the working directory, which is why
  this was invisible there;
- `TestCollectScheduledHunts_Subdirectories` in `pkg/daemon` asserted on the
  literal `"sub/nested.hunt"`. `filepath.Walk` produces `sub\nested.hunt` here.
  The production code was right the whole time.

If either pattern reappears, it will fail on Windows and pass in CI.

The repository **is** gofmt-clean, and CI fails if it stops being. It used to
look otherwise: `core.autocrlf=true` gives this machine a CRLF working tree
against an LF index, gofmt counts CR as content, and so `gofmt -l` named 115
files while only 19 were actually unformatted. `.gitattributes` now pins `*.go`
to `eol=lf`, which makes the local check agree with the runner. If you still see
a wall of unformatted files, your checkout predates that file — re-checkout, or
compare against the index with `git show :path/to/file.go | gofmt -d`.

## Conventions

- Go ≥ 1.26, single dependency (`gorilla/websocket`). Keep it that way.
- Firefox work can be checked against a real browser: `manul run x.hunt
  --browser firefox --headless`. There is no fake for either protocol — the
  unit tests drive a mock WebSocket server, which proves framing, not
  behaviour.
- The engine version lives in `cmd/manul/main.go` as `version`, without a `v`
  prefix, and must match the contracts.
- `.hunt` verbs are matched by keyword, not strict grammar. New verbs go in the
  `parseCommand` switch **before** any prefix that would swallow them — that is
  why `WAIT FOR SELECTOR` is checked ahead of `WAIT FOR`.
