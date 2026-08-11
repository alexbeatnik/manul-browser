# Where the Python engine's tests went

The standalone Python engine carried 53 test files, ~24k lines. They did not
disappear with it. Most were ported to Go before the merge; the rest are
accounted for below.

This exists so "did we lose test coverage?" has an answer that is checkable
rather than reassuring.

## Already in Go: the scoring and behaviour suites

These were ported to `core/pkg/scorer/synthetic/` as the Go engine was written.
They are the bulk of the suite — DOM fixtures and expected resolutions.

| Python | Go |
|---|---|
| `test_01_ecommerce.py` | `synthetic/ecommerce_dom_test.go` |
| `test_02_social.py` | `synthetic/social_media_test.go` |
| `test_03_saas.py` | `synthetic/saas_admin_test.go` |
| `test_04_travel.py` | `synthetic/travel_test.go` |
| `test_05_fintech.py` | `synthetic/fintech_test.go` |
| `test_06_media.py` | `synthetic/media_test.go` |
| `test_07_gov_health.py` | `synthetic/gov_health_test.go` |
| `test_08_crm.py` | `synthetic/crm_test.go` |
| `test_09_edtech.py` | `synthetic/edtech_test.go` |
| `test_10_mess.py` | `synthetic/mess_test.go` |
| `test_11_cyber.py` | `synthetic/cyber_test.go` |
| `test_12_qa_classics.py` | `synthetic/qa_classics_test.go` |
| `test_13_facebook_final_boss.py` | `synthetic/facebook_test.go` |
| `test_15_frontend_hell.py` | `synthetic/frontend_hell_test.go` |
| `test_16_disambiguation.py` | `synthetic/disambiguation_test.go` |
| `test_21_advanced_interactions.py` | `synthetic/advanced_interactions_test.go` |
| `test_24_wikipedia_search.py` | `synthetic/wikipedia_search_test.go` |
| `test_28_heuristic_weights.py` | `synthetic/heuristic_weights_test.go` |
| `test_29_visibility_treewalker.py` | `synthetic/visibility_test.go` |
| `test_30_verify_enabled.py` | `synthetic/verify_enabled_test.go` |
| `test_32_verify_checked.py` | `synthetic/verify_checked_test.go` |
| `test_34_scoring_math.py` | `synthetic/scoring_math_test.go` |
| `test_41_explain_mode.py` | `synthetic/explain_mode_test.go` |
| `test_43_attribute_semantic.py` | `synthetic/attribute_semantic_test.go` |
| `test_44_contextual_proximity.py` | `synthetic/contextual_proximity_test.go` |

## Already in Go: engine internals

| Python | Go |
|---|---|
| `test_00_engine.py` | `pkg/runtime`, `pkg/dsl` |
| `test_18_variables.py`, `test_19_dynamic_vars.py` | `pkg/runtime/variables_test.go` |
| `test_40_scoped_variables.py` | `pkg/runtime/variables_scope_test.go` |
| `test_20_tags.py`, `test_35_enterprise_dsl.py`, `test_48_exports.py` | `pkg/dsl/enterprise_dsl_test.go` |
| `test_26_logical_steps.py` | `pkg/dsl/logical_steps_test.go` |
| `test_36_set_and_indent.py` | `pkg/runtime/set_var_test.go` |
| `test_46_imports.py` | `pkg/dsl` import resolution |
| `test_50_conditionals.py`, `test_51_loops.py` | `pkg/runtime/synthetic_logic_test.go` |
| `test_22_reporting.py`, `test_23_reporter.py` | `pkg/report` |
| `test_27_iframe_routing.py` | `pkg/cdp/contexts_test.go`, `pkg/runtime/runtime_review_test.go` |
| `test_33_scanner.py`, `test_53_full_scan.py` | `pkg/scan`, `pkg/agent/agent_test.go` |
| `test_37_open_app.py` | `pkg/dsl/parser_test.go` |
| `test_38_recorder.py` | `pkg/record` |
| `test_39_scheduler.py` | `pkg/daemon` |
| `test_42_api.py` | `pkg/agent` |
| `test_25_lifecycle_hooks.py` | `pkg/runtime/setup_teardown_test.go` (`[SETUP]`/`[TEARDOWN]`) |
| `test_49_explain_next.py` | `pkg/runtime/whatif_test.go` |

## Re-tested across the wire

These tested Python code that intercepts a step. That code no longer runs inside
the engine — it runs in the client and the engine calls back — so the same
guarantees are tested through the binding instead.

| Python | Now |
|---|---|
| `test_17_custom_controls.py` | `bindings/python/tests/test_controls.py`, `pkg/runtime/extensions_test.go` |
| `test_31_call_python_args.py` | `bindings/python/tests/test_controls.py`, `pkg/dsl/call_alias_test.go` |
| `test_14_hooks.py` (suite decorators) | `pkg/lifecycle/lifecycle_test.go`, `bindings/python/tests/test_hooks.py` |
| `test_52_wait_for_selector.py` | `pkg/dsl/new_verbs_test.go` — `WAIT FOR SELECTOR` is implemented, not omitted |

What carried over: registration keyed on (page, target), case- and
whitespace-insensitive lookup, wildcard pages, page-specific beating wildcard,
handler signature enforcement at registration, sibling-page miss diagnostics,
`list_custom_controls()`, positional `CALL` arguments, `into {var}` capture, and
an object return becoming variables.

What changed: the handler receives a `ControlContext` with `eval()` and
`current_url()` rather than a live `Page` object — the engine is in another
process. `@custom_control` is still a module-level decorator applied at import
time, as it was.

## Not carried over

| Python | Why |
|---|---|
| `test_47_packager.py` | Packaged a Python module for distribution. The engine is a binary now; packaging is the wheel's job. |
| `test_45_prompts_config.py` | `prompts/` shipped LLM prompt templates with the Python engine. Not part of the engine. |
| `MANUL_GLOBAL_VARS` | Was how Python passed `@before_all` results to subprocess workers. Not needed: the suite's `GlobalContext` seeds each worker's runtime directly. |

## Nothing outstanding

`test_14_hooks.py` and the suite-level decorators are covered too:
`[SETUP]`/`[TEARDOWN]` by `pkg/runtime/setup_teardown_test.go`, and
`@before_all` / `@after_all` / `@before_group` / `@after_group` by
`pkg/lifecycle` with `bindings/python/tests/test_hooks.py` exercising them
through the protocol.

Everything from the original suite is now either ported to Go, re-tested through
the binding, or listed above as deliberately out of scope.

## Verb parity

Comparing the two DSL contracts verb by verb leaves nothing outstanding:
`WAIT FOR SELECTOR`, `FULL SCAN` and `SCAN PAGE` are implemented, and
`CALL PYTHON` is an accepted spelling of `CALL HOST`.

Comparing declared verbs was not enough, though. `WAIT FOR` and `HIGHLIGHT` were
both *listed* in the Go contract and both failed at runtime with "not yet
implemented" — declared, parsed, documented, and dead. `pkg/runtime` now carries
a guard test that walks every `CommandType` and refuses that error, plus a
companion test proving the guard can fail. A verb reaching a contract without
reaching the runtime is exactly the kind of gap a verb-list diff cannot see.
