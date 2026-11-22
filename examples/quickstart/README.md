# Manul quickstart

Twelve short hunts, one idea each, against a small site that ships with them.
Read them in order and you have the whole language; or find the thing you need
in the table below and copy it.

## Run them

You need the `manul` binary (`pip install manul-browser`, `npm install
manul-browser`, or `go build ./cmd/manul` in `core/`) and a Chrome, Chromium,
Edge or Firefox. From this directory, serve the site in one terminal:

```bash
python -m http.server 8000 --directory site
```

and run a hunt in another:

```bash
manul 01_first_hunt.hunt
```

Add `--headless` to hide the browser and `--browser firefox` to use Firefox.
To run all twelve (the last one needs `pip install manul-browser` for its hook
script):

```bash
manul . --hooks hooks/manul_hooks.py --headless
```

Any static server will do — `npx serve site -l 8000` works too. The hunts only
expect the site at `http://localhost:8000`; to put it elsewhere, change the
`@var: {site}` line at the top of each.

## I want to…

| …do this | look at | the lines that matter |
|---|---|---|
| open a page and check what it says | [`01_first_hunt`](01_first_hunt.hunt) | `NAVIGATE to`, `VERIFY that '…' is present` |
| click a link or a button | [`01_first_hunt`](01_first_hunt.hunt), [`02_login`](02_login.hunt) | `CLICK the 'Sign in' button` |
| log in | [`02_login`](02_login.hunt) | `FILL 'Username' field with '…'` |
| check something is **not** there | [`02_login`](02_login.hunt) | `VERIFY that '…' is NOT present` |
| fill a form: text, dropdown, radio, checkbox | [`03_forms`](03_forms.hunt) | `TYPE`, `SELECT … from the '…' dropdown`, `CHECK`, `UNCHECK` |
| check a field's value, or that a button is disabled | [`03_forms`](03_forms.hunt) | `VERIFY '…' field has value '…'`, `VERIFY that '…' is disabled` |
| keep a value and reuse it | [`04_variables`](04_variables.hunt) | `@var:`, `SET`, `{name}` |
| read a number off the page | [`04_variables`](04_variables.hunt) | `EXTRACT the 'Cart total' into {total}` |
| pick one of several identical buttons | [`05_which_element`](05_which_element.hunt) | `NEAR '…'` |
| act on one row of a table | [`05_which_element`](05_which_element.hunt) | `INSIDE 'Team' row with '…'` |
| use the header link, not the footer one | [`05_which_element`](05_which_element.hunt) | `ON HEADER`, `ON FOOTER` |
| wait for a slow page | [`06_waiting`](06_waiting.hunt) | nothing — `VERIFY` waits; else `WAIT FOR '…' to disappear` |
| do something only sometimes | [`07_control_flow`](07_control_flow.hunt) | `IF … :`, `ELIF`, `ELSE` |
| click until something stops appearing | [`07_control_flow`](07_control_flow.hunt) | `WHILE button '…' exists:` |
| repeat steps, or loop over a list | [`07_control_flow`](07_control_flow.hunt) | `REPEAT 3 TIMES as {n}:`, `FOR EACH {x} IN {list}:` |
| run the same test for many users | [`08_data_driven`](08_data_driven.hunt) | `@data: data/users.csv` |
| write a login once and share it | [`09_reusable_steps`](09_reusable_steps.hunt), [`lib/account`](lib/account.hunt) | `@import:`, `USE`, `@export:` |
| run something before and after a test | [`09_reusable_steps`](09_reusable_steps.hunt) | `[SETUP]`, `[TEARDOWN]` |
| hover, double-click, right-click, drag | [`10_mouse_and_keyboard`](10_mouse_and_keyboard.hunt) | `HOVER`, `DOUBLE CLICK`, `RIGHT CLICK`, `DRAG … and drop it into …` |
| press a key | [`10_mouse_and_keyboard`](10_mouse_and_keyboard.hunt) | `PRESS Enter`, `PRESS Escape` |
| get past `alert()` / `confirm()` | [`10_mouse_and_keyboard`](10_mouse_and_keyboard.hunt) | nothing — they are answered OK |
| wait for an API call | [`11_network`](11_network.hunt) | `WAIT FOR RESPONSE "api/…"` |
| fake an API response | [`11_network`](11_network.hunt) | `MOCK GET "api/…" with 'mocks/….json'` |
| call my own code from a hunt | [`12_call_your_code`](12_call_your_code.hunt), [`hooks/`](hooks/manul_hooks.py) | `CALL HOST name into {var}`, `--hooks` |
| drive the browser from Python or Node | [`drivers/`](drivers/) | `Session.step()`, `read()`, `map()`, `run()` |

## The rules, in five lines

1. One action per line. Case does not matter: `CLICK`, `Click`, `click`.
2. What you mean goes in single quotes, as a person sees it on screen:
   `CLICK the 'Sign in' button`. No selectors.
3. The word after the quotes — `button`, `field`, `link`, `dropdown` — is a hint.
   It is optional, and it helps when two things share a label.
4. `STEP N: …` groups lines for the report. Under `IF`, `WHILE`, `REPEAT` and
   `FOR EACH`, the body is the lines indented beneath it.
5. `#` starts a comment. `DONE.` ends the file.

## From Python or Node

[`drivers/quickstart.py`](drivers/quickstart.py) and
[`drivers/quickstart.mjs`](drivers/quickstart.mjs) do the same thing in each
language: log in a step at a time, read a value, list what the page offers,
then run [`02_login.hunt`](02_login.hunt) whole in the same browser.

```bash
python drivers/quickstart.py
```

```bash
node drivers/quickstart.mjs
```

Go embeds the engine with no subprocess at all; see
[`core/examples/go`](../../core/examples/go/main.go).

## When a step fails

- `--explain` prints which element each step picked, and why.
- `--screenshot always` saves a picture after every step; failures are
  captured by default, into `screenshots/`.
- `--debug` stops before every step; `PAUSE` in a hunt stops at that line.
- A label that is on the page but still not found is usually a *different*
  text from what you typed — copy it from the screen, punctuation included.

## Where next

- [`core/docs/dsl-syntax.md`](../../core/docs/dsl-syntax.md) — every command.
- [`examples/python-demo`](../python-demo/) — one project that uses every
  feature: custom controls, suite hooks, page objects, the daemon, Firefox,
  attach mode.
