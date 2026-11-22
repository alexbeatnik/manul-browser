# Manul Browser — Reporting Contract

> **Machine-readable contract for the Manul Browser reporting and result pipeline.**
> Consumed by the HTML report generator, CI/CD integrations, and downstream analytics.
>
> **Scope.** The result model (`StepResult`/`BlockResult`/`MissionResult`/`RunSummary`)
> and especially the cross-consumed **`run_history.json` record** (`{file, name,
> timestamp, status, duration_ms}`) is fixed: every downstream reader depends on
> it. It is produced by `pkg/report`; the HTML markup around it is not part of
> this contract.

```json
{
  "version": "0.1.6",
  "generatedFrom": "pkg/report :: AppendRunHistory(), GenerateHTML(), RunSummary; pkg/explain :: HuntResult",

  "statusValues": {
    "step": ["pass", "fail", "skip", "warning"],
    "block": ["pass", "fail", "warning"],
    "mission": ["pass", "fail", "broken", "flaky", "warning"],
    "note": "'broken' = [SETUP] hook failed (browser never launched). 'flaky' = failed initially but passed on retry. 'warning' = VERIFY SOFTLY failures collected."
  },

  "dataclasses": {
    "StepResult": {
      "module": "pkg/report",
      "fields": [
        { "name": "index",        "type": "int",            "default": null, "required": true,  "description": "1-based step number within the mission." },
        { "name": "text",         "type": "str",            "default": null, "required": true,  "description": "Step text after variable substitution." },
        { "name": "status",       "type": "str",            "default": "pass",                   "description": "One of: pass, fail, skip, warning." },
        { "name": "duration_ms",  "type": "float",          "default": 0.0,                      "description": "Execution time in milliseconds." },
        { "name": "error",        "type": "str | null",     "default": null,                     "description": "Error message or traceback on failure." },
        { "name": "screenshot",   "type": "str | null",     "default": null,                     "description": "Path of the PNG saved for this step under screenshots/, relative to the working directory and with forward slashes (JSON key screenshot_path; controlled by the screenshot mode)." },
        { "name": "logical_step", "type": "str | null",     "default": null,                     "description": "Active STEP label when this action ran (e.g. 'STEP 2: Login')." }
      ]
    },

    "BlockResult": {
      "module": "pkg/report",
      "fields": [
        { "name": "name",        "type": "str",                  "default": null, "required": true,  "description": "STEP label (e.g. 'STEP 2: Login')." },
        { "name": "status",      "type": "str",                  "default": "pass",                   "description": "One of: pass, fail, warning." },
        { "name": "duration_ms", "type": "float",                "default": 0.0,                      "description": "Total duration of all child actions." },
        { "name": "error",       "type": "str | null",           "default": null,                     "description": "First error encountered in block." },
        { "name": "actions",     "type": "list[StepResult]",     "default": [],                       "description": "Ordered child actions within this block." }
      ]
    },

    "MissionResult": {
      "module": "pkg/report",
      "fields": [
        { "name": "file",        "type": "str",                  "default": null, "required": true,  "description": "Absolute path to the .hunt file." },
        { "name": "name",        "type": "str",                  "default": null, "required": true,  "description": "File basename (e.g. 'saucedemo.hunt')." },
        { "name": "status",      "type": "str",                  "default": "pass",                   "description": "One of: pass, fail, broken, flaky, warning." },
        { "name": "attempts",    "type": "int",                  "default": 1,                        "description": "Total attempts including initial run + retries." },
        { "name": "duration_ms", "type": "float",                "default": 0.0,                      "description": "Wall-clock ms for all attempts." },
        { "name": "error",       "type": "str | null",           "default": null,                     "description": "Last error message when status is fail/broken." },
        { "name": "steps",       "type": "list[StepResult]",     "default": [],                       "description": "Flat ordered list of all executed step results." },
        { "name": "blocks",      "type": "list[BlockResult]",    "default": [],                       "description": "Hierarchical block grouping (STEP-based)." },
        { "name": "tags",        "type": "list[str]",            "default": [],                       "description": "Tags from @tags: header in the .hunt file." },
        { "name": "soft_errors", "type": "list[str]",            "default": [],                       "description": "Collected VERIFY SOFTLY failure messages." }
      ],
      "specialMethods": {
        "__bool__": "Returns True when status is NOT in ('fail', 'broken'). Truthy = usable result."
      }
    },

    "RunSummary": {
      "module": "pkg/report",
      "fields": [
        { "name": "session_id",       "type": "str",                  "default": "<ISO timestamp>_<PID>",  "description": "Unique session identifier for report merging." },
        { "name": "invocation_count", "type": "int",                  "default": 1,                         "description": "Number of CLI invocations merged in this session." },
        { "name": "started_at",       "type": "str",                  "default": "",                        "description": "ISO-8601 start timestamp." },
        { "name": "ended_at",         "type": "str",                  "default": "",                        "description": "ISO-8601 end timestamp." },
        { "name": "total",            "type": "int",                  "default": 0,                         "description": "Total number of missions." },
        { "name": "passed",           "type": "int",                  "default": 0,                         "description": "Missions with status 'pass'." },
        { "name": "failed",           "type": "int",                  "default": 0,                         "description": "Missions with status 'fail'." },
        { "name": "broken",           "type": "int",                  "default": 0,                         "description": "Missions with status 'broken'." },
        { "name": "flaky",            "type": "int",                  "default": 0,                         "description": "Missions with status 'flaky'." },
        { "name": "warning",          "type": "int",                  "default": 0,                         "description": "Missions with status 'warning'." },
        { "name": "duration_ms",      "type": "float",                "default": 0.0,                      "description": "Total wall-clock duration." },
        { "name": "missions",         "type": "list[MissionResult]",  "default": [],                       "description": "All mission results in execution order." }
      ]
    }
  },

  "persistenceLayer": {
    "runHistory": {
      "file": "reports/run_history.json",
      "format": "JSON Lines (one JSON object per line, newline-delimited)",
      "writeMethod": "os.open(..., O_APPEND | O_CREAT | O_WRONLY) for concurrent-safe appends",
      "schema": {
        "file":        "str — absolute path to .hunt file",
        "name":        "str — file basename",
        "timestamp":   "str — ISO-8601 datetime",
        "status":      "str — pass | fail | flaky | warning. A hunt that never ran (broken) is not recorded here.",
        "duration_ms": "float — rounded to 1 decimal"
      },
      "appendedBy": [
        "cmd/manul — sequential and parallel execution paths",
        "cmd/manul — failure/retry paths",
        "pkg/daemon — _run_scheduled_job()"
      ]
    },

    "reportState": {
      "file": null,
      "note": "Not implemented. There is no report-session state, no TTL and no merging across invocations; MANUL_REPORT_SESSION_TTL_SEC has no effect."
    },

    "htmlReport": {
      "file": "reports/report_<title>_<YYYYMMDD_HHMMSS>_<seq>.html",
      "scope": "One file per hunt result — per row for a hunt with @data:. There is no aggregate report.",
      "generator": "pkg/report :: GenerateHTML(result *explain.HuntResult, outDir string) (string, error)",
      "features": [
        "Self-contained dark-themed HTML (no external dependencies)",
        "PASS/FAIL badge, hunt title and file, a Flaky line when it passed on a retry",
        "Total / Passed / Failed / Duration stats",
        "One row per step with its duration and error",
        "Screenshots linked by relative path (../screenshots/…), not inlined",
        "A Soft Verify Warnings list when VERIFY SOFTLY failed"
      ]
    },

    "outputDirectory": {
      "path": "reports/",
      "autoCreated": true,
      "gitignored": true
    }
  },

  "functions": {
    "AppendRunHistory": {
      "signature": "AppendRunHistory(reportsDir string, result *explain.HuntResult) error",
      "description": "Appends one JSON Line to <reportsDir>/run_history.json, creating the directory and the file as needed."
    },
    "GenerateHTML": {
      "signature": "GenerateHTML(result *explain.HuntResult, outDir string) (string, error)",
      "description": "Writes one hunt's HTML report into outDir and returns its path."
    }
  }
}
```
