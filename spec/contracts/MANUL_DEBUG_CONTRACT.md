# Manul — Debug, Explain & What-If Contract

> **Machine-readable contract for interactive debugging, the pause/explain wire protocol, explain-next scoring previews, and the What-If dry-run REPL.**
> Consumed by pipe-mode debug drivers, CI/CD diagnostics, and downstream tooling.
>
> **Wire protocol stability.** This file is the authoritative byte-level spec
> for the stdin/stdout debug protocol: pause marker, explain-next marker,
> command tokens, 1-based step index. The What-If REPL added below is
> **terminal-only** and introduces no new markers, so no pipe-mode driver
> needs to change.

```json
{
  "version": "0.2.0",
  "generatedFrom": "pkg/runtime/debug.go :: shouldPause(), debugPrompt(), debugPromptTTY(), debugPromptExtension(), injectDebugModal(), debugHighlight(), explainStep(), buildExplainNextResult(), explainNextPayload; pkg/runtime/whatif.go :: WhatIfResult, evaluateWhatIf(), runWhatIfREPL(), pickWhatIfStep(), takeWhatIfReplacement(), isWhatIfSystemStep(); pkg/runtime/runtime.go :: runCommands() replacement loop; pkg/config/config.go :: DebugMode, BreakLines, ExplainMode",

  "config": {
    "DebugMode": {
      "jsonKey": "debug_mode",
      "flag": "--debug",
      "env": "MANUL_DEBUG",
      "type": "bool",
      "default": false,
      "description": "Pause execution for interactive stepping. With no breakpoints set, pauses before every DSL command."
    },
    "BreakLines": {
      "jsonKey": "break_lines",
      "flag": "--break-lines",
      "type": "[]int",
      "default": [],
      "description": "1-based .hunt line numbers that act as breakpoints. Empty means pause on every step (when DebugMode is on). CLI accepts a comma-separated list."
    },
    "ExplainMode": {
      "jsonKey": "explain_mode",
      "flag": "--explain",
      "type": "bool",
      "default": false,
      "description": "Print a full scoring breakdown for every targeted element (non-interactive)."
    }
  },

  "pauseDecision": {
    "function": "shouldPause(cmd, idx) bool",
    "state": [
      { "name": "debugContinue", "type": "bool", "description": "Set by 'continue'/'debug-stop'. Suppresses pauses until a breakLine is hit (then re-arms) or for the rest of the run." },
      { "name": "breakLines",    "type": "map[int]bool", "description": "Active line-number breakpoints (from --break-lines)." },
      { "name": "breakSteps",    "type": "map[int]bool", "description": "One-shot index breakpoints appended by the 'next' command." }
    ],
    "rules": [
      "If debugContinue is set: pause only when breakLines[cmd.LineNum] matches (which re-arms debugContinue=false); otherwise skip.",
      "If both breakLines and breakSteps are empty: pause on every step (plain DebugMode).",
      "Otherwise: pause when breakLines[cmd.LineNum] OR breakSteps[idx] matches.",
      "A step substituted by the What-If REPL is never paused on again — the decision was already made at this breakpoint."
    ]
  },

  "promptModes": {
    "selection": "debugPrompt() routes to TTY mode when stdin is a TTY, otherwise to extension (line-protocol) mode.",
    "tty": {
      "function": "debugPromptTTY()",
      "ui": "Readline prompt ('  > '). Injects the in-browser modal and prints the command list.",
      "commands": ["next", "continue", "debug-stop", "highlight <xpath>", "explain", "e | explain-next", "w | what-if", "abort"]
    },
    "pipe": {
      "function": "debugPromptPipe()",
      "transport": "Reads command tokens as stdin lines (1 MB line cap). Emits NUL-delimited markers on stdout.",
      "commands": ["next", "continue", "debug-stop", "abort", "highlight", "highlight <xpath>", "explain-next", "explain", "explain-next <json-override>", "what-if (rejected)"]
    }
  },

  "wireProtocol": {
    "pauseMarker": {
      "format": "\\x00MANUL_DEBUG_PAUSE\\x00<json>\\n",
      "payload": { "step": "string — the raw step line", "idx": "int — 1-based step index" },
      "emittedOn": "Entering a pause, and re-emitted after each non-terminal command (highlight/explain/what-if) to re-prompt."
    },
    "explainMarker": {
      "format": "\\x00MANUL_EXPLAIN_NEXT\\x00<json>\\n",
      "payload": "explainNextPayload (see explainNext.fields)"
    },
    "commandSemantics": {
      "next":         "Pause again at the next step (appends a one-shot breakStep at idx+1). Empty line == next.",
      "continue":     "Clear step breakpoints and resume (debugContinue).",
      "debug-stop":   "Clear ALL breakpoints (lines + steps) and resume to completion.",
      "abort":        "Stop the mission (ErrDebugStop).",
      "highlight":    "Scroll the current highlight into view (no arg) or highlight the given xpath.",
      "explain-next": "Emit an explain marker for the current step, or for a step override via 'explain-next {\"step\":\"...\"}'. Read-only.",
      "what-if":      "Rejected in pipe mode: stdin is reserved for control tokens, so an interactive REPL cannot read from it. The engine logs a notice, re-emits the pause marker, and stays paused. Use explain-next instead."
    }
  },

  "explainNext": {
    "function": "buildExplainNextResult(stepText, cmd) explainNextPayload",
    "description": "Read-only scoring preview for the current (or overridden) step. Ranks the live snapshot via scorer.Rank and projects the top candidate. Highlights the best candidate in the page. This is the pipe-mode surface and its wire shape is unchanged from v0.1.0.",
    "fields": [
      { "name": "step",             "json": "step",             "type": "string" },
      { "name": "score",            "json": "score",            "type": "float64", "description": "Top candidate total score (raw, not the 0-10 confidence)." },
      { "name": "confidence_label", "json": "confidence_label", "type": "string",  "description": "high (>=0.5) | medium (>=0.1) | low (>0) | none (0)." },
      { "name": "target_found",     "json": "target_found",     "type": "bool",    "description": "True when top score > 0." },
      { "name": "target_element",   "json": "target_element",   "type": "string?", "description": "XPath of the top candidate (null when none)." },
      { "name": "explanation",      "json": "explanation",      "type": "string",  "description": "Channel breakdown: text / id / semantic / penalty." },
      { "name": "risk",             "json": "risk",             "type": "string",  "description": "Set to a low-confidence warning when top score < 0.1." },
      { "name": "suggestion",       "json": "suggestion",       "type": "string?", "description": "Next-best candidate when the top is low-confidence." },
      { "name": "heuristic_score",  "json": "heuristic_score",  "type": "float64?" },
      { "name": "heuristic_match",  "json": "heuristic_match",  "type": "string?" }
    ],
    "confidenceThresholds": { "high": ">= 0.5", "medium": ">= 0.1", "low": "> 0", "none": "0" },
    "ttyExplain": "explainStep() prints the top 5 candidates (score, conf/10, tag, truncated text, xpath) to the log instead of emitting a marker."
  },

  "whatIf": {
    "availability": "Terminal (TTY) mode only. Entered with 'w' or 'what-if' at a debug pause. In pipe mode the token is rejected; see wireProtocol.commandSemantics.what-if.",
    "entryPoints": {
      "oneShot": "'e' or 'explain-next' at the TTY prompt evaluates the step currently paused on, prints WhatIfResult.FormatReport(), and stays paused.",
      "repl": "'w' or 'what-if' opens the interactive dry-run loop (runWhatIfREPL)."
    },

    "replCommands": [
      { "command": "<any step text>", "description": "Evaluate a hypothetical step and append it to the session history." },
      { "command": "!history",        "description": "List every evaluation this session with a 1-based index, score, and label." },
      { "command": "!execute",        "description": "Accept the most recent evaluation and run it in place of the paused step." },
      { "command": "!execute N",      "description": "Accept evaluation #N (1-based) from history and run it." },
      { "command": "!context",        "description": "Print the current page URL and title." },
      { "command": "!help",           "description": "Print the REPL help box." },
      { "command": "!quit",           "description": "Leave the REPL without executing anything; the original step proceeds." }
    ],

    "whatIfResult": {
      "type": "pkg/runtime.WhatIfResult",
      "note": "Terminal-facing only — it is never serialized to the wire. The extension continues to receive explainNextPayload.",
      "fields": [
        { "name": "Step",           "type": "string" },
        { "name": "Score",          "type": "int",     "description": "0-10 confidence, mapped from the raw scorer total by scoreToConfidence()." },
        { "name": "TargetFound",    "type": "bool" },
        { "name": "TargetElement",  "type": "string",  "description": "XPath of the top candidate; empty when none." },
        { "name": "Explanation",    "type": "string" },
        { "name": "Risk",           "type": "string" },
        { "name": "Suggestion",     "type": "string",  "description": "Emitted when a runner-up is close behind and Score < 8, i.e. the phrasing is ambiguous." },
        { "name": "HeuristicScore", "type": "float64" },
        { "name": "HeuristicMatch", "type": "string" }
      ],
      "confidenceScale": [
        { "score": "0",    "label": "IMPOSSIBLE", "meaning": "Target element does not exist." },
        { "score": "1-4",  "label": "LOW",        "meaning": "Ambiguous target, or disabled/hidden." },
        { "score": "5-7",  "label": "MODERATE",   "meaning": "Plausible target, some ambiguity." },
        { "score": "8-10", "label": "HIGH",       "meaning": "Clear, unique target element found." }
      ],
      "scoreMapping": "Shared with explain-next via scoreToConfidence(): >=1.0 → 10, >=0.5 → 9, >=0.1 → 7, >=0.05 → 5, >=0.01 → 3, >0 → 1, else 0.",
      "unscoredVsZero": "A step that was scored and lost (0.0) renders a 'Heuristic Score' line; a step that was never scored (system step, parse failure) omits it. The two are distinguished internally by hasHeuristic, not by the score value."
    },

    "systemSteps": {
      "function": "isWhatIfSystemStep(cmd) bool",
      "description": "Steps that complete without resolving an element are forced to Score 8 / TargetFound true, so they are not reported IMPOSSIBLE merely because nothing on the page matched.",
      "types": ["NAVIGATE", "WAIT", "WAIT_FOR", "WAIT_FOR_RESPONSE", "SCROLL", "PRESS", "PRINT", "SCREENSHOT", "SET (only when it assigns a variable)"],
      "systemStepFields": "A system step is decided before any snapshot is taken — it needs no DOM — so the heuristic fields are absent for these steps rather than reported as scores nothing produced."
    },

    "stepReplacement": {
      "signal": "errWhatIfReplace, returned by debugPrompt() when the REPL exits via !execute. It is a control signal, never a failure.",
      "carrier": "Runtime.whatIfExecuteStep holds the chosen text until runCommands() consumes it.",
      "flow": [
        "1. The REPL returns the chosen step; debugPromptTTY stores it and returns errWhatIfReplace.",
        "2. runCommands() calls takeWhatIfReplacement(), which resolves {variables} and parses the text.",
        "3. The substitute replaces the current command and re-enters the step loop with the pause suppressed.",
        "4. The abandoned step is discarded before it can reach the report, the onStep stream, or the pass/fail tally."
      ],
      "actionSteps": "For element-resolving commands the pause happens inside executeCommand() after resolution, so the signal travels out as the step's error and is intercepted in runCommands() before the result is recorded.",
      "unparseableChoice": "A chosen step that fails to parse logs a warning and keeps the original step. A typo in the REPL never aborts the run.",
      "replayGuard": "whatIfExecuteStep is cleared whenever the signal is handled, including on the parse-failure path, so a later error cannot replay it."
    },

    "readOnlyGuarantee": "evaluateWhatIf() parses the step, takes a DOM snapshot, and ranks candidates. It never dispatches the action, so no click, fill, or navigation can result from an evaluation. The only visible effect is the debug highlight, which clearDebugHighlight() removes.",

    "highlightLifecycle": [
      "1. Debug pause → debugHighlight(xpath) marks the current step's resolved target.",
      "2. Each REPL evaluation → clearDebugHighlight() then debugHighlight() on the new best candidate.",
      "3. Leaving the REPL → clearDebugHighlight() via defer, on every exit path including !execute."
    ],

    "sessionScope": "Runtime.whatIfHistory accumulates for the lifetime of the Runtime, so !history spans multiple pauses within one run. It is not persisted across runs."
  },

  "inBrowserModal": {
    "inject": "injectDebugModal(step) renders a draggable overlay showing the paused step with an Abort button.",
    "abortSignal": "The Abort button sets window.__manul_debug_action; the prompt loop polls it every 200ms ('abort' in TTY, 'ABORT' in extension mode) and returns ErrDebugStop.",
    "highlight": "debugHighlight(xpath) marks the resolved element (data-manul-debug-highlight) and scrolls it into view; clearDebugHighlight() removes it.",
    "duringRepl": "The abort poll is suspended while the What-If REPL holds stdin. Abort remains available by leaving the REPL with !quit."
  }
}
```

