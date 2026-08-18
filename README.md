<p align="center">
  <img src="core/images/manul.png" alt="Manul Browser" width="200">
</p>

# Manul Browser

**Browser automation in plain English — for humans and LLM agents.**

Manul Browser runs `.hunt` files through deterministic DOM heuristics, over
native CDP in Chromium and WebDriver BiDi in Firefox. No Playwright, no
selectors, no cloud APIs, no AI inside the runtime.

This repository is the single home for the engine and every language that drives
it. It descends from two earlier engines — one in Python, one in Go — whose full
histories are merged into this one; neither is maintained any more.

## Layout

```
core/            The engine. Go. The only implementation.
spec/            Contracts + the stdio session protocol. Source of truth.
bindings/        Thin language clients that drive the binary. (planned)
conformance/     Fixtures every path must agree on. (planned)
```

## Why one implementation

The predecessors were the same product written twice, in two languages, and
keeping them in step was manual work that did not hold — by the time they were
merged, eight of their nine shared contracts had drifted apart. A third
implementation would only have made that worse.

So the engine is Go, once. Python and JavaScript/TypeScript get **thin bindings**
that ship the platform binary and talk to it over the protocol in
[`spec/protocol.md`](spec/protocol.md) — the same shape esbuild, ruff, and biome
use. A binding is a few hundred lines of process management and typing; it
contains no scoring, no DSL parsing, and no CDP.

The agent commands already emitted JSON, so most of that contract already
existed — it just needed a session that outlives a single command.

## Status

| Piece | State |
|---|---|
| `core/` engine | Shipping — builds, tests green |
| `spec/contracts/` | Current, describes shipped behaviour |
| `spec/protocol.md` | Implemented as `manul serve --stdio` |
| `bindings/python` | Working — packaged as `manul-browser`, not yet published |
| `bindings/node` | Working — packaged as `manul-browser` (npm), not yet published |
| `conformance/` | Not started |
| Release pipeline | Binaries: `.github/workflows/release.yml` — six targets, checksums, GitHub Release, `core/vX.Y.Z` module tag. Wheels and npm: still off, see `release.yml.disabled` |
| What-If REPL | Ported to Go — terminal-only, see the debug contract |
| Custom controls, `CALL HOST` | Go handlers, client handlers via reverse call, or a `--hooks` script |
| Suite hooks | `pkg/lifecycle` — `before_all`/`after_all`/`before_group`/`after_group` |

## Use it

**Python** — `pip install manul-browser`; the wheel ships the binary, and you
need only a system Chrome or Firefox.

```python
import manul

with manul.Session() as s:
    s.step("NAVIGATE to https://example.com")
    s.step("CLICK the 'Sign in' button")
    print(s.map().labels())
```

**Go** — embed the engine directly, no subprocess.

```go
sess, _ := agent.Launch(ctx, agent.Options{Headless: true})
defer sess.Close()
sess.Step(ctx, "CLICK the 'Sign in' button")
```

**CLI**

```bash
manul checkout.hunt
manul checkout.hunt --browser firefox
manul run-step "CLICK the 'Login' button" --cdp http://127.0.0.1:9222
manul run hunts/ --hooks manul_hooks.py
```

`--hooks` points the engine at a script that supplies custom controls, `CALL
HOST` handlers and suite-level hooks. The engine spawns it and speaks the same
reverse-call protocol a binding speaks — so the script is a few decorators and
one blocking call, and every decision about it stays in the engine. See
[`core/examples/hooks/`](core/examples/hooks/).

## Browsers

`--browser chromium` (the default) drives Chrome, Chromium or Edge over CDP.
`--browser firefox` drives Firefox over **WebDriver BiDi** — not CDP, which
Firefox deprecated in 129 and removed outright in 141. Both engines run the
same scoring, the same DSL and the same in-page JavaScript; only the wire
protocol differs, and the endpoint says which one it is: `http://…` is CDP,
`ws://…/session` is BiDi.

To drive a browser that is already running, set `browser_mode` to `attach` (or
`MANUL_BROWSER_MODE=attach`, or `--attach`) and point `--cdp` at it — a CDP
endpoint for Chromium, the `ws://` URL Firefox prints at startup for Firefox.
That browser is left open when the session ends — Manul did not open it.

See [`bindings/python/README.md`](bindings/python/README.md) and
[`core/examples/go`](core/examples/go).

## Build

```bash
cd core
go build ./cmd/manul
```

Requires Go ≥ 1.26 and a system Chrome/Chromium or Firefox on PATH. The result
is one static binary with `gorilla/websocket` as its only dependency.

## Documentation

- [`core/docs/`](core/docs/) — engine documentation
- [`spec/contracts/`](spec/contracts/) — behavioural contracts
- [`spec/protocol.md`](spec/protocol.md) — the stdio session protocol
- [`core/examples/`](core/examples/) — sample `.hunt` files
- [`CLAUDE.md`](CLAUDE.md) — working notes: invariants and traps
- [`ROADMAP.md`](ROADMAP.md) — what is unverified, and what is missing

## License

See [LICENSE](LICENSE).
