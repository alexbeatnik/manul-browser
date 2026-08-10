<p align="center">
  <img src="core/images/manul.png" alt="Manul" width="200">
</p>

# Manul

**Browser automation in plain English — for humans and LLM agents.**

Manul runs `.hunt` files through deterministic DOM heuristics over native CDP.
No Playwright, no selectors, no cloud APIs, no AI inside the runtime.

This repository is the single home for the engine and every language that drives
it. It replaces the two separate implementations that came before —
[`ManulEngine`](https://github.com/alexbeatnik/ManulEngine) (Python) and
[`ManulEngineGo`](https://github.com/alexbeatnik/ManulEngineGo) (Go) — whose
full histories are preserved here.

## Layout

```
core/            The engine. Go. The only implementation.
spec/            Contracts + the stdio session protocol. Source of truth.
bindings/        Thin language clients that drive the binary. (planned)
conformance/     Fixtures every path must agree on. (planned)
legacy/python/   The frozen Python engine. Not built, not tested, not shipped.
```

## Why one implementation instead of three

The Python and Go engines were the same product written twice. Keeping them in
step was manual, and it did not hold: by the time they were merged, eight of the
nine shared contracts had drifted, and one Python feature had no Go port at all.
Writing the scorer a third time in TypeScript would have made that worse.

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
| `spec/protocol.md` | Draft — `manul serve` not implemented yet |
| `bindings/python`, `bindings/node` | Not started |
| `conformance/` | Not started |
| What-If REPL | **Python only**, no Go port — see [`spec/README.md`](spec/README.md) |

## Build

```bash
cd core
go build ./cmd/manul
```

Requires Go ≥ 1.26 and a system Chrome/Chromium on PATH. The result is one
static binary with `gorilla/websocket` as its only dependency.

## Documentation

- [`core/docs/`](core/docs/) — engine documentation
- [`spec/contracts/`](spec/contracts/) — behavioural contracts
- [`core/examples/`](core/examples/) — sample `.hunt` files

## License

See [LICENSE](LICENSE).