---

## Quick Reference

### Entering What-If mode

| Mode | Trigger | Behaviour |
|------|---------|-----------|
| Terminal (one-shot) | `e` or `explain-next` at the debug prompt | Prints the report, stays paused |
| Terminal (REPL) | `w` or `what-if` at the debug prompt | Interactive loop over stdin/stdout |
| Extension (one-shot) | `explain-next\n` on stdin | Emits `\x00MANUL_EXPLAIN_NEXT\x00{json}\n`, stays paused |
| Extension (REPL) | — | Rejected; stdin is reserved for control tokens |

### REPL commands

| Command | Description |
|---------|-------------|
| `<step text>` | Evaluate a hypothetical step (dry-run) |
| `!history` | Show all evaluations with index, score, and label |
| `!execute` | Accept the last evaluated step for execution |
| `!execute N` | Accept evaluation #N from history |
| `!context` | Show current page URL and title |
| `!help` | Show help text |
| `!quit` | Exit without executing anything |

### Confidence scale

| Score | Label | Meaning |
|-------|-------|---------|
| 0 | IMPOSSIBLE | Target element does not exist |
| 1–4 | LOW | Ambiguous target or disabled/hidden |
| 5–7 | MODERATE | Plausible target, some ambiguity |
| 8–10 | HIGH | Clear, unique target element found |
