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
implementation going forward. The Python copy is still readable at
`legacy/python/contracts/` if a lost detail needs recovering.

That drift is the whole argument for this repository existing. Contracts live in
exactly one place now.

## Known gap: What-If / Explain

`contracts/MANUL_DEBUG_CONTRACT.md` is the Go version, which covers Debug &
Explain. The Python version of that contract was three times longer and
specified a **What-If Analysis REPL** — interactive resolution debugging with a
confidence scale and a highlight lifecycle — implemented in
`legacy/python/manul_engine/debug.py` and `explain_next.py` (~890 lines
together). Nothing in `core/` references it.

So "full parity with the Python engine" is not quite true, and this is the hole.
Anyone porting it should read the Python contract at
`legacy/python/contracts/MANUL_DEBUG_CONTRACT.md` first, then fold it back into
`contracts/MANUL_DEBUG_CONTRACT.md` here.
