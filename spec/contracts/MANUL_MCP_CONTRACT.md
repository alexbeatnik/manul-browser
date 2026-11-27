# Manul Browser — MCP Server Contract

> **Machine-readable contract for `manul mcp`, the engine served to clients of the Model Context Protocol** — Claude Desktop, Claude Code, and any other MCP client — over stdio.
>
> It is a protocol front, like `manul serve`: every tool is a thin call into the `pkg/agent` API (`MANUL_API_CONTRACT.md`), and its results are that API's plain-language renderings (`StepOutcome.DescribeForLLM`, `RunOutcome.RenderForLLM`, `PageMap.RenderForLLM`). Nothing about scoring, the DSL or the browser is decided by the MCP layer.
>
> `pkg/mcp`'s tests check that `tools/list` matches the `tools` block below: names, titles, parameters and annotations.

```json
{
  "version": "0.1.6",
  "generatedFrom": "pkg/mcp/server.go, pkg/mcp/tools.go, cmd/manul/mcp_cmd.go",

  "transport": {
    "kind": "stdio",
    "framing": "JSON-RPC 2.0, one message per line (UTF-8, newline-terminated, no embedded newlines). Batches are not accepted.",
    "stdout": "Carries protocol messages and nothing else.",
    "stderr": "Carries every log line: the client, the browser launch, the engine's step logs. --verbose raises the level.",
    "maxMessageBytes": 10485760
  },

  "protocolVersions": ["2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"],
  "versionNegotiation": "initialize answers with the protocolVersion the client asked for when it is one of protocolVersions, and with the first (newest) otherwise; the client decides whether it can continue.",

  "connectorName": "manul-browser — the name the server reports, and the one the documentation registers it under (claude mcp add manul-browser, mcpServers.manul-browser). A client shows it, and names the tools after it: mcp__manul-browser__step.",
  "initializeResult": {
    "capabilities": { "tools": { "listChanged": false }, "prompts": { "listChanged": false } },
    "serverInfo": { "name": "manul-browser", "title": "Manul Browser", "version": "<engine version, e.g. 0.1.6>", "websiteUrl": "https://github.com/alexbeatnik/manul-browser" },
    "instructions": "A short paragraph on how the tools fit together: map to see the labelled controls, step with one line naming one of them, map again only on a new page, VERIFY to check, read/read_text to read, run_hunt for several steps; iframes and new tabs need nothing special; suite_guide before building a test suite."
  },

  "methods": {
    "initialize": "As above.",
    "ping": "Answers {}.",
    "tools/list": "The tools below, in this order. Listing them starts no browser.",
    "tools/call": "Runs one tool. Calls run one at a time, in the order they arrived; ping, tools/list and the prompt methods are answered while a call runs.",
    "prompts/list": "The prompts below. Listing them starts no browser.",
    "prompts/get": "One prompt, rendered with its arguments, as one user message. Starts no browser.",
    "notifications/initialized": "Accepted; no answer.",
    "notifications/cancelled": "Cancels the call with that requestId, running or queued. A cancelled call is not answered.",
    "otherRequests": "JSON-RPC error -32601 (method not found).",
    "otherNotifications": "Ignored."
  },

  "errors": {
    "protocol": [
      { "code": -32700, "when": "A line that is not a JSON object. id is null." },
      { "code": -32600, "when": "No jsonrpc \"2.0\" or no method." },
      { "code": -32601, "when": "A request method the server does not have." },
      { "code": -32602, "when": "tools/call with no name, or a name that is not a tool; prompts/get with no name, a name that is not a prompt, or without an argument it requires." }
    ],
    "tool": "Everything else is the tool's own answer, with isError true and text that says why: a failed step (with the closest real labels when the scorer saw some), a label that matches nothing, arguments that do not fit the schema, a browser that could not be started. These are meant for the model to read and correct, so they are never protocol errors."
  },

  "session": {
    "opening": "The browser is opened by the first tools/call of a tool that drives it — every tool but suite_guide — — launched, or attached to when browser_mode resolves to attach (--cdp, --attach, MANUL_BROWSER_MODE, manul.config.json). A browser that cannot be started fails that call with isError and is tried again by the next.",
    "closing": "At end of input, or on SIGINT/SIGTERM, a call still running or queued is cancelled and the session is closed: a launched browser and its temp profile are removed; an attached browser is left running.",
    "browserGone": "When a call fails because the browser's connection closed (its window was closed, the process died), the session is opened afresh once and the call run again; the answer starts with a note that pages that were open are gone.",
    "state": "One session per server: one runtime, so DSL variables (EXTRACT … into {x}, SET) and the current tab carry from call to call, as in manul serve."
  },

  "configuration": {
    "precedence": "flags > MANUL_* environment > manul.config.json in the working directory > defaults (MANUL_CONFIG_CONTRACT.md)",
    "flags": [
      { "flag": "--browser",         "description": "chromium (default, CDP) or firefox (WebDriver BiDi)." },
      { "flag": "--headless",        "description": "Run a launched browser without a window. Default: a visible window." },
      { "flag": "--cdp",             "description": "Drive an already-running browser at this endpoint instead of launching one; implies --attach." },
      { "flag": "--attach",          "description": "Drive an already-running browser at the configured endpoint." },
      { "flag": "--channel",         "description": "Browser channel to launch, as for manul run." },
      { "flag": "--executable-path", "description": "Absolute path of the browser to launch." },
      { "flag": "--user-data-dir",   "description": "Browser profile directory, kept between sessions — sign-ins survive. Default: a fresh temp profile each time." },
      { "flag": "--locale",          "description": "Language the browser uses and asks pages for. Default en-US." },
      { "flag": "--verbose",         "description": "Verbose logging on stderr." }
    ],
    "forced": "The debugger is off (it would use stdin and stdout), and so are HTML reports and per-step screenshots (the client's working directory may not be writable). A SCREENSHOT step still writes its file."
  },

  "tools": [
    {
      "name": "step",
      "title": "Run a step",
      "parameters": { "step": { "type": "string", "required": true } },
      "annotations": { "readOnlyHint": false, "destructiveHint": true, "idempotentHint": false, "openWorldHint": true },
      "does": "agent.Session.Step on one line of the DSL (MANUL_DSL_CONTRACT.md). The description lists the common verbs with examples and the qualifiers (NEAR, INSIDE … row with, ON HEADER/FOOTER).",
      "result": "StepOutcome.DescribeForLLM: 'OK: <step> — the page is now <url>', or '<page name> (<url>)' when pages/ names the page, with a warning and the closest labels after a weak match; 'FAILED: <step>' with the reason and the closest labels otherwise, isError true. A target that is not there (not_found) says nothing was done, that a label taken from map means the page has changed since, and to open what hides it first; one that matches only in part and about equally (ambiguous) says manul did not guess and to quote an exact label. Both end with the page open now. Neither says the label was made up or too vague: on Comfy the label came from map, and on YouTube 'All' was precise, only absent.",
      "refuses": "An empty step, and one with several lines (use run_hunt)."
    },
    {
      "name": "map",
      "title": "See the page",
      "parameters": {
        "max_per_group":     { "type": "integer", "required": false, "default": 8 },
        "include_unlabeled": { "type": "boolean", "required": false, "default": false }
      },
      "annotations": { "readOnlyHint": true, "destructiveHint": false, "idempotentHint": true, "openWorldHint": true },
      "does": "agent.Session.Map: the labelled interactive elements, grouped by landmark, the page's own and those of every iframe the engine enters, whose groups are named '<group> (in frame <host>)'.",
      "result": "PageMap.RenderForLLM, under 'Current page: <url>' ('<page name> (<url>)' when pages/ names it): one line per element, \"'<label>' [<role> — <verb>]\", and '… +N more' for a group cut at max_per_group. A page with none says so, and how to go on.",
      "refuses": "A negative max_per_group."
    },
    {
      "name": "read",
      "title": "Read a value",
      "parameters": { "target": { "type": "string", "required": true } },
      "annotations": { "readOnlyHint": true, "destructiveHint": false, "idempotentHint": true, "openWorldHint": true },
      "does": "agent.Session.Read: EXTRACT of one labelled value, a field's current value, or a table cell by row and column; iframes included.",
      "result": "The value itself; '(empty)' for a field that holds nothing; isError with 'Nothing on the page is labelled …' when nothing matches.",
      "refuses": "An empty target."
    },
    {
      "name": "read_text",
      "title": "Read the text",
      "parameters": {
        "selector":  { "type": "string",  "required": false, "default": "" },
        "max_chars": { "type": "integer", "required": false, "default": 20000 }
      },
      "annotations": { "readOnlyHint": true, "destructiveHint": false, "idempotentHint": true, "openWorldHint": true },
      "does": "agent.Session.ReadText: the visible text of the page, or of the region the CSS selector names; transparent text is left out.",
      "result": "The text, cut at max_chars with '[+N chars truncated]'. isError for a selector that is not valid CSS, and for one that matches nothing or nothing visible.",
      "refuses": "A negative max_chars."
    },
    {
      "name": "run_hunt",
      "title": "Run several steps",
      "parameters": { "script": { "type": "string", "required": true } },
      "annotations": { "readOnlyHint": false, "destructiveHint": true, "idempotentHint": false, "openWorldHint": true },
      "does": "agent.Session.Run on a .hunt script: one step per line, STEP headings, control flow and @var: allowed.",
      "result": "RunOutcome.RenderForLLM: the verdict, the page now open, each step's outcome, and — when it failed — that the steps after the failed one did not run. isError when the run failed.",
      "refuses": "An empty script."
    },
    {
      "name": "suite_guide",
      "title": "How to build a test suite",
      "parameters": {},
      "annotations": { "readOnlyHint": true, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false },
      "does": "Returns pkg/mcp/suite_guide.md, compiled into the binary: how to build a test suite on Manul that lives in a repository and runs in CI — learning the app with map and step first, the layout (flat tests/, lib/ beside it), manul.config.json, lib/env.hunt defaults replaced by --var and MANUL_VAR_*, shared blocks, data-driven tests, running and a GitHub Actions job — with a whole example project against examples/quickstart/site. Opens no browser.",
      "result": "The guide as markdown. Its example project's files are fenced with their paths; the unit tests parse every one, and the e2e suite runs the project.",
      "refuses": "Any argument."
    }
  ],

  "prompts": [
    {
      "name": "build_test_suite",
      "title": "Build a test suite",
      "arguments": {
        "url":      { "required": true,  "description": "Address of the app to test." },
        "journeys": { "required": false, "description": "What to cover; empty means the main journeys found." }
      },
      "renders": "One user message: build a Manul test suite for <url> (covering <journeys>), explore with step and map before writing any step, copy labels from map, write the files and run each with manul run until it passes when files and commands are available, otherwise hand the files over; ask for accounts, secrets and the CI address. The suite guide follows in full.",
      "shownAs": "Claude Code: /mcp__manul-browser__build_test_suite. Claude Desktop: the connector's entry in the attach menu."
    }
  ],

  "annotationsRationale": "A step can submit a form, place an order or delete a record, so step and run_hunt are marked destructive and not idempotent; a client that asks before destructive tools asks before them. map, read and read_text only look. Every tool that drives the browser works on the open web (openWorldHint); suite_guide reads nothing but itself.",
  "argumentRule": "Arguments are decoded strictly: an unknown property is a tool error ('bad arguments: …'), so a misspelt parameter is told rather than ignored."
}
```
