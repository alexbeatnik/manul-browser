# Speed: Manul vs Playwright

> **Manul Browser 0.1.6** — how long the same test suite takes on Manul and on
> Playwright Test, on the same local site in the same Chrome. Reproduce it
> with [`examples/speed-bench`](../../examples/speed-bench).

## In short

- **The whole suite.** 26 tests, 185 steps: **32.2 s on Manul, 40.9 s on
  Playwright** with one worker, **12.0 s and 16.8 s** with four. Every test
  passed on both, in every run.
- **It was 128.4 s.** Manul's actions slept a fixed 150–500 ms after they
  acted, whether the page did anything or not. That was three quarters of
  the suite's time. They no longer sleep; what an action sets off is waited
  for, and nothing else.
- **On a page that answers at once** the two are about even: 11.0 s and
  11.7 s for 22 tests. **On a page that is slow on purpose** Manul is faster,
  20.2 s against 27.2 s for the other four, because it looks again every
  200 ms where Playwright's checks back off to once a second.
- **Going faster found a bug.** A click measured just before the page moved
  landed where the element had been, and passed with nothing clicked. A
  press that lands on something else is now stopped in the page and made
  again where the element is.

## What was measured

| | |
|---|---|
| Date | 2026-11-28 |
| Machine | Apple M1, 8 cores, macOS 15.8 |
| Manul | 0.1.6, built from `feat/mcp-server` |
| Playwright | Playwright Test 1.66.0-alpha-2026-11-18, Node 26.13 |
| Browser | Chrome 158 from `/Applications`, new headless mode, for both |
| Site | [`examples/quickstart/site`](../../examples/quickstart/site), served by the benchmark with caching off |
| Raw data | [`examples/speed-bench/results/`](../../examples/speed-bench/results/) |

The suite is written twice, as `.hunt` files and as TypeScript, with page
objects on both sides: the same 26 tests, the same step titles, the same
actions and the same checks. Every test has at least seven steps, and each
step is one or more actions and a check of what they did. The tests cover
sign-in, a sign-up form, a shop with a cart and a team table, hover, double
and right click, drag, keys and a `confirm()`, a page whose content arrives
late, and an API answered by the server or by a mock.

Both run headless, one worker unless told otherwise, no retries, 5 s for an
element or a check and 30 s for a navigation. Each tool runs the way it is
meant to: Playwright opens a fresh browser context for each test. The whole
run is timed, browser start-up included, and so is each test. Each round
runs both tools, and which one goes first changes every round.

| | Manul | Playwright |
|---|---:|---:|
| lines in the tests | 795 | 955 |
| lines in page objects and config | 50 | 256 |

## Results

| Whole run | Manul before | Manul now | Playwright | Now ÷ Playwright |
|---|---:|---:|---:|---:|
| 1 worker, median of 5 | 128.4 s | 32.2 s | 40.9 s | 0.8× |
| 1 worker, fastest–slowest | 128.1–128.9 s | 32.0–32.2 s | 40.3–41.0 s | |
| 4 workers, median of 3 | 39.0 s | 12.0 s | 16.8 s | 0.7× |

"Before" is the engine as it was when the suite was first run; "now" is
after the change below. Before it, one 4-worker round had a Playwright test
run into its 60 s timeout as the first test of its worker, most likely while
Chrome was starting beside three others; it passed in every other round.

Per test, one worker, median of 5:

