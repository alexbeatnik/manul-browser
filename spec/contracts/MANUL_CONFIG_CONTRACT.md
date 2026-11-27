# Manul Browser — Configuration Contract

> **Machine-readable contract for the Manul Browser configuration surface.**
> Consumed by CI/CD integrations, editor tooling, and downstream consumers.
>
> **Scope.** Config keys, `MANUL_*` env vars, defaults and precedence, as
> implemented in `pkg/config`.

```json
{
  "version": "0.1.6",
  "generatedFrom": "pkg/config :: Config, jsonConfig, Default(), Load(), applyJSONFile(), overrideFromEnv(); pkg/pages :: Registry.LookupPageName(); pkg/runtime :: ScopedVariables",

  "configFile": {
    "filename": "manul.config.json",
    "format": "JSON",
    "resolution": [
      "CWD (./manul.config.json) — the only lookup location in the Go runtime"
    ]
  },

  "priority": [
    "Environment variable MANUL_* (highest)",
    "JSON config file key",
    "Built-in default (lowest)"
  ],

  "booleanParsing": {
    "function": "strconv.ParseBool, inside overrideFromEnv()",
    "truthy": ["1", "t", "T", "TRUE", "true", "True"],
    "falsy": ["0", "f", "F", "FALSE", "false", "False"],
    "invalid": "Any other value — 'yes', 'on', ' true' with a space — is ignored: the setting keeps its JSON or default value, and nothing is logged.",
    "caseInsensitive": false,
    "stripsWhitespace": false
  },

  "unknownKeys": "manul.config.json keys the engine does not know are ignored silently, so a misspelled key has no effect and raises no error.",

  "keys": [
    {
      "key": "headless",
      "envVar": "MANUL_HEADLESS",
      "type": "boolean",
      "default": false,
      "description": "Run browser in headless mode (no visible window). The headless window is 1366×768 in both Chromium and Firefox, so a responsive site lays itself out the same way in either.",
      "cliFlag": "--headless"
    },
    {
      "key": "browser_mode",
      "envVar": "MANUL_BROWSER_MODE",
      "type": "string",
      "default": "",
      "allowedValues": ["launch", "attach"],
      "description": "Whether the engine starts its own Chrome (`launch`) or drives one that is already running (`attach`). This is the single explicit answer to that question. Empty means infer — see resolveBrowserMode. In `attach` mode the browser is NOT closed when the session ends, because the engine did not open it; launch-only keys (channel, executable_path, browser_args, headless) are ignored.",
      "cliFlag": "--attach | --launch",
      "since": "0.2.0"
    },
    {
      "key": "browser",
      "envVar": "MANUL_BROWSER",
      "type": "string",
      "default": "chromium",
      "allowedValues": ["chromium", "firefox", "electron"],
      "aliases": {"chromium": ["chrome", "chromium-browser", "msedge", "edge"], "firefox": ["mozilla", "gecko"]},
      "description": "Which browser engine to launch. `chromium` (the default) is driven over CDP; `firefox` is driven over WebDriver BiDi, because Firefox deprecated CDP in 129 and removed it in 141. WebKit/Safari are not supported. Use `channel`/`executable_path` to pick a specific binary. An unrecognised value is an error, not a silent fall back to Chromium.",
      "cliFlag": "--browser",
      "deprecatedValue": {
        "value": "electron",
        "reason": "`electron` was a second, implicit way of saying `browser_mode: attach`. It still works and still selects attach, but the engine logs a deprecation warning. Use browser_mode instead.",
        "supersededBy": "browser_mode"
      }
    },
    {
      "key": "browser_args",
      "envVar": "MANUL_BROWSER_ARGS",
      "type": "string[]",
      "default": [],
      "description": "Extra launch flags passed to the browser, after the engine's own, so a later flag of the same name wins: every browser `manul run`, its --workers, `manul serve`, `manul mcp` and the Go API launch gets them. They were read and passed to no browser. Each engine has its own flags: headless Firefox ignores --window-size and takes -width and -height. JSON: native array. Env: flags separated by spaces or commas, where a comma separates two flags only if the next starts with '-', so --window-size=1920,1080 stays one flag (it used to be split at the comma).",
      "examples": [["--disable-gpu"], ["--window-size=1920,1080"], ["-width", "1920", "-height", "1080"]],
      "specialHandling": "Env: split on whitespace, and on a comma followed by '-'. A flag whose value has a space in it needs the JSON array."
    },
    {
      "key": "channel",
      "envVar": "MANUL_CHANNEL",
      "type": "string | null",
      "default": null,
      "description": "Browser channel — selects an installed binary of the engine named by `browser` (chrome, msedge, chromium, firefox-esr, firefox-nightly, ...). An unknown value is treated as a bare binary name to look up on PATH.",
      "examples": [null, "chrome", "chrome-beta", "msedge", "firefox-esr", "firefox-dev", "firefox-nightly"],
      "validation": "Must be a channel identifier of the selected engine (chrome/chrome-beta/chrome-dev/chromium/msedge, or firefox/firefox-esr/firefox-dev/firefox-beta/firefox-nightly), a binary name on PATH, or null."
    },
    {
      "key": "locale",
      "envVar": "MANUL_LOCALE",
      "type": "string",
      "default": "en-US",
      "description": "Language a launched browser uses and asks pages for, as a BCP 47 tag. It sets navigator.language and the Accept-Language header, as the tag followed by its language alone (\"uk-UA\" asks for \"uk-UA,uk\"): --lang and --accept-lang for Chromium, intl.accept_languages and intl.locale.requested for Firefox. Empty is en-US. Left to themselves the two engines differ — headless Chrome asked for en-US, Firefox for the system's language — and the same hunt met a site in two languages. A browser that is attached to (--cdp) keeps its own.",
      "examples": ["en-US", "uk-UA", "de-DE"]
    },
    {
      "key": "executable_path",
      "envVar": "MANUL_EXECUTABLE_PATH",
      "type": "string | null",
      "default": null,
      "description": "Absolute path to a custom browser or Electron app executable. Used with OPEN APP command for desktop automation.",
      "cliFlag": "--executable-path"
    },
    {
      "key": "cdp_endpoint",
      "envVar": "MANUL_CDP_ENDPOINT",
      "type": "string | null",
      "default": null,
      "description": "The endpoint to dial when attaching. Its scheme selects the protocol: an HTTP endpoint (e.g. http://127.0.0.1:9222) is CDP/Chromium, a WebSocket URL (e.g. ws://127.0.0.1:9222/session) is WebDriver BiDi/Firefox — a bare ws://host:port, as Firefox prints it at startup, gets /session appended. Defaults to http://127.0.0.1:9222 in attach mode. Setting it also *implies* attach when browser_mode is empty — inference kept for existing configs; prefer setting browser_mode explicitly.",
      "cliFlag": "--cdp"
    },
    {
      "key": "timeout",
      "envVar": "MANUL_TIMEOUT",
      "type": "integer",
      "unit": "milliseconds",
      "default": 5000,
      "description": "How long a step waits for what it is waiting for. An action whose target is not on the page yet (CLICK, DOUBLE CLICK, RIGHT CLICK, FILL, TYPE, SELECT, CHECK, UNCHECK, HOVER, DRAG, UPLOAD, EXTRACT, PRESS … ON, SCROLL … inside) looks for it again every 250ms for this long before failing not_found; VERIFY polls for this long. 0 turns the waiting for a target off.",
      "minimum": 0
    },
    {
      "key": "nav_timeout",
      "envVar": "MANUL_NAV_TIMEOUT",
      "type": "integer",
      "unit": "milliseconds",
      "default": 30000,
      "description": "Navigation timeout for NAVIGATE, page loads, and WAIT FOR RESPONSE. NAVIGATE that runs out of it fails, unless the document has already been parsed, in which case the hunt continues with a warning. The wait for a page a CLICK started loading is cut off at it without failing the step. 0 removes the limit for NAVIGATE and page loads, and leaves WAIT FOR RESPONSE with the step timeout.",
      "minimum": 0
    },
    {
      "key": "disable_cache",
      "envVar": "MANUL_DISABLE_CACHE",
      "type": "boolean",
      "default": false,
      "description": "Turn off the DOM snapshot cache, so every command probes the page afresh instead of reusing the snapshot taken earlier in the step. MANUL_SEMANTIC_CACHE_ENABLED is accepted as its inverse (false disables the cache) and wins when both are set; there is no semantic_cache_enabled JSON key.",
      "cliFlag": "--disable-cache"
    },
    {
      "key": "verbose",
      "envVar": "MANUL_VERBOSE",
      "type": "boolean",
      "default": false,
      "description": "Verbose engine logging.",
      "cliFlag": "--verbose"
    },
    {
      "key": "debug_mode",
      "envVar": "MANUL_DEBUG",
      "type": "boolean",
      "default": false,
      "description": "Pause before every step, as --debug does.",
      "cliFlag": "--debug"
    },
    {
      "key": "break_lines",
      "envVar": null,
      "type": "integer[]",
      "default": [],
      "description": ".hunt line numbers to pause at, as --break-lines does. JSON only.",
      "cliFlag": "--break-lines"
    },
    {
      "key": "tags",
      "envVar": "MANUL_TAGS",
      "type": "string[]",
      "default": [],
      "description": "Run only hunt files whose @tags: header shares at least one tag, as --tags does. Env: comma-separated.",
      "cliFlag": "--tags"
    },
    {
      "key": "workers",
      "envVar": "MANUL_WORKERS",
      "type": "integer",
      "default": 1,
      "minimum": 1,
      "description": "Max hunt files to run in parallel. Each worker spawns a separate subprocess with its own browser. Forced to 1 when --debug or --break-lines is active.",
      "cliFlag": "--workers"
    },
    {
      "key": "tests_home",
      "envVar": "MANUL_TESTS_HOME",
      "type": "string",
      "default": "tests",
      "description": "Loaded and overridable, but no command reads it yet: SCAN PAGE and `manul scan` write where they are told, and nothing else consults it."
    },
    {
      "key": "auto_annotate",
      "envVar": "MANUL_AUTO_ANNOTATE",
      "type": "boolean",
      "default": false,
      "description": "After each NAVIGATE, log '📍 Auto-Nav: <name>' with the page name the pages/ registry gives the URL (the URL itself when it has none). Only NAVIGATE is annotated, not a page a click led to. Log output only — no .hunt file is modified.",
      "uiLabel": "Auto-Annotate Page Navigation"
    },
    {
      "key": "retries",
      "envVar": "MANUL_RETRIES",
      "type": "integer",
      "default": 0,
      "minimum": 0,
      "description": "Number of times to retry a failed hunt file. Pass on retry marks status as 'flaky'.",
      "cliFlag": "--retries"
    },
    {
      "key": "screenshot",
      "envVar": "MANUL_SCREENSHOT",
      "type": "string",
      "default": "on-fail",
      "allowedValues": ["on-fail", "always", "none"],
      "description": "Screenshot capture mode for hunt runs: PNG files under screenshots/ in the working directory, recorded on each step result as screenshot_path and linked from the HTML report.",
      "cliFlag": "--screenshot"
    },
    {
      "key": "html_report",
      "envVar": "MANUL_HTML_REPORT",
      "type": "boolean",
      "default": false,
      "description": "Write a self-contained HTML report per hunt — per row for a hunt with @data: — as reports/report_<title>_<YYYYMMDD_HHMMSS>_<seq>.html. There is no aggregate report and no merging across invocations.",
      "cliFlag": "--html-report"
    },
    {
      "key": "verify_max_retries",
      "envVar": "MANUL_VERIFY_MAX_RETRIES",
      "type": "integer",
      "default": 15,
      "minimum": 1,
      "description": "Maximum polling retries for VERIFY steps before declaring failure. Each retry waits ~1.0s for checked/enabled/disabled state verification and ~1.5s for text presence verification."
    },
    {
      "key": "explain_mode",
      "envVar": "MANUL_EXPLAIN",
      "type": "boolean",
      "default": false,
      "description": "Print detailed per-channel heuristic score breakdown for each element resolution.",
      "cliFlag": "--explain"
    }
  ],

  "resolveBrowserMode": {
    "function": "config.Config.ResolveBrowserMode() string",
    "returns": ["launch", "attach"],
    "description": "Settles the one question every session must answer: start a new Chrome, or drive one that is already running. Historically that bit was spread across two keys that could disagree — cdp_endpoint (set ⇒ attach) and browser (electron ⇒ attach) — so `browser: chromium` plus a cdp_endpoint was ambiguous. browser_mode is the explicit answer; the rest is inference kept so existing configs keep working.",
    "precedence": [
      "1. browser_mode, when it names a known mode (case-insensitive).",
      "2. browser == 'electron' — the deprecated spelling of attach.",
      "3. cdp_endpoint being set at all.",
      "4. launch."
    ],
    "notes": [
      "An unrecognised browser_mode value does not error here; it falls through to inference. The serve protocol rejects an unknown `open` mode explicitly, so a caller cannot silently get a browser they did not ask for.",
      "BrowserModeIsDeprecatedSpelling() reports when attach was chosen only by the electron alias, so the engine can warn once instead of honouring it silently.",
      "AttachEndpoint() supplies http://127.0.0.1:9222 when attaching with no endpoint configured."
    ],
    "lifetime": "A launched browser is closed with the session. An attached one is not — the engine did not open it."
  },

  "pagesRegistry": {
    "directory": "<project>/pages/",
    "format": "One JSON fragment per site (file name = <safe_netloc>.json), and page files: pages/*.hunt with @page: (MANUL_DSL_CONTRACT.md metadata page), whose @url: joins the registry as a path for any site or a whole address for its own, read before the JSON fragments so a fragment that names the same address differently wins. Both are read again only when a file is added, removed or changed.",
    "resolution": [
      "CWD/pages/*.json — writable, directory auto-created if absent. There is no override; the directory is always pages/ under the working directory."
    ],
    "uses": "Every step's result (page_name), the agent's and MCP's answers and map, VERIFY that the page is '<name>', IF the page is '<name>', NAVIGATE to '<name>', auto_annotate log lines and `manul pages`. Only auto_annotate writes to it; the rest read it, and an 'Auto:' placeholder is not a name to them. The fragments are read again only when one is added, removed or changed. It is NOT how custom controls find their page: a control's page label is document.title, or a label derived from the URL path when the title is empty (see the hooks contract).",
    "fragmentShapes": {
      "lean": {
        "description": "Preferred. site is the explicit site root; remaining keys are pattern→name mappings.",
        "example": {
          "site": "https://example.com/",
          "Domain": "Example Site",
          ".*/login": "Login Page",
          "https://example.com/dashboard": "Dashboard"
        }
      },
      "anySite": {
        "description": "A lean fragment whose site is \"*\": its keys hold on every http(s) site that no fragment of its own covers, so a suite run against localhost and staging names its pages once, by path. NAVIGATE to one of its names goes to the path on the site open now, or from a blank page on the hunt's {site}. An address such a fragment covers is never auto-populated.",
        "example": { "site": "*", "/login.html": "Login", "/shop.html": "Shop" }
      },
      "wrapped": {
        "description": "Back-compat shape mirroring the pre-0.0.9.30 nested form. The single top-level key is the site root.",
        "example": {
          "https://example.com/": {
            "Domain": "Example Site",
            ".*/login": "Login Page"
          }
        }
      }
    },
    "lookupFunction": "lookup_page_name(url: str) -> str",
    "lookupBehavior": [
      "Re-merges every pages/*.json fragment from disk on every call (live edits picked up instantly)",
      "Finds best-matching site block (longest-prefix wins across all fragments)",
      "Within site block, the most specific of: the address itself; the address without its query and fragment; every other key that matches — a plain path key ('/login': starts with '/', no regular-expression characters, a dot is a dot) matches the path from its start, a whole segment at a time, and any other key matches as a regular expression, or failing that as a substring, of the address — the longest such key winning, and of keys as long the first in byte order; then the 'Domain' fallback. Two keys that matched one address used to name it at random, and a path key matched anywhere in the address ('/admin' named /blog/admin-tips).",
      "Auto-populates an address on a site with no fragment by writing pages/<safe_netloc>.json: 'Domain' is 'Auto: <host>' and the address, without its query and fragment, is 'Auto: <host>/<path>'. It used to name the whole site after the first address it saw, query and all. The fragment is written to a temporary file and renamed into place, so a parallel worker never reads one half written. Lookups, and so writes, happen only for auto_annotate and `manul pages`."
    ],
    "introspection": {
      "cli": "manul pages list",
      "description": "Print the registry directory. The mappings themselves are not listed; read pages/<site>.json."
    },
    "migration": {
      "cli": "manul pages migrate",
      "description": "One-shot migration of a legacy pages.json (pre-0.0.9.30 monolithic file) into per-site fragments under pages/. Renames the original to pages.json.bak. The legacy flat file is no longer read by the engine."
    },
    "breakingChange": "The monolithic pages.json file is no longer read or written. Run `manul pages migrate` once to convert."
  },

  "scopedVariables": {
    "class": "ScopedVariables",
    "module": "pkg/runtime",
    "levels": [
      {
        "id": "LEVEL_ROW",
        "priority": 1,
        "label": "row",
        "description": "Per-iteration variables from @data CSV/JSON rows. Cleared between data-driven iterations.",
        "populatedBy": "@data: file rows"
      },
      {
        "id": "LEVEL_STEP",
        "priority": 2,
        "label": "step",
        "description": "Runtime variables from EXTRACT, SET, and CALL GO ... into {var}.",
        "populatedBy": "EXTRACT, SET, CALL GO ... into {var}"
      },
      {
        "id": "LEVEL_MISSION",
        "priority": 3,
        "label": "mission",
        "description": "File-level @var: declarations and [SETUP] hook return values.",
        "populatedBy": "@var: headers, CALL GO ... into {var} in [SETUP]"
      },
      {
        "id": "LEVEL_GLOBAL",
        "priority": 4,
        "label": "global",
        "description": "Variables published by suite hooks. Seeded into every hunt's runtime directly, worker-pool goroutines included; nothing travels through the environment.",
        "populatedBy": "before_all / before_group hooks (pkg/lifecycle)"
      }
    ],
    "resolution": "Highest-priority level first (row → step → mission → global). First non-null match wins.",
    "substitution": "{placeholder} syntax in step text. resolve() + substitute() methods.",
    "dictCompat": true,
    "methods": [
      "resolve(name) -> str | None",
      "resolve_level(name) -> tuple[str | None, str | None]",
      "as_flat_dict() -> dict[str, str]",
      "substitute(text) -> str",
      "set(name, value, level)",
      "set_many(mapping, level)",
      "clear_level(level)",
      "clear_runtime()",
      "clear_all()",
      "dump() -> str"
    ]
  },

  "environmentVariables": {
    "description": "All MANUL_* env vars override the corresponding JSON config key. Additional runtime-only env vars:",
    "runtimeOnly": [
      {
        "var": "MANUL_PYTHON",
        "type": "string",
        "description": "Interpreter for a .py --hooks script. Defaults to the first of python/python3 on PATH, which need not be the interpreter that has `manul` installed — set this to the virtualenv's python."
      },
      {
        "var": "MANUL_NODE",
        "type": "string",
        "description": "Interpreter for a .js/.mjs/.cjs --hooks script. Defaults to node on PATH."
      }
    ],
    "notRead": "MANUL_GLOBAL_VARS, MANUL_WORKER_TIMEOUT, MANUL_REPORT_SESSION_TTL_SEC and MANUL_PAGES_DIR belonged to the Python engine and have no effect on this one."
  }
}
```
