# Building a test suite on Manul

Use this when someone wants tests that live in their repository and run again
on every change — in CI, before a release — rather than a browser driven once.
What you produce is a directory of `.hunt` files, a `manul.config.json`, shared
blocks, test data and a CI job, all run by the `manul` command.

## 1. Learn the app with the browser first

Do not write steps from guesses. Open the app with the `step` tool, call `map`,
and walk each journey you will test, one `step` at a time. Labels in a hunt are
the text a person sees, copied exactly from `map` — punctuation, case and
apostrophes included. When a journey works step by step, it becomes a test.

Ask, or find out, before you start:

- the address of the app for local runs and for CI (a staging URL, or a server
  the CI job starts itself);
- test accounts, and which of their values are secret;
- the journeys that matter most: they are the `smoke` tests.

A site behind bot protection (Cloudflare and the like) refuses any automated
browser. For the app's own tests that is the app's own setting: test against a
staging host where it is off, or allow the CI runner through. Never try to get
around protection on a site that is not the user's.

## 2. Layout

```text
manul.config.json        how every run starts the browser
lib/env.hunt             defaults that differ between environments
pages/*.hunt             page objects: a page's name, address, how to tell
                         it is open, and what a person does there
tests/*.hunt             one journey per file; the run target
data/*.csv               rows for data-driven tests
mocks/*.json             canned API responses, if any
.github/workflows/manul.yml
```

- `manul run tests/` runs the `.hunt` files **directly in** `tests/`, not in
  its subdirectories. Keep tests flat and group them with `@tags:`, or name
  several targets: `manul run tests/ more-tests/` and `manul run tests/*.hunt`
  run them all.
- `lib/` and `pages/` are not under `tests/`: their blocks are not tests, and
  a run of `tests/` must not execute them.
- Paths in `@import:` and `@data:` are relative to the file that names them.

## 3. Configuration

`manul.config.json` is read from the directory `manul` runs in. Unknown keys
are ignored silently, so use only these:

```json manul.config.json
{
  "browser": "chromium",
  "headless": false,
  "timeout": 5000,
  "nav_timeout": 30000,
  "screenshot": "on-fail",
  "retries": 0,
  "workers": 1,
  "locale": "en-US"
}
```

`timeout` is how long a step waits for its target to appear (milliseconds);
`nav_timeout` bounds a navigation. A flag wins over the file and `MANUL_*`
environment variables: `--headless`, `--browser firefox`, `--workers 4`,
`--retries 1`, `--timeout 10s`.

## 4. Environments and secrets

A value that changes between environments is not written into a test. It has a
default in `lib/env.hunt`, every test imports that file, and a run replaces the
default with `--var name=value` or the environment variable
`MANUL_VAR_<name>`. Secrets have no default at all: they come only from the
environment, so they are never committed.

```hunt lib/env.hunt
@title: env
@context: Defaults for a local run. A run replaces them: --var site=https://staging.example.com or MANUL_VAR_site.
@var: {site} = http://localhost:8000
@var: {user} = ada
```

```bash
MANUL_VAR_password=… manul run tests/ --var site=https://staging.example.com
```

A run's value replaces a hunt's own `@var:` of the same name too; a row of
`@data:` replaces both.

## 5. Pages

A page object is a file in `pages/`: the page's name, its address, what it
shows when it is open, and what a person does there as blocks.

```hunt pages/login.hunt
@page: Login
@url: /login.html
@expect: 'Sign in'
@context: The sign-in page, and signing in and out.
@export: SignIn, SignOut

STEP SignIn:
    NAVIGATE to 'Login'
    FILL 'Username' field with '{user}'
    FILL 'Password' field with '{password}'
    CLICK the 'Sign in' button
    VERIFY that 'Welcome, {user}' is present

STEP SignOut:
    CLICK the 'Logout' button ON HEADER
    VERIFY that the page is 'Login'
```

```hunt pages/shop.hunt
@page: Shop
@url: /shop.html
@expect: 'Cart total'
@context: The shop, and putting a product in the cart.
@export: Add

STEP Add:
    CLICK the 'Add to cart' button NEAR '{product}'
    VERIFY that 'Added {product}' is present
```

What that gives every hunt, with no `@import`:

- `USE Login.SignIn` runs the page's block; a block may `USE` another of its
  own page's blocks by its short name. The page's `@var:` are defaults.
- `NAVIGATE to 'Shop'` opens `@url` on the site open now, or on `{site}` from
  a blank page. `@url` is a path (also written `{site}/shop.html`), or a whole
  address for a page on a site of its own.
- `VERIFY that the page is 'Login'` waits until the page is at its `@url` and
  shows every `@expect` text — so a single-page app's views, which share one
  address, are told apart by `@expect` alone when `@url` is left out.
  `IF the page is 'Login':` asks the same at one look.
- Every step's result names the page it ended on.

A page with no actions can be named in JSON instead: `pages/app.json` with
`{"site": "*", "/about.html": "About"}`. A key that is a plain path matches
that path and the ones under it; any other key is a regular expression. A
fragment with a site of its own (`"site": "https://admin.example.com/"`)
comes before the `*` one.