| Test | Manul before | Manul now | Playwright | Now ÷ Playwright |
|---|---:|---:|---:|---:|
| login_admin | 3.35 s | 0.38 s | 0.56 s | 0.7× |
| login_member | 5.61 s | 0.48 s | 0.51 s | 0.9× |
| login_wrong_password | 5.76 s | 0.48 s | 0.46 s | 1.0× |
| login_unknown_user | 6.01 s | 0.51 s | 0.52 s | 1.0× |
| login_switch_users | 5.99 s | 0.56 s | 0.51 s | 1.1× |
| form_free_plan | 2.87 s | 0.21 s | 0.37 s | 0.6× |
| form_pro_plan | 3.24 s | 0.29 s | 0.41 s | 0.7× |
| form_terms_gate | 2.42 s | 0.15 s | 0.42 s | 0.4× |
| form_resubmit | 4.84 s | 0.54 s | 0.49 s | 1.1× |
| form_countries | 6.25 s | 0.82 s | 0.57 s | 1.5× |
| shop_add_each | 4.19 s | 0.55 s | 0.47 s | 1.2× |
| shop_boots | 4.15 s | 0.54 s | 0.45 s | 1.2× |
| shop_team_edit | 4.09 s | 0.53 s | 0.46 s | 1.1× |
| shop_team_remove | 4.63 s | 0.63 s | 0.48 s | 1.3× |
| shop_help_links | 4.13 s | 0.55 s | 0.47 s | 1.2× |
| shop_from_home | 4.20 s | 0.39 s | 0.57 s | 0.7× |
| mouse_hover_copy | 3.36 s | 0.30 s | 0.43 s | 0.7× |
| mouse_menu_and_board | 3.73 s | 0.76 s | 0.65 s | 1.2× |
| keyboard_search | 4.46 s | 0.52 s | 0.36 s | 1.4× |
| mouse_everything | 4.04 s | 0.63 s | 0.61 s | 1.0× |
| slow_report | 6.92 s | 7.21 s | 8.73 s | 0.8× |
| slow_feed | 5.66 s | 4.48 s | 5.71 s | 0.8× |
| quotes_cycle | 6.17 s | 4.58 s | 6.99 s | 0.7× |
| quotes_mocked | 5.42 s | 3.89 s | 5.77 s | 0.7× |
| home_navigation | 6.50 s | 0.45 s | 0.90 s | 0.5× |
| cross_page_journey | 9.32 s | 0.68 s | 1.06 s | 0.6× |

## Where the time went

Before the change, on a page that answers at once, Manul was 6 to 13 times
slower. Its actions slept after they acted:

| After | Fixed pause | Per run | Slept in all |
|---|---:|---:|---:|
| CLICK | 500 ms | 130 | 65.0 s |
| FILL | 300 ms | 44 | 13.2 s |
| NAVIGATE | 300 ms, and up to 500 ms more | 26 | 7.8–20.8 s |
| PRESS | 300 ms | 9 | 2.7 s |
| SELECT, HOVER, DOUBLE / RIGHT CLICK, TYPE | 300 ms | 16 | 4.8 s |
| CHECK, UNCHECK | 150 ms | 14 | 2.1 s |

NAVIGATE's extra 500 ms was a wait in the CDP backend for `readyState` to
leave "complete", which a page that loads at once never does. The pauses
were older than `settleNavigation`, which waits for what an action actually
starts — a new document, the requests it sent, the DOM going quiet — and
they ran in front of it.

## What changed

- **No action sleeps.** `settleNavigation` gives an action 50 ms
  (`actionGrace`) to start something before it decides the action went
  nowhere, and stops looking as soon as something starts: a form that Enter
  submits navigates from a task of its own, and a framework sends its
  request from an effect after it renders.
- **NAVIGATE** marks the document before it leaves, and knows the new one by
  the mark it does not have.
- **CHECK and UNCHECK** read the box at once and look again, for up to
  150 ms, only if it is not yet as asked.
- **What a step looks for after an action** it waits for itself, as it waits
  for any target, for up to `timeout`.

| Command | Per run | Before | Now |
|---|---:|---:|---:|
| CLICK | 130 | 538 ms | 70 ms |
| NAVIGATE | 26 | 835 ms | 28 ms |
| FILL | 44 | 317 ms | 7 ms |
| PRESS | 9 | 311 ms | 60 ms |
| SELECT | 6 | 317 ms | 63 ms |
| HOVER | 4 | 331 ms | 16 ms |
| CHECK | 9 | 164 ms | 5 ms |
| VERIFY | 150 | 49 ms | 96 ms |
| WAIT FOR | 3 | 792 ms | 1,399 ms |

