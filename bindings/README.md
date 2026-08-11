# bindings/

Thin language clients for the engine.

```
bindings/
├─ python/    manul    (PyPI)          — implemented
└─ node/      @manul/engine (npm)      — not started
```

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
  extension, so no compiler and no `cibuildwheel`: a script takes the goreleaser
  output and writes wheels with the right `--plat-name`.
- **npm** — `@manul/engine` declaring `optionalDependencies` on per-platform
  packages (`@manul/engine-linux-x64`, `-darwin-arm64`, `-win32-x64`, …), each
  carrying one binary and gated by `os`/`cpu`.

One git tag builds the binaries and publishes both packages in a single
workflow, so versions can never disagree.

## Note on pure-Python

The previous Python engine ran with one dependency and no binary. That property
is lost here by construction — a binding is a wrapper around a compiled engine.
It was dropped deliberately: no user requirement for it was known at the time of
the merge. If one appears, the old implementation is recoverable from history
(see [`../spec/README.md`](../spec/README.md)), and it would have to be held to
`conformance/` like anything else.
