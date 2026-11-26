# Token efficiency: Manul vs Playwright MCP

> **Manul Browser 0.1.6** — what it costs an LLM agent, in tokens, to see a
> page and act on it, measured against Playwright MCP on the same Chrome tab.
> Reproduce it with [`examples/token-bench`](../../examples/token-bench).

## In short

- **Seeing a page.** `manul map` is **2.8–21× smaller** than Playwright MCP's
  page snapshot on the pages measured, with its default budget of 8 elements
  per group. Without any budget it is **2.1–8.9× smaller**.
- **One task, end to end.** Sign in, add a product, open the cart and check
  it on Sauce Demo: **916 tokens with Manul against 4,314 with Playwright MCP
  (4.7× fewer)**. Both finished the task.
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
| Date | 2026-11-26 |
| Playwright MCP | `@playwright/mcp` 0.0.86 (playwright-core 1.66.0-alpha-2026-11-18), default options |
| Manul | 0.1.6, its agent CLI: `manul map`, `manul run-step` |
| Browser | Chrome 157, headless, 1366×768, en-US, password manager off |
| Tokenizer | `cl100k_base` (js-tiktoken) |
| Raw data | [`examples/token-bench/results/2026-11-26.json`](../../examples/token-bench/results/2026-11-26.json) |

Both tools attach to **one** Chrome over CDP and read the **same tab**.
Playwright MCP is driven through its own MCP server over stdio, exactly as an
agent calls it. What is counted is the text content of each tool's reply,
which is what the model reads. Manul is called as an agent calls it: one CLI
command per step, counting what it prints.

Claude's tokenizer is not public. Absolute counts differ between tokenizers;
the ratios are what this page is about. Two runs gave the same numbers to
within a few dozen tokens on the largest page.

## Seeing a page

What an agent feeds the model so it can decide what to do next.

| Page | Raw HTML | Playwright snapshot | `manul map`, full | `manul map` (default) | Snapshot ÷ map | Snapshot ÷ full map |
|---|---:|---:|---:|---:|---:|---:|
| Local fixture: form in iframes | 172 | 375 | 67 | 67 | 5.6× | 5.6× |
| Local fixture: order summary | 208 | 258 | 29 | 29 | 8.9× | 8.9× |
| the-internet: home | 4,397 | 5,706 | 1,681 | 334 | 17.1× | 3.4× |
| Sauce Demo: login | 711 | 303 | 110 | 110 | 2.8× | 2.8× |
| Books to Scrape: catalogue | 9,820 | 8,675 | 2,081 | 1,276 | 6.8× | 4.2× |
| Wikipedia: *Web browser* | 199,072 | 38,008 | 18,476 | 3,126 | 12.2× | 2.1× |
| GitHub: microsoft/playwright | 182,157 | 20,203 | 4,021 | 1,321 | 15.3× | 5.0× |
| MDN: `<button>` reference | 37,466 | 60,581 | 11,346 | 2,944 | 20.6× | 5.3× |

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
- Raw HTML is shown for scale. It includes scripts and styles, which no
  reasonable agent sends. A snapshot can still be larger than the HTML: on
  MDN it is 60,581 tokens against 37,466.

## One action

The reply to a single click, each tool from the same freshly loaded page.

| Page | Playwright `browser_click` | Manul `run-step CLICK` |
|---|---:|---:|
| Local fixture: form in iframes | 131 | 63 |
| the-internet: home | 194 | 56 |
| Sauce Demo: login | 122 | 55 |
| Books to Scrape: catalogue | 206 | 64 |
| Wikipedia: *Web browser* | 200 | 58 |
| GitHub: microsoft/playwright | 123 | 65 |
| MDN: `<button>` reference | 213 | 66 |

Both replies are small. This version of Playwright MCP writes the new page's
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
benchmark checked that both ended on the cart page with one item in it.

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

**Manul: 9 calls, 916 tokens.** It maps each new page once and checks with
`VERIFY`.

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

Playwright MCP's 25 tool definitions are **4,484 tokens**. The model receives
them with every request, whatever the page, unless the client caches them.
Manul is not an MCP server: how much describing it costs depends on how it is
exposed, such as a shell tool or a binding, so that cost is not counted here.
`manul schema` prints the agent API's own description, about 5.7k characters.

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

This needs Google Chrome. Point `CHROME` at its binary outside macOS. The
benchmark serves the local fixtures from `core/e2e/testdata/site` itself; the
public pages need the network. Pages and click targets are listed in
`pages.json`.
