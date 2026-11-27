# Integration

> **Manul Browser 0.1.6** — embedding the engine in Go programs, CI/CD pipelines, and agent stacks.

## Go Embedding API (`pkg/agent`)

`agent.Session` is the programmatic face of the engine — it owns the browser lifecycle and routes every call through the full heuristic pipeline. No selectors, no raw CDP in your code:

```go
import "github.com/alexbeatnik/manul-browser/core/pkg/agent"

sess, err := agent.Connect(ctx, agent.Options{Port: 9222}) // attach if Chrome is up, else launch & own it
if err != nil { … }
defer sess.Close()

out, _  := sess.Step(ctx, "Click the 'Login' button")   // StepOutcome{OK, Reason, Score, Near}
val, _  := sess.Read(ctx, "Order total")                 // zero-scan targeted read
txt, _  := sess.ReadText(ctx, "#answer")                 // sanitized region text
pm, _   := sess.Map(ctx, agent.MapBudget{MaxPerGroup: 8})
res, _  := sess.Run(ctx, huntScript)                     // whole .hunt → RunOutcome
```

| Concern | Answer |
|---|---|
| Lifecycle | `Launch` owns Chrome (Close reaps it); `Attach` doesn't; `Connect` = attach-or-launch |
| Concurrency | one `Session` = one page = one goroutine; use `pkg/worker` for parallel suites |
| Failures | typed `Reason` + `Near` candidates — branch on values, not error strings |
| LLM output | `pm.RenderForLLM(n)`, `out.DescribeForLLM()`, `res.RenderForLLM()`, `agent.DescribePageChange(before, after)` |

The full surface is frozen in `contracts/MANUL_API_CONTRACT.md`. The Python sibling exposes the equivalent via `ManulSession` (async context manager).

### Parallel suites

```go
results, err := worker.RunHuntsInParallel(ctx, cfg, hunts, 4, logger)
```

One `Runtime` + `Page` + `ChromeProcess` per worker; register `CALL GO` handlers and custom controls **before** spawning the pool. Verified with `go test -race`.

---

## MCP server (Claude Desktop, Claude Code)

`manul mcp` serves the engine to any client of the Model Context Protocol over stdio. It gives five tools: `step` (one line of the DSL), `map` (the labelled controls of the page, iframes included), `read` (one labelled value), `read_text` (prose, by CSS region), `run_hunt` (a whole script) and `suite_guide` (how to build a test suite on Manul: layout, configuration, shared blocks, environments and secrets, CI, with an example project the e2e suite runs). The `build_test_suite` prompt starts that work: in Claude Code it is `/mcp__manul-browser__build_test_suite <url>`, in Claude Desktop the connector's entry in the attach menu. The browser starts with the first tool call that needs one and closes when the client goes away; `step` and `run_hunt` are marked destructive, so a client that asks before acting asks before them.

**Claude Desktop** — in `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "manul-browser": { "command": "/usr/local/bin/manul", "args": ["mcp"] }
  }
}
```

**Claude Code**:

```bash
claude mcp add manul-browser -- manul mcp
```

Add flags after `mcp` as needed:

| Flag | |
|---|---|
| `--browser firefox` | Firefox over WebDriver BiDi instead of Chrome |
| `--headless` | no window (the default shows the browser, so you can watch it work) |
| `--user-data-dir <dir>` | a profile kept between sessions, so sign-ins survive |
| `--cdp http://127.0.0.1:9222` | drive a Chrome you started yourself instead of launching one |
| `--locale uk-UA` | the language the browser asks pages for |

Logs go to stderr, which Claude Desktop keeps in its MCP log. One caution for `--cdp` against your own browser: with its password manager on, a click after a sign-in can be taken by the manager's own UI. The step notices that the page never received the click, tries once more, and then fails saying so rather than passing ([ROADMAP](../../ROADMAP.md) §missing 8); a profile the engine launches has the manager off. What each tool returns is in [`MANUL_MCP_CONTRACT.md`](../../spec/contracts/MANUL_MCP_CONTRACT.md), and what it costs in tokens against Playwright MCP is in [token-efficiency.md](token-efficiency.md).

---

## Agent commands (external LLM drivers)

For a driver that keeps one Chrome open and issues stateless calls:

```bash
google-chrome --remote-debugging-port=9222 &

manul schema                                  # DSL grammar + JSON shapes (no browser)
manul map --cdp http://127.0.0.1:9222         # landmark-grouped page map
manul run-step "Click the 'Login' button" --cdp http://127.0.0.1:9222
manul read 'Order total' --cdp http://127.0.0.1:9222
```

JSON payload on **stdout**, logs on **stderr**; non-zero exit when a step fails. Pin `manul schema` in the driver instead of prose docs. See [dsl-for-llms.md](dsl-for-llms.md).

---

## CI/CD

The engine is a single static binary — no interpreter or `node_modules` in the image; only system Chrome is needed.

### GitHub Actions

```yaml
jobs:
  e2e:
    runs-on: ubuntu-latest        # GitHub-hosted runners ship Google Chrome
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.26' }
      - run: go build -o manul ./cmd/manul
      - run: ./manul --headless --html-report --screenshot on-fail tests/
      - uses: actions/upload-artifact@v4
        if: always()
        with: { name: manul-reports, path: reports/ }
```

### Exit codes & machine output

| Signal | Meaning |
|---|---|
| exit `0` | all hunts passed |
| exit non-zero | at least one hunt/step failed |
| `--json` | full `HuntResult` JSON on stdout |
| `--jsonl` | per-step JSON Lines stream + final result |
| `reports/run_history.json` | JSONL, one `{file,name,timestamp,status,duration_ms}` entry per run |

### Containers

Any image with Go (build stage) + Chrome (run stage) works:

```dockerfile
FROM golang:1.26 AS build
WORKDIR /src
COPY . .
RUN go build -o /manul ./cmd/manul

FROM debian:stable-slim
RUN apt-get update && apt-get install -y chromium && rm -rf /var/lib/apt/lists/*
COPY --from=build /manul /usr/local/bin/manul
ENV MANUL_HEADLESS=true MANUL_BROWSER_ARGS=--no-sandbox
ENTRYPOINT ["manul"]
```

---

## Scheduling (synthetic monitoring)

```hunt
@schedule: every 5 minutes
```

```bash
manul daemon hunts/ --headless
```

The daemon watches the directory for `@schedule:`-tagged hunts and runs them on their cadence.

