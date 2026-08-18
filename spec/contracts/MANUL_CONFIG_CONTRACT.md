# Manul Browser — Configuration Contract

> **Machine-readable contract for the Manul Browser configuration surface.**
> Consumed by CI/CD integrations, editor tooling, and downstream consumers.
>
> **Scope.** Config keys, `MANUL_*` env vars, defaults and precedence, as
> implemented in `pkg/config`.

```json
{
  "version": "0.1.1",
  "generatedFrom": "pkg/config :: _KEY_MAP, _CFG, get_threshold(), lookup_page_name(); pkg/runtime :: ScopedVariables; pkg/config :: envBool()",

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
    "function": "env_bool(name, default)",
    "truthy": ["true", "1", "yes"],
    "falsy": "everything else",
    "caseInsensitive": true,
    "stripsWhitespace": true
  },

  "keys": [
    {
      "key": "headless",
      "envVar": "MANUL_HEADLESS",
      "type": "boolean",
      "default": false,
      "description": "Run browser in headless mode (no visible window).",
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
      "description": "Extra launch flags passed to the browser. JSON: native array. Env: comma-or-space-separated string.",
      "examples": [["--disable-gpu"], ["--window-size=1920,1080"]],
      "specialHandling": "List cannot round-trip via plain env string; env is split on comma/space."
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
      "description": "Default action timeout for click, fill, select, hover operations.",
      "minimum": 0
    },
    {
      "key": "nav_timeout",
      "envVar": "MANUL_NAV_TIMEOUT",
      "type": "integer",
      "unit": "milliseconds",
      "default": 30000,
      "description": "Navigation timeout for NAVIGATE, page loads, and WAIT FOR RESPONSE.",
      "minimum": 0
    },
    {
      "key": "semantic_cache_enabled",
      "envVar": "MANUL_SEMANTIC_CACHE_ENABLED",
      "type": "boolean",
      "default": true,
      "description": "Enable in-session semantic cache (learned_elements). Feeds the scorer as one channel (never bypasses scoring); provides a +200,000 scaled score boost within a single run. Resets when the engine instance is destroyed.",
      "uiLabel": "Semantic Cache"
    },
    {
      "key": "custom_controls_dirs",
      "envVar": "MANUL_CUSTOM_CONTROLS_DIRS",
      "type": "string[]",
      "default": ["controls"],
      "description": "Directories scanned for @custom_control Go handlers. Resolved relative to CWD. Env: comma-separated.",
      "legacyAlias": {
        "key": "custom_modules_dirs",
        "envVar": "MANUL_CUSTOM_MODULES_DIRS"
      }
    },
    {
      "key": "log_name_maxlen",
      "envVar": "MANUL_LOG_NAME_MAXLEN",
      "type": "integer",
      "default": 0,
      "description": "If > 0, truncates element names in console log output to this many characters.",
      "minimum": 0
    },
    {
      "key": "log_thought_maxlen",
      "envVar": "MANUL_LOG_THOUGHT_MAXLEN",
      "type": "integer",
      "default": 0,
      "description": "If > 0, truncates verbose 'thought'/diagnostic strings in console log output.",
      "minimum": 0
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
      "envVar": null,
      "type": "string",
      "default": "tests",
      "description": "Default directory for new hunt files, SCAN PAGE output, and manul scan default output. JSON-only — no env var override.",
      "note": "No MANUL_* env var for this key."
    },
    {
      "key": "auto_annotate",
      "envVar": "MANUL_AUTO_ANNOTATE",
      "type": "boolean",
      "default": false,
      "description": "Automatically inject '# 📍 Auto-Nav: <name>' comments into .hunt files whenever the page URL changes during a run. Page names from pages.json.",
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
      "description": "Screenshot capture mode. Screenshots stored as base64 PNGs in StepResult.screenshot and the HTML report.",
      "cliFlag": "--screenshot"
    },
    {
      "key": "html_report",
      "envVar": "MANUL_HTML_REPORT",
      "type": "boolean",
      "default": false,
      "description": "Generate self-contained HTML report after the run (reports/manul_report.html). Recent invocations merged via report session state.",
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
    "format": "One JSON fragment per site (file name = <safe_netloc>.json)",
    "resolution": [
      "CWD/pages/*.json — writable, directory auto-created if absent",
      "Override directory via MANUL_PAGES_DIR env var (absolute or CWD-relative)"
    ],
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
      "Within site block: exact URL → regex/substring patterns (skipping 'Domain' key) → 'Domain' fallback",
      "Auto-populates new URLs by writing a per-site fragment pages/<safe_netloc>.json with placeholder 'Auto: domain/path'"
    ],
    "introspection": {
      "cli": "manul pages list",
      "description": "Print every site → pattern → label mapping discovered under pages/."
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
        "description": "CLI/env context and @before_all lifecycle hook variables. Shared across all hunt files via MANUL_GLOBAL_VARS.",
        "populatedBy": "@before_all hooks, MANUL_GLOBAL_VARS env var"
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
        "var": "MANUL_GLOBAL_VARS",
        "type": "JSON string",
        "description": "Serialised GlobalContext.variables dict for passing @before_all results to parallel worker subprocesses."
      },
      {
        "var": "MANUL_WORKER_TIMEOUT",
        "type": "integer (seconds)",
        "default": 600,
        "description": "Subprocess worker timeout. Workers killed after this duration."
      },
      {
        "var": "MANUL_REPORT_SESSION_TTL_SEC",
        "type": "integer (seconds)",
        "default": 1800,
        "description": "TTL for HTML report session state merging. Older sessions start fresh."
      }
    ]
  }
}
```
