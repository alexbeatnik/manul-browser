# token-bench

What it costs an LLM agent, in tokens, to see a page and act on it, with
[Playwright MCP](https://github.com/microsoft/playwright-mcp) and with Manul,
both attached to the same Chrome tab. The results and what they do and do not
show are in [`core/docs/token-efficiency.md`](../../core/docs/token-efficiency.md).

```bash
npm install
MANUL=/path/to/manul node bench.mjs > results/$(date +%F).json
```

| Variable | Default |
|---|---|
| `MANUL` | `manul` on `PATH` |
| `CHROME` | `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome` |
| `TASK` | `0` leaves out the Sauce Demo task, which signs in |

It measures:

- **Each page in `pages.json`.** Raw HTML, Playwright's `browser_snapshot`,
  and `manul map` with its default budget and without one. Where the page
  names a click target, it also measures the reply to that click from each
  tool.
- **One task end to end, on Sauce Demo.** Sign in, add a product, open the
  cart and check it, with each tool's cheapest honest sequence of calls.
- **Overhead.** Playwright MCP's tool definitions, which are sent with every
  request.

Tokens are counted with `cl100k_base`. The local pages come from
`core/e2e/testdata/site` and are served by the benchmark itself; the public
ones need the network and change over time, so keep the date in the file
name. Chrome runs with a fresh profile with its password manager off. Left on,
the manager shows its own UI after a login and takes the clicks meant for the
page, for both tools alike.
