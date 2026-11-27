# Manul Browser — CLI Contract

> **Machine-readable contract for the `manul` CLI interface.**
> Consumed by CI/CD integrations, editor tooling, and other downstream consumers.
>
> **Scope.** Subcommands, flags, env vars and exit codes of the `manul` binary.
> Implementation lives under `cmd/manul` + `pkg/...`.

```json
{
  "version": "0.1.6",
  "generatedFrom": "cmd/manul :: main(), _run_hunt_file(), parse_hunt_file(), sync_main(); pkg/config :: _KEY_MAP, global config constants; pkg/scan :: scan_main(); pkg/record :: record_main(); pkg/daemon :: daemon_main()",
  "entryPoints": {
    "console_script": "manul",
    "module": "manul",
    "devCli": "go run ./cmd/manul"
  },
  "subcommands": [
    {
      "id": "run",
      "syntax": "manul <path> [more paths]",
      "description": "Run .hunt files. Accepts .hunt files, directories of .hunt files, or '.' for CWD — as many as are given, each file once, in the order given and sorted within a directory. A directory contributes the .hunt files directly in it, not those of its subdirectories.",
      "positionalArgs": [
        {
          "name": "path",
          "required": true,
          "repeatable": true,
          "description": "Path to a .hunt file, a directory containing .hunt files, '.' for the current directory, or '-' to read one hunt from stdin ('-' cannot be combined with other paths). Every path counts: `manul run tests/*.hunt` runs every file the shell expands to. Only the first used to, and the others were dropped without a word."
        }
      ],
      "isDefault": true,
      "targetDetection": "A first argument that is '.', ends in .hunt, or names an existing directory is run as a target — unless it is a subcommand's name (run, run-step, read, map, schema, serve, mcp, scan, record, daemon, pages, controls, help), which always means the subcommand. A directory with such a name is run as `manul run pages/` or `manul ./pages`."
    },
    {
      "id": "scan",
      "syntax": "manul scan <URL> [--output <file>]",
      "description": "Scan a URL for interactive elements and generate a draft .hunt file.",
      "positionalArgs": [
        {
          "name": "url",
          "required": true,
          "description": "URL to scan for interactive elements."
        }
      ],
      "specificFlags": ["--output"]
    },
    {
      "id": "record",
      "syntax": "manul record <URL> [output_file]",
      "description": "Record browser interactions and generate a .hunt file.",
      "positionalArgs": [
        {
          "name": "url",
          "required": true,
          "description": "URL to open for recording."
        },
        {
          "name": "output_file",
          "required": false,
          "default": "tests/recorded_mission.hunt",
          "description": "Output file path for the recorded hunt."
        }
      ],
      "specificFlags": ["--output", "--browser"]
    },
    {
      "id": "daemon",
      "syntax": "manul daemon <directory>",
      "description": "Run scheduled .hunt files (those with @schedule: headers) as a long-running daemon. --var and MANUL_VAR_* set variables as for run. A flag overrides manul.config.json and MANUL_* only when it is passed: it used to copy every default across, so \"headless\": true and \"browser\": \"firefox\" from the config were undone.",
      "positionalArgs": [
        {
          "name": "directory",
          "required": true,
          "description": "Directory to scan for scheduled .hunt files."
        }
      ],
      "specificFlags": ["--headless", "--browser", "--screenshot"]
    },
    {
      "id": "test",
      "syntax": "go test ./...",
      "description": "Dev-only: runs the synthetic DOM test suite via go test. This workflow is not exposed through the installed `manul` binary.",
      "devOnly": true
    },
    {
      "id": "controls",
      "syntax": "manul controls list",
      "description": "List the custom controls registered in this process with RegisterCustomControl, as 'page → target' lines. The released binary registers none, so it prints 'No custom controls registered.'; nothing is scanned from disk. Controls supplied by a --hooks script or a language binding exist only for the run that registered them.",
      "positionalArgs": [
        {
          "name": "subcommand",
          "required": false,
          "default": "list",
          "description": "Currently only 'list' is supported. Any other value exits 1."
        }
      ]
    },
    {
      "id": "pages",
      "syntax": "manul pages [list|migrate]",
      "description": "Inspect and migrate the per-site page registry. `list` prints the registry directory (pages/ under the CWD); the mappings are in the JSON fragments themselves. `migrate <legacy-pages.json>` splits a legacy pre-0.0.9.30 pages.json into pages/<safe_netloc>.json fragments and renames the original to pages.json.bak.",
      "positionalArgs": [
        {
          "name": "subcommand",
          "required": false,
          "default": "list",
          "description": "'list' or 'migrate' (which takes the legacy file as a second argument). Required; anything else exits 1."
        }
      ]
    },
    {
      "id": "schema",
      "syntax": "manul schema [--json]",
      "description": "Agent command. Emits the DSL grammar + agent JSON shapes (verbs, hunt_rules, step_outcome, page_map, failure_reasons, agent_commands) as a compact JSON contract for an external LLM driver. No browser needed; payload on stdout, logs on stderr. --json is accepted for symmetry (output is always JSON).",
      "positionalArgs": []
    },
    {
      "id": "map",
      "syntax": "manul map [--cdp <url>] [--tab <url-substr>] [--max-per-group <n>] [--include-unlabeled]",
      "description": "Agent command. Attaches to an already-running Chrome over CDP (default http://127.0.0.1:9222) and emits a compact, landmark-grouped JSON map of the open page ({url, groups:[{name, elements:[{label, role, editable?}], truncated?}]}), deduped and per-group capped (default 8). Groups are ordered Page → content landmarks → chrome. A form control's label is the text of the <label> bound to it when it has one.",
      "positionalArgs": []
    },
    {
      "id": "read",
      "syntax": "manul read '<label>' [--cdp <url>] [--tab <url-substr>] [--selector '<css>'] [--max-chars <n>]",
      "description": "Agent command. Attaches over CDP and reads off the open page without a full scan. Targeted form resolves a human label and extracts its value → {value, found, reason}; a label that names a form control reads what the control currently holds, and found is true even when that is empty. With --selector, returns the sanitized visible text of that CSS region → {text, selector}, optionally truncated to --max-chars; a selector that matches nothing gives an empty text, and one that is not valid CSS is an error.",
      "positionalArgs": [
        {
          "name": "label",
          "required": false,
          "description": "Human-visible label to read; omit when using --selector."
        }
      ]
    },
    {
      "id": "run-step",
      "syntax": "manul run-step '<instruction>' [--cdp <url>] [--tab <url-substr>] [--compact]",
      "description": "Agent command. Attaches over CDP and runs one plain-English DSL instruction against the open page, emitting a compact step-outcome JSON {ok, step, action, value?, url?, page?, reason, error?, score?, near?}; page is what pages/ calls the page, when it names it. Exit code is non-zero when ok is false. 'near' lists top candidates on failure or a low-confidence (<0.35) match.",
      "positionalArgs": [
        {
          "name": "instruction",
          "required": true,
          "description": "One DSL line, e.g. \"Click the 'Login' button\"."
        }
      ]
    },
    {
      "id": "mcp",
      "syntax": "manul mcp [--browser <engine>] [--headless] [--cdp <url> | --attach] [--channel <name>] [--executable-path <path>] [--user-data-dir <dir>] [--locale <tag>] [--verbose]",
      "description": "Serves the engine to MCP clients (Claude Desktop, Claude Code, any Model Context Protocol client) over stdio: JSON-RPC 2.0 on stdin/stdout, logs on stderr. Six tools — step, map, read, read_text, run_hunt over the pkg/agent API, and suite_guide, how to build a test suite — and the build_test_suite prompt. The browser is opened by the first tool call that drives it (launched, or attached to with --cdp/--attach) and closed at end of input. The protocol, the tools and their annotations are MANUL_MCP_CONTRACT.md.",
      "positionalArgs": []
    }
  ],
  "flags": [
    {
      "id": "headless",
      "flag": "--headless",
      "type": "boolean",
      "default": false,
      "configKey": "headless",
      "envVar": "MANUL_HEADLESS",
      "description": "Run browser in headless mode. Applies to every browser a run launches, the one of each --workers worker included.",
      "appliesTo": ["run", "scan", "record", "daemon"]
    },
    {
      "id": "browser",
      "flag": "--browser",
      "type": "string",
      "default": "chromium",
      "configKey": "browser",
      "envVar": "MANUL_BROWSER",
      "allowedValues": ["chromium", "firefox", "electron"],
      "aliases": {"chromium": ["chrome", "chromium-browser", "msedge", "edge"], "firefox": ["mozilla", "gecko"]},
      "description": "Browser engine to launch. `chromium` is driven over CDP, `firefox` over WebDriver BiDi. An unsupported value exits non-zero before anything is launched.",
      "appliesTo": ["run", "scan", "record", "daemon"]
    },
    {
      "id": "locale",
      "flag": "--locale",
      "type": "string",
      "default": "en-US",
      "configKey": "locale",
      "envVar": "MANUL_LOCALE",
      "description": "Language the launched browser uses and asks pages for (navigator.language, Accept-Language), e.g. uk-UA. The same on both engines; workers get it too. The daemon takes it from the config key or MANUL_LOCALE, having no flag; scan, record and the agent API launch with the default.",
      "appliesTo": ["run"]
    },
    {
      "id": "workers",
      "flag": "--workers",
      "type": "integer",
      "default": 1,
      "minimum": 1,
      "configKey": "workers",
      "envVar": "MANUL_WORKERS",
      "description": "Max hunt files to run in parallel. Each worker is a goroutine with its own browser instance, launched with the run's --headless, --browser, --channel and --executable-path; --cdp, --target and --user-data-dir do not apply to workers and are reported as ignored. Passing --workers 1 overrides a configured value. Forced to 1 when --debug or --break-lines is active. An interrupt (Ctrl+C, SIGTERM) cancels the run in either mode: steps stop, hunts not yet started are counted as failed, and every launched browser is closed and its temp profile removed before the process exits; a second interrupt ends it at once.",
      "appliesTo": ["run"]
    },
    {
      "id": "tags",
      "flag": "--tags",
      "type": "string",
      "default": null,
      "description": "Comma-separated tag filter. Only run .hunt files whose @tags: header contains at least one matching tag.",
      "example": "--tags smoke,regression",
      "appliesTo": ["run"]
    },
    {
      "id": "var",
      "flag": "--var",
      "type": "string",
      "repeatable": true,
      "default": null,
      "envVar": "MANUL_VAR_<name>",
      "description": "Set a variable for every hunt of the run: --var site=https://staging.example.com is {site}. It is the value of its name from the first line of each hunt on, so a @var: of the same name — or one built from it, such as @var: {login} = {site}/login — does not replace it but falls back to it; an imported @var: is below it too, and a row of @data: is above it. The environment does the same: MANUL_VAR_password=… is {password}, the rest of the name taken as written, so a CI secret reaches a hunt without being written into one. A flag wins over the environment. A name may be written with its braces ({site}=…); a value without a name, or without '=', exits 1 before anything runs.",
      "example": "--var site=https://staging.example.com --var user=ada",
      "appliesTo": ["run", "daemon"]
    },
    {
      "id": "hooks",
      "flag": "--hooks",
      "type": "string",
      "default": null,
      "description": "Path to a hook script supplying custom controls, CALL HOST handlers and suite-level hooks. The engine spawns it and speaks the reverse-call half of the session protocol down its stdio; the script's stdout carries that protocol and nothing else, its stderr passes through. Interpreter is chosen by extension: .py runs under $MANUL_PYTHON or the first of python/python3 on PATH (python first on Windows, where python3 is usually the Store alias stub), .js/.mjs/.cjs under $MANUL_NODE or node, .exe and any executable file directly. Registration completes before the first hunt starts; the script is shut down by closing its stdin after the last after-all hook, with 10s grace before it is killed.",
      "example": "--hooks manul_hooks.py",
      "appliesTo": ["run"]
    },
    {
      "id": "debug",
      "flag": "--debug",
      "type": "boolean",
      "default": false,
      "description": "Interactive step-by-step mode with visual element highlighting. Pauses before every step. Forces --workers 1. Mutually exclusive with --break-lines in practice.",
      "appliesTo": ["run"]
    },
    {
      "id": "break_lines",
      "flag": "--break-lines",
      "type": "string",
      "default": null,
      "description": "Comma-separated list of .hunt file line numbers to pause at. Drives gutter-breakpoint style debugging from an editor. Forces --workers 1.",
      "example": "--break-lines 5,10,15",
      "appliesTo": ["run"]
    },
    {
      "id": "retries",
      "flag": "--retries",
      "type": "integer",
      "default": 0,
      "minimum": 0,
      "configKey": "retries",
      "envVar": "MANUL_RETRIES",
      "description": "Retry failed hunt files up to N times, sequentially or with --workers. Each attempt starts from a fresh runtime on the same page. A hunt that passes on a retry is marked 'flaky' (result fields flaky and attempts, run_history status 'flaky') and counts as passed; a data-driven hunt retries row by row.",
      "appliesTo": ["run"]
    },
    {
      "id": "screenshot",
      "flag": "--screenshot",
      "type": "string",
      "default": "on-fail",
      "configKey": "screenshot",
      "envVar": "MANUL_SCREENSHOT",
      "allowedValues": ["on-fail", "always", "none"],
      "description": "Screenshot capture mode for hunt runs. 'on-fail' saves the page as it stood after each failed step, 'always' after every step, 'none' nothing. Files are written as PNGs under screenshots/ in the working directory; each step result carries its file in screenshot_path, which the HTML report links. A block (IF, REPEAT, WHILE, FOR EACH) adds none of its own: the step that failed inside it already has one. Single agent steps (run-step) are never captured.",
      "appliesTo": ["run", "daemon"]
    },
    {
      "id": "html_report",
      "flag": "--html-report",
      "type": "boolean",
      "default": false,
      "configKey": "html_report",
      "envVar": "MANUL_HTML_REPORT",
      "description": "Write a self-contained HTML report for each hunt — for each row of a hunt with @data: — as reports/report_<title>_<YYYYMMDD_HHMMSS>_<seq>.html, and print its path. There is no aggregate report and no merging across invocations.",
      "appliesTo": ["run"]
    },
    {
      "id": "explain",
      "flag": "--explain",
      "type": "boolean",
      "default": false,
      "configKey": "explain_mode",
      "envVar": "MANUL_EXPLAIN",
      "description": "Print detailed heuristic score breakdown for each element resolution.",
      "appliesTo": ["run"]
    },
    {
      "id": "json",
      "flag": "--json",
      "type": "boolean",
      "default": false,
      "configKey": null,
      "envVar": null,
      "description": "Print each hunt's result (HuntResult: hunt_file, title, total_steps, passed, failed, results, …) as an indented JSON document on stdout — one document per hunt, and per row of a hunt with @data:, one after another rather than in an array. Human logs go to stderr.",
      "appliesTo": ["run"]
    },
    {
      "id": "jsonl",
      "flag": "--jsonl",
      "type": "boolean",
      "default": false,
      "configKey": null,
      "envVar": null,
      "description": "Stream JSON Lines to stdout: {\"event\":\"step\",\"data\":<step result>} per step, then {\"event\":\"result\",\"data\":<HuntResult>} per hunt. Human logs go to stderr. With --workers > 1 only the result lines are emitted, and a warning says so.",
      "appliesTo": ["run"]
    },
    {
      "id": "disable_cache",
      "flag": "--disable-cache",
      "type": "boolean",
      "default": false,
      "configKey": "disable_cache",
      "envVar": "MANUL_DISABLE_CACHE",
      "description": "Re-probe the page for every command instead of reusing the DOM snapshot taken earlier in the step. MANUL_SEMANTIC_CACHE_ENABLED=false does the same.",
      "appliesTo": ["run"]
    },
    {
      "id": "executable_path",
      "flag": "--executable-path",
      "type": "string",
      "default": null,
      "configKey": "executable_path",
      "envVar": "MANUL_EXECUTABLE_PATH",
      "description": "Absolute path to a custom browser or Electron app executable. Use with OPEN APP command in .hunt files for desktop automation.",
      "appliesTo": ["run"]
    },
    {
      "id": "cdp_endpoint",
      "flag": "--cdp",
      "type": "string",
      "default": null,
      "configKey": "cdp_endpoint",
      "envVar": "MANUL_CDP_ENDPOINT",
      "description": "Attach to a running browser at this endpoint instead of launching one. The scheme selects the protocol: http://127.0.0.1:9222 is CDP/Chromium, ws://127.0.0.1:9222/session is WebDriver BiDi/Firefox (a bare ws://host:port, as Firefox prints it, gets /session appended).",
      "appliesTo": ["run"]
    },
    {
      "id": "target",
      "flag": "--target",
      "type": "string",
      "default": null,
      "configKey": null,
      "envVar": null,
      "description": "With --cdp, select the page whose URL contains the given substring (form: url=<substr>; the url= prefix is optional). Falls back to the first page.",
      "appliesTo": ["run"]
    },
    {
      "id": "output",
      "flag": "--output",
      "type": "string",
      "default": null,
      "description": "Output file path for scan and record subcommands.",
      "appliesTo": ["scan", "record"]
    },
    {
      "id": "help",
      "flag": "--help",
      "aliases": ["-h"],
      "type": "boolean",
      "description": "Print usage information and exit.",
      "appliesTo": ["run", "scan", "record", "daemon"]
    }
  ],
  "configPriority": [
    "CLI flag (highest)",
    "Environment variable (MANUL_*)",
    "manul.config.json",
    "Built-in default (lowest)"
  ],
  "configFile": {
    "filename": "manul.config.json",
    "resolution": "CWD only (./manul.config.json). Unknown keys are ignored silently.",
    "keys": [
      { "key": "headless",               "envVar": "MANUL_HEADLESS",               "type": "boolean",       "default": "false" },
      { "key": "browser",                "envVar": "MANUL_BROWSER",                "type": "string",        "default": "chromium" },
      { "key": "browser_args",           "envVar": "MANUL_BROWSER_ARGS",           "type": "string[]",      "default": "[]" },
      { "key": "timeout",                "envVar": "MANUL_TIMEOUT",                "type": "integer",       "default": "5000" },
      { "key": "nav_timeout",            "envVar": "MANUL_NAV_TIMEOUT",            "type": "integer",       "default": "30000" },
      { "key": "disable_cache",          "envVar": "MANUL_DISABLE_CACHE",          "type": "boolean",       "default": "false" },
      { "key": "verbose",                "envVar": "MANUL_VERBOSE",                "type": "boolean",       "default": "false" },
      { "key": "debug_mode",             "envVar": "MANUL_DEBUG",                  "type": "boolean",       "default": "false" },
      { "key": "break_lines",            "envVar": null,                           "type": "integer[]",     "default": "[]" },
      { "key": "tags",                   "envVar": "MANUL_TAGS",                   "type": "string[]",      "default": "[]" },
      { "key": "verify_max_retries",     "envVar": "MANUL_VERIFY_MAX_RETRIES",     "type": "integer",       "default": "15" },
      { "key": "workers",                "envVar": "MANUL_WORKERS",                "type": "integer",       "default": "1" },
      { "key": "tests_home",             "envVar": "MANUL_TESTS_HOME",             "type": "string",        "default": "tests" },
      { "key": "auto_annotate",          "envVar": "MANUL_AUTO_ANNOTATE",          "type": "boolean",       "default": "false" },
      { "key": "channel",                "envVar": "MANUL_CHANNEL",                "type": "string | null", "default": "null" },
      { "key": "locale",                 "envVar": "MANUL_LOCALE",                 "type": "string",        "default": "en-US" },
      { "key": "executable_path",        "envVar": "MANUL_EXECUTABLE_PATH",        "type": "string | null", "default": "null" },
      { "key": "cdp_endpoint",           "envVar": "MANUL_CDP_ENDPOINT",           "type": "string | null", "default": "null" },
      { "key": "retries",                "envVar": "MANUL_RETRIES",                "type": "integer",       "default": "0" },
      { "key": "screenshot",             "envVar": "MANUL_SCREENSHOT",             "type": "string",        "default": "on-fail" },
      { "key": "html_report",            "envVar": "MANUL_HTML_REPORT",            "type": "boolean",       "default": "false" },
      { "key": "explain_mode",           "envVar": "MANUL_EXPLAIN",                "type": "boolean",       "default": "false" }
    ]
  },
  "exitCodes": {
    "0": "All hunts passed (includes flaky and warning statuses)",
    "1": "One or more hunts failed or are broken"
  },
  "outputArtifacts": [
    {
      "id": "run_history",
      "path": "reports/run_history.json",
      "format": "JSON Lines",
      "description": "Append-only history of all mission results. Each line is a JSON object with file, name, timestamp, status, duration_ms.",
      "alwaysGenerated": true
    },
    {
      "id": "html_report",
      "path": "reports/report_<title>_<YYYYMMDD_HHMMSS>_<seq>.html",
      "description": "Self-contained HTML report, one per hunt (one per row for a hunt with @data:). <seq> is a per-process counter, so parallel workers never overwrite each other.",
      "alwaysGenerated": false,
      "trigger": "--html-report flag or html_report config key"
    },
    {
      "id": "screenshots",
      "path": "screenshots/*.png",
      "description": "Step screenshots per the --screenshot mode (fail_<ms>_<n>.png), plus the files SCREENSHOT \"name\" asks for.",
      "alwaysGenerated": false
    }
  ],
  "notGenerated": "There is no reports/last_run.log, no aggregate reports/manul_report.html and no reports/manul_report_state.json; the engine writes neither a run log nor report-session state.",
  "missionStatuses": [
    {
      "id": "pass",
      "label": "PASS",
      "icon": "✅",
      "description": "All steps succeeded on the first attempt.",
      "exitCode": 0
    },
    {
      "id": "fail",
      "label": "FAIL",
      "icon": "❌",
      "description": "One or more steps failed and all retries (if any) also failed.",
      "exitCode": 1
    },
    {
      "id": "broken",
      "label": "BROKEN",
      "icon": "💥",
      "description": "The [SETUP] hook failed before any browser step ran, or @before_all / @before_group failed.",
      "exitCode": 1
    },
    {
      "id": "flaky",
      "label": "FLAKY",
      "icon": "⚠️",
      "description": "Failed on first attempt but passed on a retry. Counted as passed for exit code purposes.",
      "exitCode": 0
    },
    {
      "id": "warning",
      "label": "WARNING",
      "icon": "⚠️",
      "description": "All steps passed but one or more VERIFY SOFTLY assertions failed. Counted as passed for exit code.",
      "exitCode": 0
    }
  ],
  "parallelModel": {
    "mechanism": "One process. pkg/worker runs hunts on a pool of goroutines, each Worker launching and owning its own browser.",
    "concurrency": "At most `workers` hunts — and browsers — at once.",
    "stdout": "Results are printed after the pool finishes, in submission order, each under a '📜 <file> (worker N)' header on stderr.",
    "htmlReport": "One report per hunt, exactly as in a sequential run.",
    "runHistory": "Appended by the parent for every hunt.",
    "globalVars": "Each worker's runtime is seeded with the suite's GlobalContext variables directly.",
    "restrictions": [
      "--debug and --break-lines force --workers 1 (sequential)",
      "--jsonl emits only the per-hunt result lines, not per-step lines",
      "Suite hooks still run once per suite: before_all/after_all once, group hooks once per hunt. Callbacks to a --hooks script are serialised over its one pipe."
    ]
  },
  "parsedHuntFile": {
    "type": "ParsedHunt (NamedTuple, 12 fields)",
    "fields": [
      { "name": "mission",         "type": "str",                     "description": "Concatenated mission body (non-header, non-hook, non-comment lines). USE directives are expanded inline." },
      { "name": "context",         "type": "str",                     "description": "@context: value" },
      { "name": "title",           "type": "str",                     "description": "@title: value (or @blueprint:)" },
      { "name": "step_file_lines", "type": "list[int]",               "description": "1-based file line numbers for each mission line, for breakpoint mapping. Expanded USE actions get synthetic line 0." },
      { "name": "setup_lines",     "type": "list[str]",               "description": "Extracted [SETUP] block instruction strings" },
      { "name": "teardown_lines",  "type": "list[str]",               "description": "Extracted [TEARDOWN] block instruction strings" },
      { "name": "parsed_vars",     "type": "dict[str, str]",          "description": "Key/value pairs from @var: headers (keys stored without braces)" },
      { "name": "tags",            "type": "list[str]",               "description": "Tags from @tags: header (empty list if absent)" },
      { "name": "data_file",       "type": "str",                     "description": "@data: file path (empty string if absent)" },
      { "name": "schedule",        "type": "str",                     "description": "@schedule: expression (empty string if absent)" },
      { "name": "exports",         "type": "list[str]",               "description": "Block names from @export: headers (empty list if absent). ['*'] for wildcard export." },
      { "name": "imports",         "type": "list[ImportDirective]",   "description": "Parsed @import: directives (empty list if absent). Each has block_names, source, aliases." }
    ],
    "scriptAliasRewriting": "@script: aliases are resolved and rewritten to real dotted paths in mission lines and hook lines before returning"
  },
  "debugProtocol": {
    "description": "Stdin/stdout protocol between the CLI runner and a non-TTY driver for gutter breakpoint debugging.",
    "pauseMarker": "\\x00MANUL_DEBUG_PAUSE\\x00{\"step\":\"...\",\"idx\":N}\\n",
    "stdinCommands": [
      { "command": "next\\n",       "effect": "Advance one step then pause again" },
      { "command": "continue\\n",   "effect": "Run until the next gutter breakpoint or end" },
      { "command": "debug-stop\\n", "effect": "Clear all breakpoints; run to completion without pausing" },
      { "command": "abort\\n",      "effect": "Force-kill the process (extension sends this + kills after 500ms)" },
      { "command": "explain\\n",    "effect": "Trigger DOMScorer explain output for the current step" }
    ],
    "ttyDetection": "_Tee.isatty() delegates to the underlying terminal so --debug TTY prompts work correctly even when stdout is tee'd to a log file"
  }
}
```
