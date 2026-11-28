# speed-bench

The same test suite on Manul and on Playwright Test, timed against the same
local site in the same Chrome. The results and what they do and do not show
are in [`core/docs/speed.md`](../../core/docs/speed.md).

```bash
node bench.mjs                      # ROUNDS=5 WORKERS=1 MANUL=manul
ROUNDS=3 WORKERS=4 node bench.mjs
```

| Variable | Default |
|---|---|
| `ROUNDS` | `5` — each tool runs once per round, and which goes first changes every round |
| `WORKERS` | `1` — both tools run with this many workers |
| `MANUL` | `manul` on `PATH` |

| | Manul | Playwright |
|---|---|---|
| tests | `manul/tests/*.hunt` | `playwright/tests/*.spec.ts` |
| page objects | `manul/pages/*.hunt` | `playwright/pages/*.ts` |
| settings | `manul/manul.config.json` | `playwright/playwright.config.ts` |

26 tests, at least seven steps each and 185 in all, over the pages of
[`../quickstart/site`](../quickstart/site), which the benchmark serves itself
with caching off. Both suites run the installed Chrome (`channel: 'chrome'`
for Playwright), headless, with the same timeouts and no retries.

It writes the medians as Markdown to stdout and the raw numbers — every
run's wall time, every test's time, and Manul's time per command — to
`results/<date>-w<workers>.json`. `results/*-before.json` are the runs on the
engine from before its fixed pauses were removed.

`playwright/node_modules` must hold Playwright 1.66 (`npm install` in
`playwright/`). Here it is a link to `../token-bench/node_modules`, which
has the same Playwright, and is not committed.
