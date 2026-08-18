# Reports & Explainability

> **Manul Browser 0.1.1**

Manul Browser provides multiple layers of observability: HTML reports with screenshots, per-channel scoring breakdowns, and an interactive debugger with a read-only explain-next preview.

## HTML Reports

### Generating a report

```bash
manul --html-report examples/
manul --html-report --screenshot on-fail examples/   # screenshots on failure
manul --html-report --screenshot always examples/    # screenshot every step
```

Each hunt gets a styled per-hunt HTML report under `reports/`; parallel/directory runs additionally get an aggregate `reports/index.html` with one row per hunt and worker attribution.

### Screenshot modes

| Mode | CLI flag | Behavior |
|------|----------|----------|
| **on-fail** (default) | `--screenshot on-fail` | Captures a screenshot only when a step fails |
| **always** | `--screenshot always` | Captures a screenshot after every step (forensic) |
| **none** | `--screenshot none` | No screenshots |

The `SCREENSHOT ["<name>"]` DSL step additionally saves an on-demand PNG under `screenshots/`.

### Run history

Each test run appends a JSON Lines entry to `reports/run_history.json`:

```json
{"file": "examples/login.hunt", "name": "login.hunt", "timestamp": "2026-07-02T10:30:00+00:00", "status": "pass", "duration_ms": 3400}
```

Each row is `{file, name, timestamp, status, duration_ms}`, so a dashboard can read the file to display sparkline history per hunt file.

> **Note:** there is no report-session merging — the engine writes one report per run, plus the aggregate index.

---

## Explain Mode

Explain mode reveals exactly how the engine scored and selected each element. This is the primary debugging tool when a step resolves to the wrong element or fails unexpectedly.

### Enabling explain mode

```bash
manul --explain examples/login.hunt
```

Or set `"explain_mode": true` in `manul.config.json` (env: `MANUL_EXPLAIN`).

### Reading the output

For each targeted step, explain mode prints the top-5 candidate ranking with per-channel signals:

```text
explain: top 5 for "Login"
  #1 score=0.593 conf=9/10 <button> "Login"
      xpath=/html/body/main/form/button[1]
  #2 score=0.112 conf=7/10 <a> "Login help"
      xpath=/html/body/footer/a[2]
```

The channels behind the total (text · id · semantics · proximity, plus visibility/interactability penalties) are defined by `contracts/MANUL_SCORING_CONTRACT.md`.

### Machine-readable outcomes

For drivers and CI, prefer the structured outputs over parsing logs:

```bash
manul run --json file.hunt        # full HuntResult JSON on stdout
manul run --jsonl file.hunt       # per-step JSON Lines + final HuntResult
manul run-step "Click 'Login'"    # compact StepOutcome JSON (default)
```

`StepOutcome.reason` ∈ `ok · not_found · ambiguous · timeout · verify_failed · action_failed`; failed/low-confidence steps carry `near` candidates with scores.

---

## Interactive Debugger

```bash
manul --debug examples/login.hunt              # pause before every step
manul --debug --break-lines 12,20 file.hunt    # pause only at those lines
```

At a pause (TTY): `next` · `continue` · `debug-stop` · `highlight <xpath>` · `explain` · `abort`. An in-browser modal shows the paused step with an Abort button; the resolved target is highlighted on the page.

`explain-next` (emitted as a marker in pipe mode, also available as `explain` in the TTY) is a **read-only scoring preview** of the upcoming step — it ranks the live DOM without executing anything, so you can edit the step text and re-preview until the confidence is right.

> **Note:** the preview is read-only by design — it ranks the step without executing it, so previewing can never change page state.

The pause/explain wire protocol (`MANUL_DEBUG_PAUSE` / `MANUL_EXPLAIN_NEXT` NUL-markers on stdout) is specified in [`spec/contracts/MANUL_DEBUG_CONTRACT.md`](../../spec/contracts/MANUL_DEBUG_CONTRACT.md).
