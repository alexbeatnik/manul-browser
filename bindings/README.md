# bindings/

Thin language clients for the engine. **Not started yet** — they are blocked on
`manul serve`, specified in [`../spec/protocol.md`](../spec/protocol.md).

```
bindings/
├─ python/    manul-engine       (PyPI)
└─ node/      @manul/engine      (npm)
```

## The rule

A binding ships the platform binary, starts it, and speaks the protocol. That
is all it does.

It must **not** contain: element scoring, DSL parsing, CDP framing, in-page
probe JavaScript, or report generation. If a binding needs behaviour the engine
does not expose, the fix is a new protocol command in `core/`, not a local
implementation. Two implementations of the scorer is the exact failure this
repository was created to end.

What a binding *is* allowed to own: process lifecycle, idiomatic typing,
async/await ergonomics, error translation, and packaging.

## Distribution

The esbuild model, in both ecosystems:

- **npm** — `@manul/engine` declares `optionalDependencies` on per-platform
  packages (`@manul/engine-linux-x64`, `-darwin-arm64`, `-win32-x64`, …), each
  carrying one binary and gated by `os`/`cpu`. npm installs only the matching one.
- **PyPI** — platform-tagged wheels built from the same release artifacts. This
  is not a C extension, so no compiler and no `cibuildwheel`: a script takes the
  goreleaser output and writes wheels with the right `--plat-name`.

One git tag builds the binaries and publishes both packages in a single
workflow, so versions can never disagree.

## Note on pure-Python

The previous Python engine ran with one dependency and no binary. That property
is lost here by construction — a binding is a wrapper around a compiled engine.
It was dropped deliberately: no user requirement for it was known at the time of
the merge. If one appears, the frozen implementation in `legacy/python/` is the
starting point, and it would have to be held to `conformance/` like anything else.
