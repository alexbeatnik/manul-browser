# Token efficiency: Manul vs Playwright MCP

> **Manul Browser 0.1.6** — what it costs an LLM agent, in tokens, to see a
> page and act on it, measured against Playwright MCP on the same Chrome tab.
> Reproduce it with [`examples/token-bench`](../../examples/token-bench).

## In short

- **Seeing a page.** `manul map` is **1.7–18× smaller** than Playwright MCP's
  page snapshot on the pages measured, with its default budget of 8 elements
  per group. Without any budget it is **1.7–8.9× smaller**.
- **One task, end to end.** Sign in, add a product, open the cart and check
  it on Sauce Demo: **570 tokens through `manul mcp`, 916 through Manul's
  CLI, against 4,314 through Playwright MCP — 7.6× and 4.7× fewer**. All three
  finished the task.
- **Sent with every request.** Manul's six MCP tools and its instructions are
  **1,322 tokens**; Playwright MCP's 25 tools are **4,484**.
- **Why.** Playwright acts on elements through refs taken from a snapshot,
  and every action invalidates every ref. So the agent needs a new snapshot
  before each action that targets an element. Manul acts on a label (`CLICK
  the 'Cart' button`), so it looks at the page once per page, not once per
  action.
- **What it does not mean.** A snapshot holds the page's text and structure;
  `map` lists only labelled interactive elements. They are not the same
  information. See [What this does not show](#what-this-does-not-show).

## What was measured

| | |
|---|---|
| Date | 2026-11-28: pages, one action, what is sent with every request. 2026-11-26: the task |
| Playwright MCP | `@playwright/mcp` 0.0.86 (playwright-core 1.66.0-alpha-2026-11-18), default options |
| Manul | 0.1.6: its MCP server (`manul mcp`) and its agent CLI (`manul map`, `manul run-step`) |
| Browser | Chrome 158 (157 for the task), headless, 1366×768, en-US, password manager off |
| Tokenizer | `cl100k_base` (js-tiktoken) |
| Raw data | [`results/2026-11-28.json`](../../examples/token-bench/results/2026-11-28.json), run with `TASK=0`; the task from [`results/2026-11-26.json`](../../examples/token-bench/results/2026-11-26.json) |

Both tools attach to **one** Chrome over CDP. Playwright MCP and `manul mcp`
are each driven through their MCP server over stdio, exactly as a Claude app
calls them, and what is counted is the text content of each tool's reply,
which is what the model reads. Manul's CLI is called the way a shell-driven
agent calls it, one command per step, counting what it prints: JSON, where
`manul mcp` answers in plain language.

Claude's tokenizer is not public. Absolute counts differ between tokenizers;
the ratios are what this page is about. Two runs gave the same numbers to
within a few dozen tokens on the largest page.

## Seeing a page

What an agent feeds the model so it can decide what to do next.

| Page | Raw HTML | Playwright snapshot | `manul map`, full | `manul map` (default) | Snapshot ÷ map | Snapshot ÷ full map |
|---|---:|---:|---:|---:|---:|---:|
| Local fixture: form in iframes | 172 | 375 | 225 | 225 | 1.7× | 1.7× |
| Local fixture: order summary | 208 | 258 | 29 | 29 | 8.9× | 8.9× |
| the-internet: home | 4,397 | 5,707 | 1,681 | 334 | 17.1× | 3.4× |
| Sauce Demo: login | 711 | 304 | 110 | 110 | 2.8× | 2.8× |
| Books to Scrape: catalogue | 9,820 | 8,675 | 2,081 | 1,276 | 6.8× | 4.2× |
| Wikipedia: *Web browser* | 200,421 | 38,059 | 18,496 | 3,126 | 12.2× | 2.1× |
| GitHub: microsoft/playwright | 180,972 | 20,054 | 4,047 | 1,318 | 15.2× | 5.0× |
| MDN: `<button>` reference | 37,454 | 60,583 | 11,692 | 3,290 | 18.4× | 5.2× |

Reading the table:

- **`manul map` (default)** is what an agent gets without asking for more: at
  most 8 elements per landmark group, with the number left out reported as
  `truncated`. It was truncated on every public page except Sauce Demo.
- **`manul map`, full** (`--max-per-group 100000`) leaves nothing out, which
  is the like-for-like comparison with a snapshot.
- A budget can leave out the very element an agent needs. On the-internet,
  "Form Authentication" was not in the default map. `run-step` still clicked
  it, because Manul finds a target by its label rather than from the map, but
  an agent that had only the map would not have known the link was there.
- The map covers iframes as steps do. Until `manul mcp` was written it did
  not, so the iframe fixture's map was a third of its real size (67 tokens
  against 225) and this table was kinder to Manul than the page.
- Raw HTML is shown for scale. It includes scripts and styles, which no
  reasonable agent sends. A snapshot can still be larger than the HTML: on
  MDN it is 60,583 tokens against 37,454.

## One action

The reply to a single click, each tool from the same freshly loaded page.

| Page | Playwright `browser_click` | Manul `run-step CLICK` |
|---|---:|---:|
| Local fixture: form in iframes | 132 | 63 |
| the-internet: home | 196 | 56 |
| Sauce Demo: login | 123 | 55 |
| Books to Scrape: catalogue | 208 | 64 |
| Wikipedia: *Web browser* | 202 | 58 |
| GitHub: microsoft/playwright | 124 | 65 |
| MDN: `<button>` reference | 215 | 66 |

