# manul (Python)

Browser automation in plain English — for humans and LLM agents.

```bash
pip install manul-browser
```

> Not on PyPI yet — the release workflow builds the wheels but does not upload
> them. Until it does, install from a workflow artifact or build one yourself:
> `MANUL_TARGET=<goos>/<goarch> python -m build --wheel`.

The distribution is `manul-browser`; the import stays `manul`. The wheel carries
the engine binary and installs the `manul` command, so there is nothing else to
install except a system Chrome/Chromium.

It supersedes `manul-engine`, the standalone pure-Python engine, and installs the
same `manul` command — so the two do not belong in one environment.

## Use

```python
import manul

with manul.Session() as s:
    s.step("NAVIGATE to https://example.com")
    s.step("CLICK the 'Sign in' button")
    s.step("FILL 'Email address' field with 'ada@example.com'")

    print(s.map().labels())
    print(s.read("Order total").value)
```

No selectors, no waits, no page objects. Targets are named the way a person
would name them, and the engine resolves them with deterministic DOM heuristics.

### Drive a Chrome you already have open

Start Chrome with `--remote-debugging-port=9222`, then:

```python
with manul.Session(mode="attach") as s:
    print(s.state()["url"])
```

That browser stays open when the session ends — Manul did not open it. This is
the one difference between the modes that surprises people, so it is worth
saying twice.

Mode can also come from configuration, which is what you want when the same
script runs locally and in CI:

```bash
export MANUL_BROWSER_MODE=attach
export MANUL_CDP_ENDPOINT=http://127.0.0.1:9222
```

Precedence is `Session(...)` arguments › environment › `manul_engine_configuration.json` › defaults.

### Variables persist across steps

This is why `Session` is a session and not a series of one-shot commands:

```python
with manul.Session() as s:
    s.step("NAVIGATE to https://shop.example.com/cart")
    s.step("EXTRACT the 'Order total' into {total}")

    print(s.vars()["total"])          # "142.50"

    s.set_vars(coupon="SPRING")
    s.step("FILL 'Coupon' field with '{coupon}'")
```

### Run a whole script

```python
with manul.Session() as s:
    result = s.run(path="checkout.hunt")
    print(result.passed, "/", result.total_steps)
```

## Custom controls

Some things a page does cannot be said as "click the thing called X": a canvas
signature pad, a third-party date picker, a widget that only answers synthetic
events. A custom control claims one target and takes over whenever a step aims
at it — the `.hunt` file keeps saying what it means, and the awkwardness stays
in one Python function.

```python
import manul

@manul.custom_control(page="Checkout", target="Signature Pad")
def sign(ctx):
    ctx.eval("document.querySelector('#pad').dispatchEvent(new Event('sign'))")

with manul.Session() as s:
    s.step("CLICK the 'Signature Pad'")     # your function runs instead
```

Leave `page` out and the control applies everywhere; a page-specific handler
beats a wildcard. Lookup ignores case and extra whitespace on both.

`ctx` carries `target`, `action`, `value`, `page`, `step` and `vars`, plus
`eval(js)` and `current_url()` for reaching into the live page. Those two work
while the engine is paused mid-step, which the ordinary session methods
deliberately do not — calling `s.map()` from inside a handler is refused rather
than deadlocking.

A handler that raises fails that step. The session stays open.

## CALL

`CALL HOST` reaches ordinary Python functions from a `.hunt` script. `CALL
PYTHON` is the same command, so scripts written for the standalone Python engine
keep working.

```python
@manul.call("compute_total")
def compute_total(ctx):
    return str(sum(float(a) for a in ctx.args))
```

```
CALL PYTHON compute_total with args: "12" "30" into {total}
```

Return an object and its keys become variables; return a scalar and it lands in
`into {var}`.

Handlers register at import time and are published to the engine when a session
opens, so a decorator does not need a session to exist yet.

## Suite hooks

A hunt's own `[SETUP]` / `[TEARDOWN]` covers one file. These cover a whole run —
logging in once for twenty hunts, tearing an environment down afterwards.

```python
@manul.before_all
def login(ctx):
    ctx.set("token", fetch_token())     # every hunt now sees {token}

@manul.after_all
def cleanup(ctx):
    drop_test_database()

@manul.before_group("smoke")
def seed(ctx):
    reset_fixtures()
```

Run them with `run_suite`, which is not a loop over `run()`: only the engine
parses `@tags:`, so only it can decide which group hooks a hunt belongs to.

```python
with manul.Session() as s:
    result = s.run_suite(["checkout.hunt", "search.hunt"])
    print(result.passed, "/", result.total, "-", result.skipped, "skipped")
```

Failure behaviour differs per hook, deliberately:

| Hook | On failure |
|---|---|
| `before_all` | the suite aborts; no hunt runs. `after_all` still fires. |
| `before_group` | those hunts are skipped; the rest of the suite runs. |
| `after_all`, `after_group` | reported, changes no result |

Cleanup that stops halfway leaves more behind than it saves, so the `after_*`
hooks always run every handler.

`before_all` runs before any browser exists, so `ctx.eval` is unavailable there
and says so rather than returning nothing.

## Errors

A step that resolves nothing is **not** an exception — it is an outcome to react
to:

```python
out = s.step("CLICK the 'Checkout' button")
if not out:
    print(out.reason, out.near)      # why, and what was nearby
```

Exceptions are reserved for things that are actually wrong:

| Exception | Meaning |
|---|---|
| `EngineNotFound` | the binary could not be located; the message lists everywhere it looked |
| `ProtocolError` | the engine died, or speaks a protocol major version this package predates |
| `EngineError` | the engine rejected the request (`.code` is `bad_request`, `not_open`, …). Recoverable — the session stays usable |
| `SessionClosed` | a call was made after `close()` |

## What this package is

A thin client. It ships the engine binary, starts it, and speaks a JSON protocol
over stdio. The scoring, the `.hunt` DSL and the CDP transport live in the
engine — one implementation, shared with the Go API and the CLI.

That means Python and Go cannot drift apart on what a step *does*, which is the
whole reason the project has one core.

### Pointing at your own build

```python
manul.Session(binary="/path/to/manul")            # explicit
```

or `MANUL_BINARY=/path/to/manul`. Resolution order: the `binary` argument,
`$MANUL_BINARY`, the copy bundled in the wheel, then `manul` on `PATH`.

`binary` also accepts a sequence when the engine needs a wrapper:

```python
manul.Session(binary=["docker", "exec", "-i", "web", "manul"])
```

## Development

```bash
cd bindings/python
python -m pytest tests/
```

The tests run against a fake engine that speaks the protocol, so they need
neither Chrome nor a compiled binary and finish in under a second.
