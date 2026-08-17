# Roadmap

Two lists: things that are **unverified** (they may already work, nobody has
checked) and things that are **missing** (they demonstrably do not exist).

Kept separate on purpose. Most of the risk in this repository right now is in
the first list, because unverified work reads exactly like finished work.

Everything described here is committed and pushed; nothing below is waiting on
the working tree.

---

## Needs verifying

### 1. Only Windows, only launch mode

Everything was exercised on one Windows machine against one Chrome.

- **Linux and macOS**: never run. The engine cross-compiles, but Chrome
  discovery, temp profiles and path handling are the usual places this bites.
- **`attach` mode**: never driven end-to-end against a real running Chrome. The
  config resolution is unit-tested; the actual attach path is not. Specifically
  unverified: that the browser is genuinely *left open* on close, which is the
  behaviour documented in three places.
- **The What-If REPL**: implemented and unit-tested, never driven interactively.
  It needs a TTY, so it has to be tried by hand: `manul <file> --debug`, then
  `w`, then `!execute`.

### 2. ~~`TestCollectScheduledHunts_Subdirectories`~~ — done

All twelve long-standing Windows failures are fixed, and none was an engine
defect. This one asserted on a literal `"sub/nested.hunt"` while `filepath.Walk`
produces `sub\nested.hunt`; the eleven in `pkg/config` chdir'd into their own
`t.TempDir()`, which Windows then refuses to delete. Details in `CLAUDE.md`.

The suspicion recorded here was half right — it *was* a path-separator bug, just
in the assertion rather than in `CollectScheduledHunts`.

### 3. Suite hooks under `--workers > 1`

Group hooks fire concurrently in the worker pool — the same handler may run on
several goroutines at once. The code is written for that and the pool wiring
compiles, but no test runs hooks under real parallelism. Race detector on a
multi-hunt tagged run would settle it.

### 4. `run-suite` session semantics

All hunts in a suite share one browser session, so state can leak between them.
Nobody has decided whether that is correct or whether each hunt should get a
fresh page. Decide, then write it into the contract either way.

### 5. Reverse-call nesting depth

A custom control whose handler triggers a `CALL HOST` — an invoke inside an
invoke — is untested. The design should support it (the exchange is strictly
nested), but "should" is doing real work in that sentence.

### 6. `manul map` label quality

On a fixture with `<label for="email">Email address</label>`, `map` reported the
element as `email` — the id, not the label a person would say. Possibly correct
by design, possibly a gap in the map's label resolution. Worth one look, because
`map` is what an LLM sees.

---

## Missing

### 1. Hooks and custom controls from the CLI — `--hooks`

Done. `manul run <hunts> --hooks <script>` is the third registration route
beside embedding and a binding, and the suite wiring in `cmdRun` now has a way
to fire — `lifecycle.IsEmpty()` was unconditionally true in the stock binary
before this.

It adds no wire format. The engine spawns the script and speaks the reverse-call
half of the existing session protocol down its stdio: the script writes the same
`register` line a binding writes, answers the same `invoke` lines, and may issue
the same nested `page.eval` / `page.url` while a handler runs. `pkg/serve` grew
a peer mode (`NewPeer`, `Handshake`) for the inverted direction; `pkg/hooks` is
process handling and almost nothing else.

Everything the feature decides lives in Go, deliberately — which script, which
interpreter, when hooks fire, in what order, how shutdown happens. A binding
supplies only the library the script imports, which in Python is
`manul.serve_hooks()` and a dispatcher shared with `Session`.

Verified end-to-end against a real Chrome: `before_all` publishing a variable a
hunt then read, `CALL HOST` reaching Python and returning, `after_all` at
teardown, and `print()` inside a hook not corrupting the stream.

What is deliberately not there:

- **Suite hooks get no page access under `--hooks`.** Controls and `CALL HOST`
  are handed the live page; a hook is not, because the peer server holds no
  session. `before_all` legitimately has no page anyway, but a `before_group`
  one would be reachable and is not wired.
- **Hooks do not run in parallel.** Under `--workers > 1` group hooks fire from
  several goroutines and there is one pipe, so the engine serialises the
  exchanges. That is the correct trade, but it means a slow hook is a bottleneck
  a parallel run cannot route around. §verify 3 is now partly answered by
  construction rather than by a race-detector run.
- **No auto-discovery.** A `manul_hooks.py` sitting beside the hunts does
  nothing unless `--hooks` names it. Making the path implicit was rejected: the
  only place that could infer it is a binding's CLI shim, and that would put
  behaviour outside the engine.

### 2. Conformance suite

`conformance/` has a README and nothing else. The point is that the same request
gives the same bytes through the one-shot CLI, a `serve` session, and each
binding. Right now nothing enforces that, which is precisely the failure mode
this repository was created to end.

### 3. Node binding — written, unpublished

`bindings/node/` exists: TypeScript, ESM, no runtime dependencies, Node ≥ 22.
`Session` mirrors Go's `agent.Session` and Python's `manul.Session` method for
method, the handler registry and reverse-call dispatch match Python's, and
`serveHooks()` is the `--hooks` peer so a `.js` hook script works from the CLI.
43 tests run against a fake engine, needing neither Chrome nor a binary.