Both replies are small, and `manul mcp`'s are smaller still: a plain-language
line such as *"OK: CLICK the 'Login' button — the page is now …"* is 23–29
tokens. This version of Playwright MCP writes the new page's
snapshot to a file and replies with a link to it, instead of inlining it as
earlier versions did. The difference that matters is not in the reply. It is
what each tool needs before it can act:

- **Playwright** needs the element's `ref` from a snapshot of the page **as it
  is now**. After any action, the old refs are refused with *"Ref … not found
  in the current page snapshot. Try capturing new snapshot."*
- **Manul** needs the element's label, which does not change when the page
  does.

## One task, end to end

Sauce Demo: sign in, add the first product to the cart, open the cart, and
check the product is in it. Each tool got its cheapest honest route, and the
benchmark checked that each ended on the cart page with the product in it.

**Playwright MCP: 10 calls, 4,314 tokens.** It takes a snapshot only when it
needs refs it does not have, and checks with `browser_wait_for` rather than
another snapshot.

| # | Call | Tokens |
|---:|---|---:|
| 1 | `browser_navigate` | 111 |
| 2 | `browser_snapshot`: refs for the login form | 222 |
| 3 | `browser_type` username | 25 |
| 4 | `browser_type` password | 118 |
| 5 | `browser_click` Login | 114 |
| 6 | `browser_snapshot`: refs on the product page | 1,673 |
| 7 | `browser_click` Add to cart | 121 |
| 8 | `browser_snapshot`: the click above invalidated the refs | 1,686 |
| 9 | `browser_click` Cart | 114 |
| 10 | `browser_wait_for` "Sauce Labs Backpack" | 130 |

**Manul through `manul mcp`: 9 calls, 570 tokens.** It maps each new page
once and checks with `VERIFY`.

| # | Call | Tokens |
|---:|---|---:|
| 1 | `step` NAVIGATE to https://www.saucedemo.com/ | 29 |
| 2 | `map`: the login form | 76 |
| 3 | `step` FILL 'Username' field with 'standard_user' | 27 |
| 4 | `step` FILL 'Password' field with 'secret_sauce' | 28 |
| 5 | `step` CLICK the 'Login' button | 24 |
| 6 | `map`: the product page | 310 |
| 7 | `step` CLICK the 'Add to cart' button | 26 |
| 8 | `step` CLICK the 'Cart' button | 23 |
| 9 | `step` VERIFY that 'Sauce Labs Backpack' is present | 27 |

**Manul through its CLI: 9 calls, 916 tokens.** The same calls, answered in
JSON.

| # | Call | Tokens |
|---:|---|---:|
| 1 | `run-step NAVIGATE to https://www.saucedemo.com/` | 68 |
| 2 | `map`: the login form | 110 |
| 3 | `run-step FILL 'Username' field with 'standard_user'` | 67 |
| 4 | `run-step FILL 'Password' field with 'secret_sauce'` | 69 |
| 5 | `run-step CLICK the 'Login' button` | 58 |
| 6 | `map`: the product page | 370 |
| 7 | `run-step CLICK the 'Add to cart' button` | 60 |
| 8 | `run-step CLICK the 'Cart' button` | 60 |
| 9 | `run-step VERIFY that 'Sauce Labs Backpack' is present` | 54 |

Most of Playwright's cost is calls 6 and 8: two snapshots of the same product
page, the second only because a click made the first one's refs stale. On a
longer task the gap grows with every action that has to be preceded by a
snapshot. Each reply also stays in the conversation, so it is paid again as
input on every later turn.

## Sent with every request

| | Tools | Tokens |
|---|---:|---:|
| Playwright MCP: tool definitions | 25 | 4,484 |
| `manul mcp`: tool definitions | 6 | 1,134 |
| `manul mcp`: server instructions | | 188 |

The model receives these with every request, whatever the page, unless the
client caches them. The guide `suite_guide` returns is 3,043 tokens, and the
`build_test_suite` prompt, which carries it, 3,146; both are sent only when
the model or the person asks for them. Manul's CLI is not counted here: how
much describing it costs depends on how it is exposed, such as a shell tool
or a binding.

## What this does not show

- **A snapshot and a map hold different things.** The snapshot has the page's
  text, its headings and its structure. `map` has the labelled interactive
  elements and nothing else. An agent that has to read the page uses
  `manul read` or `ReadText` for that, at a cost of its own that is not
  measured here.
- **Playwright can act without a snapshot.** `browser_click` also takes "a
  unique element selector" instead of a ref. An agent that already knows the
  selectors can skip the snapshots. It has to have learned them somewhere,
  usually from a snapshot or from the page source.
- **One run, on pages that change.** The public sites change, so the numbers
  are dated. The two local fixtures do not change.
- **One task.** Sauce Demo is short and small. Pages the size of Wikipedia or
  MDN make each snapshot an order of magnitude dearer.
- **Output tokens are not counted.** The model's own tool calls cost tokens
  too: a ref and a selector for Playwright, a one-line step for Manul. They
  are small next to the replies.
- **Playwright MCP's defaults.** `--snapshot-mode none` and snapshot `depth`
  change what Playwright returns. Only the defaults were measured.

## Reproduce

```bash
cd examples/token-bench
npm install
MANUL=/path/to/manul node bench.mjs > results/$(date +%F).json
```

`TASK=0` leaves out the Sauce Demo task, which signs in. This needs Google
Chrome. Point `CHROME` at its binary outside macOS. The
benchmark serves the local fixtures from `core/e2e/testdata/site` itself; the
public pages need the network. Pages and click targets are listed in
`pages.json`.
