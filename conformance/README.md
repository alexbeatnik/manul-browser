# conformance/

**Not started yet.** Blocked on `manul serve` in the same way `bindings/` is.

## What it is for

With one engine, this is not about keeping rival implementations honest — it is
about keeping the *paths into* the engine honest. The same request must produce
the same bytes whether it arrives as:

1. a one-shot CLI command (`manul run-step '…'`),
2. a `serve` session command over stdio,
3. the Python binding,
4. the Node binding.

Divergence between 1 and 2 is a bug in `serve`. Divergence between 2 and 3/4 is
a bug in the binding. Neither is an acceptable variation.

## Shape

```
conformance/
├─ pages/      static HTML fixtures, served locally — no network
├─ cases/      *.hunt + the command sequence to drive
├─ expected/   golden JSON payloads
└─ run.go      harness: executes every case through every path, diffs
```

Fixtures are local files. A conformance run that needs the internet is not
reproducible and will rot.

## Why golden JSON and not assertions

The agent-facing payloads *are* the product — an LLM driver consumes them
directly, and the token-efficiency claims (`map` being 4–8× cheaper than raw
HTML) are claims about their exact shape. A hand-written assertion checks that a
field is right; a golden file also catches the field nobody meant to add.

Regenerating goldens must be a deliberate, reviewed act, never a flag someone
reaches for to make CI green.
