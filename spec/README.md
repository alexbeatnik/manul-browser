# spec/

The engine's contracts. Everything else in this repository is an implementation
of, or a client of, what is written here.

- **`contracts/`** — describes behaviour that **ships today**, as implemented by
  `core/`. Changing the engine's observable behaviour means changing a contract
  in the same commit.
- **`protocol.md`** — the stdio session protocol used by `bindings/`. Currently
  a **draft**: `manul serve` does not exist yet.

## A note on where these came from

Both the Go and the Python repositories carried a `contracts/` directory with
the same nine filenames. At the time of the merge, eight of the nine had
drifted apart — same names, different content, no mechanism forcing them to
agree. The copy kept here is the Go one, because `core/` is the single
implementation.

That drift is the whole argument for this repository existing. Contracts live in
exactly one place now.

## Recovering the Python engine

The Python implementation is not in the working tree — nothing unused is kept
here — but it is not lost either. Its full history is an ancestor of `main`, so
any file can be read back without a network round-trip:

```bash
git show 9249843:legacy/python/manul_engine/explain_next.py
git ls-tree -r 9249843 --name-only legacy/python/
```

`9249843` is the commit that grafted it in; `b5d85d7` is the final upstream
Python commit.

## What-If

`contracts/MANUL_DEBUG_CONTRACT.md` v0.2.0 covers Debug, Explain **and**
What-If. The What-If REPL was the one feature that existed only in Python when
the repositories were merged — the Go engine's claim of "full parity" had this
one hole. It is now implemented in `core/pkg/runtime/whatif.go`, and the
contract documents the two places the Go behaviour deliberately differs from the
Python original: system steps skip the DOM snapshot entirely, and the terminal
`WhatIfResult` is kept separate from the extension's `explainNextPayload` so the
existing wire format stays byte-compatible.
