# bindings/

Thin language clients for the engine.

```
bindings/
├─ python/    manul-browser (PyPI)     — implemented
└─ node/      manul-browser (npm)      — implemented
```

One name in every registry: `manul-browser` on PyPI and npm, `Manul.Browser` on
NuGet if a C# client happens, and `github.com/alexbeatnik/manul-browser/core` as
the Go module. What a user types to install differs by ecosystem convention;
what they write in code does not — `import manul`, `from 'manul-browser'`.
(The bare name `manul` on PyPI belongs to an unrelated project, which is what
settled the question.)

Go needs no binding: it embeds the engine directly via
[`core/pkg/agent`](../core/pkg/agent), with
[`core/examples/go`](../core/examples/go) as the worked example.

## The rule

A binding ships the platform binary, starts `manul serve --stdio`, and speaks
the protocol in [`../spec/protocol.md`](../spec/protocol.md). That is all it
does.

It must **not** contain: element scoring, DSL parsing, CDP framing, in-page
probe JavaScript, or report generation. If a binding needs behaviour the engine
does not expose, the fix is a new protocol command in `core/`, not a local
implementation. Two implementations of the scorer is the exact failure this
repository was created to end.

What a binding *is* allowed to own: process lifecycle, idiomatic typing,
async/await ergonomics, error translation, and packaging.

## Keeping the two languages honest

The Python `Session` mirrors Go's `agent.Session` method for method — `step`,
`run`, `map`, `read`, `state`, `vars`, `close`. Same names, same meanings, same
results, because both are the same engine.

Two behaviours worth stating once, since they are easy to get wrong in a
wrapper and both are already right here:

- **A failed step is not an exception.** `step()` returns an outcome whose `ok`
  is false. Not finding an element is something an agent reacts to.
- **Attach does not close the browser.** Only a session that launched Chrome
  closes it.

## Distribution

The esbuild model, in both ecosystems:

- **PyPI** — platform-tagged wheels with the binary at `manul/_bin/`. Not a C
  extension, so there is nothing for hatchling to infer a tag from and no
  `cibuildwheel` to run: [`python/hatch_build.py`](python/hatch_build.py) states
  the tag outright, from the same `GOOS/GOARCH` pair the cross-compile used.
  An sdist goes up beside them, for platforms with no wheel: it installs without
  an engine and looks for one in `$MANUL_BINARY` or on `PATH`.
- **npm** — `manul-browser` declaring `optionalDependencies` on per-platform
  packages (`@manul-browser/engine-linux-x64`,
  `@manul-browser/engine-darwin-arm64`, …), each carrying one binary and gated
  by `os`/`cpu`. Unscoped main package, scoped platform packages — the same
  split esbuild and rollup use, and it means one npm organisation reserves every
  platform name instead of six global ones being claimable separately.
  [`node/src/binary.ts`](node/src/binary.ts) resolves exactly this name at
  runtime. The committed `package.json` lists none of them:
  [`node/scripts/pack.mjs`](node/scripts/pack.mjs) adds the six, at the version
  being packed, so the list cannot name a different version from the package
  that carries it.

[`build-packages.sh`](build-packages.sh) is the one description of how any of
this is put together. It cross-compiles the engine, wraps a wheel and an npm
package around each binary, and leaves everything under `dist/` — and it refuses
to start unless the engine's `version` constant, `manul.__version__` and
`package.json` are the same string, so the packages cannot ship at different
versions from the engine inside them. It publishes nothing.

```bash
python -m pip install build
bash bindings/build-packages.sh      # → dist/pypi, dist/npm, dist/bin
```

On Windows it builds the Windows targets only. A wheel or a tarball records the
mode bits it finds on disk, and a Linux engine that installs as a file nobody
can run is worse than no package — so the full set of six comes from a POSIX
host, which in practice means CI.

Publishing is [`.github/workflows/publish.yml`](../.github/workflows/publish.yml),
and a push to `main` starts it. It asks PyPI and npm whether the version on
`main` is already there and stops if it is, so what releases is the version
bump, not the push. Otherwise it runs that script, installs each package on a
matching runner and drives a real Chrome through it — a package is useless if
the engine inside it will not start — and only then uploads. Started by hand
with its inputs left alone it is a rehearsal that leaves the packages on the run
as artifacts. Its header lists the one-time setup each registry needs.

An upload cannot be taken back, so bumping the version is the decision to
publish: the number is spent the moment it reaches a registry.

The Go module is tagged separately as `core/vX.Y.Z` by `release.yml`, because Go
derives module versions from the subdirectory the module lives in.

## Note on shipping a binary

A binding is a wrapper around a compiled engine, so a wheel carries a platform
binary rather than pure Python. That is the deliberate trade for having one
implementation. If a pure-language client is ever wanted, it would have to be
held to
`conformance/` like anything else.