From Python or Node, a page object is `manul.Page` (`Page` in Node) with the
same name: `class Login(manul.Page): name = "Login"`, then
`Login(session).open().use("SignIn", user="ada", password=…)` — `open()` is
`NAVIGATE to 'Login'` and `VERIFY that the page is 'Login'`.

## 6. Shared blocks

A flow that is not one page's goes in `lib/`: a library names its blocks
`STEP <Name>:`, lists them in `@export:`, and a test imports them —
`@import: Checkout from '../lib/flows.hunt'`, then `USE Checkout`.

## 7. Tests

One journey per file, and each one stands alone: it starts with `NAVIGATE` or
a `USE` that does, and never relies on another test having run first.

```hunt tests/login.hunt
@title: login
@context: A member signs in, sees their role, and signs out.
@tags: smoke
@import: * from '../lib/env.hunt'

STEP 1: Sign in
    USE Login.SignIn
    VERIFY 'Role' field has value 'admin'
    VERIFY that the page is NOT 'Login'

STEP 2: Sign out
    USE Login.SignOut

DONE.
```

```hunt tests/login_rejected.hunt
@title: login rejected
@context: A wrong password is refused and says so.
@tags: regression
@import: * from '../lib/env.hunt'

STEP 1: Try a wrong password
    NAVIGATE to 'Login'
    FILL 'Username' field with '{user}'
    FILL 'Password' field with 'not-the-password'
    CLICK the 'Sign in' button
    VERIFY that 'Invalid username or password' is present
    VERIFY that 'Welcome' is NOT present
    VERIFY that the page is 'Login'

DONE.
```

```hunt tests/cart.hunt
@title: cart
@context: Two products in the cart add up to the total shown.
@tags: smoke
@import: * from '../lib/env.hunt'

STEP 1: Add two products
    NAVIGATE to 'Shop'
    VERIFY that the page is 'Shop'
    SET {product} = Manul Plush
    USE Shop.Add
    SET {product} = Snow Boots
    USE Shop.Add
    VERIFY that 'Cart: 2' is present

STEP 2: Check the total
    VERIFY 'Cart total' field has value '114.49'

DONE.
```

A data-driven test runs once per row; each column is a variable. Rows are for
test accounts and inputs, not for real secrets.

```text data/users.csv
user,password,role
ada,lovelace,admin
grace,hopper,member
```

```hunt tests/roles.hunt
@title: roles
@context: Every test account signs in and sees its own role.
@tags: regression
@import: * from '../lib/env.hunt'
@data: ../data/users.csv

STEP 1: Sign in as {user}
    USE Login.SignIn
    VERIFY 'Role' field has value '{role}'

DONE.
```

How to write the steps:

- `STEP N: …` per thing a person does; it is what the report shows.
- Check the outcome after each action that matters, with `VERIFY that '…' is
  present`, `… is NOT present`, `VERIFY '…' field has value '…'`.
- A step waits for a target that has not appeared yet, and an action waits
  for the requests it started and the page they lead to. Use `WAIT FOR '…'
  to be visible` for something that comes later still; never `WAIT 3`.
- When a label repeats, narrow it: `NEAR 'Manul Plush'`,
  `INSIDE 'Team' row with 'Grace Hopper'`, `ON HEADER`, `ON FOOTER`. Never
  count positions.
- The word after the quotes — `button`, `link`, `field`, `dropdown`,
  `checkbox` — is a hint that helps when two kinds share a label.
- `MOCK GET "api/quotes" with 'mocks/quotes.json'` answers a request with a
  file, for a state the real backend cannot be put in on demand.
- `[SETUP]` … `[END SETUP]` and `[TEARDOWN]` … `[END TEARDOWN]` run before and
  after a test's steps, a teardown even when a step failed.
- `#` starts a comment; `DONE.` ends the file.

## 8. Run it

Run every test you write, and the whole suite at the end:

```bash
manul run tests/login.hunt              # one test, in a window
manul run tests/ --headless             # the suite
manul run tests/ --tags smoke           # the smoke tests only
manul run tests/ --headless --workers 4 # one browser per worker
manul run tests/ --html-report          # reports/report_<title>_….html
manul run tests/ --json                 # one result document per test
```

The exit code is non-zero when a test failed. `--retries 1` runs a failed test
once more and marks it flaky if it passes then. On a failure, a screenshot is
saved under `screenshots/`; `--explain` prints which element each step chose
and why. If a label stopped matching, open the page and `map` it again: the
app's text changed, not the engine.

## 9. CI

`pip install manul-browser` installs the `manul` command; `npm install -g
manul-browser` does too. GitHub's Ubuntu runners have Chrome.

```yaml .github/workflows/manul.yml
name: manul
on: [push, pull_request]
jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with:
          python-version: "3.12"
      - run: pip install manul-browser
      - run: manul run tests/ --headless --retries 1 --html-report
        env:
          MANUL_VAR_site: ${{ vars.STAGING_URL }}
          MANUL_VAR_password: ${{ secrets.TEST_PASSWORD }}
      - if: always()
        uses: actions/upload-artifact@v4
        with:
          name: manul-reports
          path: |
            reports/
            screenshots/
```

If the app is not deployed anywhere, start it in the job before `manul run`
and point `MANUL_VAR_site` at it.

```text .gitignore
reports/
screenshots/
```