VERIFY and WAIT FOR take longer because the page's own delays — a report
that takes 1.5 s, a feed that loads after 300 ms — are now waited for by the
check that needs them, where before they passed inside the pauses.

The pauses that stay are on paths this suite does not take: a custom
dropdown, which looks for its options once right after it opens, and a field
inside a shadow root.

## The click that moved

The slow page removes its "Loading page..." line a second after it loads,
which moves everything under it up. Without the pauses, about one run in
four, the click on 'Save settings' was measured just before that and pressed
just after. The press landed on the button's own section, where the button
had been, and the step passed with nothing clicked. Before, the pauses made
the engine slow enough not to meet it.

The pointer watch now knows which element a press is for. A press whose
target is not the element, nor inside it, nor its `<label>` — and, when it is
something the element is inside, one that lands outside the element's own
box — is stopped in the page, every event of it, before any of the page's
handlers see it. Then the engine measures the element again and clicks where
it is now, up to three times. An element that is gone from the document by
the time of the press replaced itself under the pointer, as a link that
redraws on hover does, and the press goes through.

In 10 runs after the change the watch caught the moved button twice, and
every run passed. [`core/e2e`](../e2e) has a page for each way it happens:
`moved.html`, where a notice pushes the button down as the pointer arrives,
and `moved_up.html`, where a line above it goes. Both fail on the engine
from before the change, on Chrome and Firefox.

## Slow pages

Four tests wait 1–1.5 s for content that arrives late: a report, a feed, a
toast, a quote fetched after a delay. Playwright's `expect` looks again
after 100, 250 and 500 ms and then every 1,000 ms, so it notices late; Manul
looks every 200 ms. Those four took 20.2 s on Manul and 27.2 s on
Playwright.

## Firefox

Playwright drives a Firefox of its own build, not an installed one, so the
comparison is on Chrome only. On the installed Firefox the Manul suite runs
in 37–38 s, all 26 tests passing. Running it there found two bugs that were
older than this change; both are fixed:

- **A drag left text selected, and the next drag dragged the text.** The
  pointer, crossing the board with the button down, selected the second
  tile's text; on Firefox a press on selected text starts a drag of that
  text, which takes the mouse events, and the second tile never let go.
  DRAG now clears the selection before the pointer goes down
  (`core/e2e` `board.html`).
- **A MOCK outlived its hunt.** Each hunt of a run gets a page of its own on
  the same tab. Over BiDi every page shares one connection, and the
  intercept a MOCK added stayed on the tab, answering from its rules: every
  hunt after `quotes_mocked` read the mocked quotes. A page now removes its
  intercept when it closes (`TestMocks_EndWithTheirHunt`).

## What this does not show

- **One machine, one local site.** The site answers in a millisecond. On a
  real site every step also waits for the network, the same for both tools,
  and the difference between them is a smaller share of the whole.
- **One suite.** It is written to be the same on both sides, but their
  locators are not the same thing: Playwright needed `exact: true` where
  'Board' is also inside 'Keyboard', a CSS class where the hunt says
  `NEAR 'Manul Plush'`, and a dialog handler where Manul accepts `confirm()`
  by itself.
- **Defaults.** Each tool was run the way it is set up out of the box, apart
  from the shared timeouts.
- **Not the time to write the tests.** Only the time to run them.

## Reproduce

```bash
cd examples/speed-bench
node bench.mjs                              # ROUNDS=5 WORKERS=1 MANUL=manul
ROUNDS=3 WORKERS=4 node bench.mjs
```

It needs Google Chrome and Playwright in `playwright/node_modules`; the
results go to `results/`.
