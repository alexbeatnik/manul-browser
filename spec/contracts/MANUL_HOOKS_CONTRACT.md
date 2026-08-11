# ManulEngine (Go) — Hooks & Lifecycle Contract

> **Machine-readable contract for the hook system, Go extension registration, and variable scoping.**
> Consumed by test framework integrations, CI/CD runners, and downstream tooling that extends ManulEngine (Go)'s execution lifecycle.
>
> **Host surface.** The `.hunt`-level surface is `[SETUP]`/`[TEARDOWN]` blocks
> plus five-level variable scoping. The inline call verb is `CALL HOST` (also
> spelled `CALL GO` / `CALL PYTHON`), resolving a handler the host registered —
> `RegisterGoCall` when the engine is embedded, or a client-side function
> reached by reverse call when it is driven through a binding. It no longer
> imports a module by dotted path.
>
> **Suite-level hooks** (`before_all` / `after_all` / `before_group` /
> `after_group`) live in `pkg/lifecycle`. They are registered at process init in
> Go, or declared over the session protocol by a binding. `MANUL_GLOBAL_VARS`
> has no equivalent and none is needed: the suite's global variables are held in
> a `GlobalContext` and seeded into each hunt's runtime directly, including
> across worker-pool goroutines.

```json
{
  "version": "0.1.0",
  "generatedFrom": "pkg/runtime/extensions.go :: RegisterGoCall(), RegisterCustomControl(), GetGoCall(), GetCustomControl(), ResetRuntimeRegistries(), GoCallHandler, GoCallInvocation, CustomControlHandler, CustomControlInvocation; pkg/runtime :: [SETUP]/[TEARDOWN] execution; pkg/runtime/variables.go :: ScopedVariables, Level",

  "fileHooks": {
    "description": "Per-hunt-file [SETUP] and [TEARDOWN] blocks. Run CALL GO and PRINT instructions outside the browser for data injection and cleanup.",

    "blocks": {
      "SETUP": {
        "markers": ["[SETUP]", "[END SETUP]"],
        "timing": "Runs BEFORE the browser launches.",
        "failureBehavior": "Mission status set to 'broken'; browser steps are skipped entirely.",
        "allowedInstructions": ["CALL GO", "PRINT"]
      },
      "TEARDOWN": {
        "markers": ["[TEARDOWN]", "[END TEARDOWN]"],
        "timing": "Runs AFTER the mission (pass or fail). Only executes if [SETUP] succeeded.",
        "failureBehavior": "Logged but does not override the mission result.",
        "allowedInstructions": ["CALL GO", "PRINT"]
      }
    },

    "hookResult": {
      "description": "Outcome of executing one hook line. Language-neutral shape shared with ManulEngine.",
      "fields": [
        { "name": "success",        "type": "bool",                "description": "True if the line executed without error." },
        { "name": "message",        "type": "string",  "default": "",   "description": "Human-readable status message." },
        { "name": "returnValue",    "type": "string?", "default": null, "description": "Stringified handler return value when 'into {var}' was requested." },
        { "name": "varName",        "type": "string?", "default": null, "description": "Variable name from the 'into {var}' clause." },
        { "name": "returnMapping",  "type": "map[string]string", "default": {}, "description": "Key-value pairs when the handler returns a map/struct." }
      ]
    },

    "callGoSyntax": {
      "variants": [
        "CALL GO <package>.<function>",
        "CALL GO {alias}.<function>",
        "CALL GO {callable_alias}",
        "CALL GO <package>.<function> \"arg1\" 'arg2' {var}",
        "CALL GO <package>.<function> into {result}",
        "CALL GO <package>.<function> \"arg1\" {var} into {result}"
      ],
      "argumentParsing": "Single-quoted, double-quoted, and unquoted tokens accepted. {var} placeholders resolved from runtime memory before invocation.",
      "captureKeyword": "'into' (primary) or 'to' (alias) followed by {variable_name}",
      "scriptAlias": "@script: {alias} = package.function declares a file-local alias. The parser rewrites it to the registered handler name.",
      "resolution": "The dotted name is normalized and looked up in the process-global Go-call registry (RegisterGoCall). There is NO filesystem/module import — only pre-registered handlers are callable.",
      "restrictions": [
        "The named handler MUST have been registered via RegisterGoCall before the mission runs.",
        "Only valid as a hook instruction ([SETUP]/[TEARDOWN]) or an inline mission step."
      ]
    },

    "printSyntax": {
      "format": "PRINT \"message with {vars}\"",
      "description": "Variable-interpolated console output. Valid inside [SETUP]/[TEARDOWN] blocks and as a mission step."
    }
  },

  "goRegistration": {
    "description": "ManulEngine (Go) extends the runtime through Go function registration at process init (main()/init()/TestMain), not through suite-level lifecycle decorators. This replaces ManulEngine's Python @before_all/@after_all/@before_group/@after_group hooks, which have no ManulEngine (Go) equivalent.",

    "registerGoCall": {
      "signature": "RegisterGoCall(name string, handler GoCallHandler) error",
      "handlerType": "GoCallHandler = func(context.Context, GoCallInvocation) (any, error)",
      "invocation": {
        "type": "GoCallInvocation",
        "fields": [
          { "name": "Name",      "type": "string",            "description": "Registered handler name being invoked." },
          { "name": "Args",      "type": "[]string",          "description": "Positional arguments (placeholders already resolved)." },
          { "name": "Variables", "type": "map[string]string", "description": "Flattened mission variables at call time." },
          { "name": "Page",      "type": "browser.Page",      "description": "The live page (nil for [SETUP] before launch)." },
          { "name": "Command",   "type": "dsl.Command",       "description": "The parsed CALL GO command, including the capture target." }
        ]
      },
      "returnHandling": "A returned scalar is stringified and stored at the 'into {var}' target (LevelRow). A returned map sets multiple variables. A non-nil error fails the line.",
      "backsVerb": "CALL GO"
    },

    "registerCustomControl": {
      "signature": "RegisterCustomControl(page, target string, handler CustomControlHandler) error",
      "handlerType": "CustomControlHandler = func(context.Context, browser.Page, CustomControlInvocation) error",
      "invocation": {
        "type": "CustomControlInvocation",
        "fields": [
          { "name": "Page",       "type": "string",            "description": "Page/site name the control is scoped to ('*' = any page)." },
          { "name": "Target",     "type": "string",            "description": "Human label the control overrides." },
          { "name": "ActionType", "type": "string",            "description": "The action being performed (click, fill, …)." },
          { "name": "Value",      "type": "string",            "description": "Action value, where applicable." },
          { "name": "Variables",  "type": "map[string]string", "description": "Flattened mission variables." },
          { "name": "Command",    "type": "dsl.Command",       "description": "The parsed command." }
        ]
      },
      "lookup": "GetCustomControl(page, target) resolves the page-scoped handler first, then the '*' (any-page) handler."
    },

    "getters": [
      { "name": "GetGoCall",        "signature": "GetGoCall(name string) (GoCallHandler, bool)" },
      { "name": "GetCustomControl", "signature": "GetCustomControl(page, target string) (CustomControlHandler, bool)" },
      { "name": "ResetRuntimeRegistries", "signature": "ResetRuntimeRegistries()", "description": "Clears both registries. For test fixtures only — MUST NOT be called while any Worker is running." }
    ],

    "registryPolicy": {
      "lifecycle": "All Register* calls should happen at process init; the worker pool then only reads (Get*) and invokes handlers; process exits.",
      "concurrency": "Registries are guarded by sync.RWMutex (data-race-free). Registering/unregistering while workers execute is permitted by the type system but discouraged (visibility becomes timing-dependent).",
      "handlerSafety": "Handlers MUST be safe for concurrent invocation: with --workers > 1 the same handler may run on every worker goroutine simultaneously."
    },

    "notInHeart": "No @before_all/@after_all/@before_group/@after_group decorators, no GlobalContext, no load_hooks_file(), and no MANUL_GLOBAL_VARS env serialization. These are ManulEngine (Python) only."
  },

  "suiteLifecycle": {
    "package": "pkg/lifecycle",
    "description": "Hooks around a whole run, and around every hunt carrying a given tag. A hunt's own [SETUP]/[TEARDOWN] covers one file; these cover the suite — logging in once for twenty hunts, tearing an environment down afterwards.",
    "why": "They live in the engine because the engine is what knows when a suite begins, which files it contains, and what @tags: each carries. A client looping over `run` could not select group hooks at all.",

    "globalContext": {
      "type": "lifecycle.GlobalContext",
      "variables": "Published to every hunt at LEVEL_GLOBAL (priority 4), so a hunt's own values and per-row data still shadow them.",
      "metadata": "Hook-to-hook scratch space. Never reaches a hunt.",
      "concurrency": "Mutex-guarded; Vars() returns a copy. Group hooks may run on several goroutines at once under --workers."
    },

    "hooks": [
      {
        "kind": "before_all",
        "register": "lifecycle.RegisterBeforeAll(handler)",
        "timing": "Once, before any hunt file runs and before any browser is launched.",
        "onFailure": "The suite aborts; no hunt runs. after_all still fires, because a half-finished before_all may have left something to clean up.",
        "note": "No page exists yet, so page primitives are unavailable inside it."
      },
      {
        "kind": "after_all",
        "register": "lifecycle.RegisterAfterAll(handler)",
        "timing": "Once, after every hunt, whatever happened.",
        "onFailure": "Reported, changes no result. Every remaining after_all hook still runs."
      },
      {
        "kind": "before_group",
        "register": "lifecycle.RegisterBeforeGroup(tag, handler)",
        "timing": "Before each hunt whose @tags: includes tag.",
        "onFailure": "That hunt is skipped and counted as skipped. The rest of the suite runs."
      },
      {
        "kind": "after_group",
        "register": "lifecycle.RegisterAfterGroup(tag, handler)",
        "timing": "After each hunt whose @tags: includes tag.",
        "onFailure": "Reported, changes no result."
      }
    ],

    "tagMatching": "Case- and whitespace-insensitive on both sides, so `@tags: Smoke` and tag=\"smoke\" are one group. A hunt with no tags fires no group hooks.",
    "ordering": "Handlers run in registration order. before_all and before_group stop at the first failure; the after_* hooks never short-circuit.",
    "executionModes": "Applied identically in sequential and worker-pool runs. In the pool each worker's Runtime is seeded with the suite's global variables and every hunt is bracketed by its group hooks.",
    "reset": "lifecycle.Reset() clears every registration. For test fixtures; must not be called while a suite is running.",
    "protocol": "A binding declares hooks through the session protocol's `register` command (one slot per kind+tag) and runs them via `run-suite`. See spec/protocol.md."
  },

  "scopedVariables": {
    "type": "ScopedVariables",
    "module": "pkg/runtime/variables.go",
    "description": "Five-level variable hierarchy with strict precedence resolution. Used throughout the engine for {placeholder} substitution.",

    "levels": [
      { "constant": "LevelRow",     "priority": 1, "label": "row",     "description": "Per-iteration from @data CSV/JSON rows and CALL GO ... into captures. Cleared between iterations." },
      { "constant": "LevelStep",    "priority": 2, "label": "step",    "description": "Runtime: EXTRACT, SET ... into." },
      { "constant": "LevelMission", "priority": 3, "label": "mission", "description": "File-level @var: declarations and [SETUP] hook returns." },
      { "constant": "LevelGlobal",  "priority": 4, "label": "global",  "description": "CLI/env context variables." },
      { "constant": "LevelImport",  "priority": 5, "label": "import",  "description": "@var: declarations inherited from @import: source files. Lowest priority — overridden by all other levels." }
    ],
    "resolutionOrder": "Highest priority first: row → step → mission → global → import. First match wins.",

    "methods": [
      { "name": "Set",          "signature": "Set(name, value string, level Level)" },
      { "name": "Resolve",      "signature": "Resolve(name string) (string, bool)" },
      { "name": "ResolveLevel", "signature": "ResolveLevel(name string) (string, Level, bool)" },
      { "name": "ClearLevel",   "signature": "ClearLevel(level Level)" },
      { "name": "ClearAll",     "signature": "ClearAll()" },
      { "name": "Flatten",      "signature": "Flatten() map[string]string", "description": "Merge all levels into one map, honoring precedence (higher priority overwrites)." },
      { "name": "Interpolate",  "signature": "Interpolate(s string) string", "description": "Replace $var, ${var}, and {var} placeholders. Keys sorted longest-first to avoid partial matches." },
      { "name": "String",       "signature": "String() string", "description": "Debug dump of the merged scope." }
    ]
  }
}
```
