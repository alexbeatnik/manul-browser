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

### 2. `TestCollectScheduledHunts_Subdirectories`

Fails in 0.00s, which rules out the temp-file locking that explains the eleven
`pkg/config` failures. It looks like a real path-separator bug on Windows. Worth
half an hour; it is the only pre-existing failure that might be a genuine defect.

### 3. Suite hooks under `--workers > 1`

Group hooks fire concurrently in the worker pool — the same handler may run on
several goroutines at once. The code is written for that and the pool wiring
compiles, but no test runs hooks under real parallelism. Race detector on a
multi-hunt tagged run would settle it.

### 4. `run-suite` session semantics

All hunts in a suite share one browser session. The standalone Python engine
almost certainly gave each hunt a fresh page. Nobody has decided which is
correct here, so state leaking between hunts in a suite is currently possible
and undocumented. Decide, then write it into the contract either way.

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

### 1. Hooks and custom controls are unreachable from the CLI

The registration functions have exactly one caller outside tests: `pkg/serve`.
So extensions work when the engine is **embedded** (you call `RegisterGoCall` in
your own `main`) and when it is **driven through a binding** (declared over the
protocol) — and not at all from the stock `manul` binary. `lifecycle.IsEmpty()`
is always true there, so the suite wiring in `cmdRun` can never fire.

The Python engine solved this with auto-discovery of a `manul_hooks.py` beside
the hunts. Go has no equivalent and cannot have the same one. Options, roughly
in order of how much they cost:

- a `--hooks <script>` flag that runs a subprocess speaking the same reverse-call
  protocol — reuses everything that already exists;
- documenting that extensions require embedding or a binding, and removing the
  dead wiring from `cmdRun`;
- a plugin mechanism, which Go makes unpleasant.

Until one is chosen, the CLI half of the suite feature is decoration.

### 2. Conformance suite

`conformance/` has a README and nothing else. The point is that the same request
gives the same bytes through the one-shot CLI, a `serve` session, and each
binding. Right now nothing enforces that, which is precisely the failure mode
this repository was created to end.

### 3. Node binding

`bindings/node/` does not exist. The protocol and the Python client are the
template; the work is packaging (`optionalDependencies` per platform) more than
logic.

### 4. Release pipeline

No goreleaser config, no wheel build, no npm publish. The Python package expects
the binary at `manul/_bin/` and nothing puts it there. One git tag should build
the binaries and publish both packages together, so versions cannot disagree.

### 5. CI covers one thing

`.github/workflows/synthetic-tests.yml` runs the Go tests. Not run: the Python
binding tests, `gofmt`, any linter, any cross-platform build. The Python tests
take about a second and need no browser — that is the cheapest gap to close on
this list.

### 6. Suite-level lifecycle: `MANUL_GLOBAL_VARS`

Deliberately not carried over — the `GlobalContext` seeds each runtime directly,
including across pool goroutines, so the env-var serialisation has no job. Listed
here only so nobody re-adds it thinking it was forgotten.

---

## Suggested order

1. Add the Python tests and `gofmt` to CI. (§missing 5)
2. Decide the CLI extension story. (§missing 1)
3. Verify `attach` end-to-end, and on one non-Windows platform. (§verify 1)
4. Settle `run-suite` session semantics and write it into the contract. (§verify 4)
5. Conformance suite. (§missing 2)
6. Release pipeline, then the Node binding. (§missing 4, 3)

Items 1–4 are cheap and each one stops a class of future surprise. Items 5–6 are
the real remaining engineering.