Verified against a real Chrome: `saucedemo.hunt` through the binding gives the
same 15/15 the CLI and the Python client give, and a Node hook script publishing
a variable, answering `CALL HOST`, and evaluating in the page all work.

The prediction that this would be "packaging more than logic" was wrong in one
place. Python blocks on a read and holds a lock; Node cannot block, so replies
are matched by id against a pending map. That makes nested `page.eval` fall out
for free, but it also means two overlapping commands could each open a reverse
call — and reverse calls are strictly nested. Top-level commands are therefore
serialised through a queue, which is what Python's lock does by other means.

What is left is the packaging half:

- **Nothing is published.** `optionalDependencies` is empty because the
  `@manul-browser/engine-<platform>-<arch>` packages do not exist yet; the
  release workflow that would build them is §4, and it is switched off. Until
  then the binding finds the engine through `$MANUL_BINARY` or `PATH`, which is
  what the tests and the manual runs used.
- **The release workflow does not know about npm.** It builds six wheels; it
  builds no tarballs, and `bindings/node` is not in it at all.

### 4. Release pipeline — written, switched off, never fired

`.github/workflows/release.yml.disabled` takes a `vX.Y.Z` tag, cross-compiles the
engine for six targets, wraps each in a platform-tagged wheel, and installs every
wheel on a matching runner to drive a real Chrome through it. Locally verified as
far as a Windows machine allows: the wheel builds, installs, and `manul
--version` answers from the bundled binary.

**None of it runs.** The file is disabled twice over — the extension is not
`.yml`, so GitHub never parses it, and every line inside is commented out as
well. A tag pushed today starts nothing.

That is deliberate while the repository is still being put in order. It also
buys the one thing worth being careful about: the first upload to PyPI is the
single irreversible step here — the version number is spent whether or not the
artifact was any good, and yanking does not give it back. So the `publish` and
`github-release` jobs are commented a *second* time inside the workflow, and
stay that way when the rest is switched back on. The order to follow is written
above them: repository renamed first, then the PyPI trusted publisher and the
`pypi` environment, then uncomment.

What is still open:

- **It has never run**, and cannot until it is re-enabled. Everything below is
  therefore theory, including whether `pypi` as a trusted publisher is
  configured at all — which is a setting on PyPI, not in this repository.
- **npm and NuGet do not exist yet.** "Both packages together" is currently one
  package. The npm half is §3.
- **`macos-15-intel` and `ubuntu-24.04-arm` smoke jobs are best-effort.** They
  are marked `continue-on-error` so a retired runner label cannot hold up a
  release; those two wheels ship built but unproven.
- **Nothing publishes the Go module.** It needs its own `core/vX.Y.Z` tag,
  because Go derives the version from the subdirectory the module lives in. The
  release job does not create it.

### 5. CI runs on Linux only

Mostly closed. `.github/workflows/synthetic-tests.yml` became
`.github/workflows/ci.yml` and grew three jobs beside the engine tests: a
`gofmt` gate with `go vet`, a cross-compile of all six release targets, and the
Python binding tests on 3.9 and 3.13 across Linux and Windows. The binding no
longer waits for a tag to be tested.

Closing the `gofmt` gate needed a one-time cleanup of 19 files, and a
`.gitattributes` pinning `*.go` to `eol=lf`. The claim that ~80 files were
unformatted was a measurement error: `core.autocrlf` on Windows made gofmt
compare CRLF against its own LF output and report almost every file.

What is still not covered:

- **The Go tests run on `ubuntu-latest` and nowhere else.** The cross-build job
  proves every target compiles, which is not the same as passing. A Windows
  runner would go red today on the twelve failures described in `CLAUDE.md` —
  eleven of them a `TempDir` cleanup artefact rather than a defect, so switching
  it on means fixing or skipping those first. That is §verify 2's real cost.
- **Nothing drives Chrome.** Only the release workflow does, and only on a tag.
- **No Python linting or type checking.** `mypy`/`ruff` would each be a new
  development dependency in a package that deliberately has none; if they go in,
  they go in as CI-only tools, not as `pyproject` dependencies.

### 6. Suite-level lifecycle: `MANUL_GLOBAL_VARS`

Deliberately not carried over — the `GlobalContext` seeds each runtime directly,
including across pool goroutines, so the env-var serialisation has no job. Listed
here only so nobody re-adds it thinking it was forgotten.

---

## Suggested order

1. ~~Add the Python tests and `gofmt` to CI.~~ **Done** — see §missing 5 for
   what that left behind.
2. ~~Decide the CLI extension story.~~ **Done** — `--hooks`, see §missing 1.
3. Verify `attach` end-to-end, and on one non-Windows platform. (§verify 1)
4. Settle `run-suite` session semantics and write it into the contract. (§verify 4)
5. Conformance suite. (§missing 2)
6. Teach the release pipeline about npm, push a tag, and watch it run.
   (§missing 4, and the packaging half of §missing 3)

Items 2–4 are cheap and each one stops a class of future surprise. Items 5–6 are
the real remaining engineering.
