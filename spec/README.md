# spec/

The engine's contracts. Everything else in this repository is an implementation
of, or a client of, what is written here.

- **`contracts/`** — describes behaviour that **ships today**, as implemented by
  `core/`. Changing the engine's observable behaviour means changing a contract
  in the same commit.
- **`protocol.md`** — the stdio session protocol used by `bindings/`. Currently
  a **draft**: `manul serve` does not exist yet.

## What-If

`contracts/MANUL_DEBUG_CONTRACT.md` covers Debug, Explain **and** What-If. The
REPL is implemented in `core/pkg/runtime/whatif.go`; the contract records the
two decisions worth knowing about it — a system step skips the DOM snapshot
entirely, and the terminal `WhatIfResult` is kept separate from the pipe-mode
`explainNextPayload` so the marker's wire format never moves.
