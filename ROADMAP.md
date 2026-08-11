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

### 5. CI covers one thing

`.github/workflows/synthetic-tests.yml` runs the Go tests on push and PR. Not
run there: the Python binding tests, `gofmt`, any linter, any cross-platform
build. The release workflow does run the Python tests, but only on a tag — which
is the worst moment to discover they fail.

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
6. Push a tag, watch the release pipeline actually run, then the Node binding.
   (§missing 4, 3)

Items 1–4 are cheap and each one stops a class of future surprise. Items 5–6 are
the real remaining engineering.
