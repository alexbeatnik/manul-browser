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
  Written, in [`.github/workflows/release.yml.disabled`](../.github/workflows/release.yml.disabled),
  and switched off there — see below.
- **npm** — `manul-browser` declaring `optionalDependencies` on per-platform
  packages (`manul-browser-linux-x64`, `-darwin-arm64`, `-win32-x64`, …), each
  carrying one binary and gated by `os`/`cpu`. Not started.

One git tag — `vX.Y.Z` — builds every binary and packages them together, so
versions cannot disagree; the release job refuses to start unless the tag, the
engine's `version` constant and `manul.__version__` are the same string. The Go
module is tagged separately as `core/vX.Y.Z`, because Go derives module versions
from the subdirectory the module lives in.

A wheel is useless if the engine inside it will not start, so the same workflow
installs each wheel on a matching runner and drives a real Chrome through it.

None of which happens yet. The whole workflow is switched off — the file does
not end in `.yml`, and its contents are commented out on top of that — so a tag
today starts nothing at all. Inside it, the `publish` and `github-release` jobs
are commented a second time, because publishing is the step to re-enable last
and separately. Both headers say what to switch on and in what order.

## Note on pure-Python

The previous Python engine ran with one dependency and no binary. That property
is lost here by construction — a binding is a wrapper around a compiled engine.
It was dropped deliberately: no user requirement for it was known at the time of
the merge. If one appears, the old implementation is recoverable from history
(see [`../spec/README.md`](../spec/README.md)), and it would have to be held to
`conformance/` like anything else.
