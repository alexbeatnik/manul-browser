# Manul Browser — Scoring & Heuristics Contract

> **Machine-readable contract for the DOMScorer heuristic pipeline and element snapshot shape.**
> Consumed by explain-mode UIs, custom scoring extensions, benchmark tooling, and downstream analytics.
>
> **Scope.** Scoring channels, `WEIGHTS`, `SCALE`, thresholds and the
> element-snapshot field shape (`cache 2.0 · semantics 0.60 · text 0.45 ·
> attributes 0.25 · proximity 0.10`), as implemented under `pkg/...`.

```json
{
  "version": "0.1.6",
  "generatedFrom": "pkg/scorer/scorer.go :: DOMScorer, WEIGHTS, SCALE, score_elements(); pkg/dom (snapshot JS) :: SNAPSHOT_JS",

  "constants": {
    "WEIGHTS": {
      "cache":      2.0,
      "text":       0.45,
      "attributes": 0.25,
      "semantics":  0.60,
      "proximity":  0.10
    },
    "SCALE": 177778,
    "MAX_THEORETICAL_SCORE": 177778,
    "note": "Final integer score = (Σ channel_score × channel_weight) × penalty_multiplier × SCALE. Explain mode normalises back to [0.0, 1.0] by dividing by MAX_THEORETICAL_SCORE."
  },

  "scoringPipeline": {
    "order": [
      "1. _preprocess() — attach normalised strings and type classifications to each element",
      "2. _score_cache_reuse() — semantic cache hit or blind context reuse",
      "3. _score_text_match() — aria, placeholder, data-qa, name, icons, name_attr matching",
      "4. _score_attributes() — target_field, html_id, context words, NEAR anchor affinity",
      "5. _score_semantics() — element-type alignment, mode synergy, cross-mode penalties",
      "6. _score_proximity() — contextual: NEAR (Euclidean), ON HEADER/FOOTER (region), INSIDE (subtree), default (xpath depth)",
      "7. _calculate_penalties() — disabled/hidden multipliers",
      "8. Final = (Σ weighted channels) × penalty × SCALE",
      "9. Sort descending by score. On a tie a descendant ranks above its own ancestor, so a wrapper holding nothing but the target does not stand in for it; otherwise document order decides"
    ],
    "contextualBoost": "When a contextual qualifier (NEAR/ON HEADER/ON FOOTER/INSIDE) is active AND has effective resolved data, proximity channel weight increases from 0.10 to 1.5."
  },

  "channels": [
    {
      "name": "cache",
      "weight": 2.0,
      "method": "_score_cache_reuse(el)",
      "returnRange": "[0.0, 1.05]",
      "signals": [
        { "signal": "Semantic cache hit (learned_elements)", "boost": 1.0, "scaledApprox": 355000 },
        { "signal": "Blind context reuse (same xpath as last step)", "boost": 0.05, "scaledApprox": 17000 }
      ],
      "shortCircuits": [
        { "threshold": 200000, "condition": "Semantic cache reuse — pkg/runtime skips further scoring" },
        { "threshold": 10000,  "condition": "Blind/context reuse — pkg/runtime skips further scoring" }
      ]
    },
    {
      "name": "text",
      "weight": 0.45,
      "method": "_score_text_match(el)",
      "returnType": "tuple[float, bool]",
      "returnRange": "[0.0, 1.0+]",
      "signals": [
        { "signal": "Exact data-qa match",              "boost": 1.0,    "scaledApprox": 80000 },
        { "signal": "data-qa substring match",           "boost": 0.375,  "scaledApprox": 30000 },
        { "signal": "Exact aria_label match",            "boost": 0.625,  "scaledApprox": 50000 },
        { "signal": "Exact placeholder match",           "boost": 0.625,  "scaledApprox": 50000 },
        { "signal": "Exact name field text match",       "boost": 0.625,  "scaledApprox": 50000 },
        { "signal": "name_attr exact match",             "boost": 0.0375, "scaledApprox": 3000 },
        { "signal": "name_attr substring match",         "boost": 0.0125, "scaledApprox": 1000 },
        { "signal": "Icon class match",                  "boost": "varies" }
      ],
      "secondReturn": "isPerfect: bool — True when an exact high-confidence match was found (used by semantics channel)"
    },
    {
      "name": "attributes",
      "weight": 0.25,
      "method": "_score_attributes(el)",
      "returnRange": "[0.0, 1.0+]",
      "signals": [
        { "signal": "Exact html_id match",              "boost": 0.6,   "scaledApprox": 26000 },
        { "signal": "target_field exact match",          "boost": "multi-channel (higher via combined text+attr)" },
        { "signal": "Context words in prefix/class",     "boost": "varies" },
        { "signal": "NEAR anchor affinity (dev attrs)",  "boost": "varies (contextual)" }
      ]
    },
    {
      "name": "semantics",
      "weight": 0.60,
      "method": "_score_semantics(el, is_perfect, all_els)",
      "returnRange": "[-1.0, 1.0+]",
      "note": "Can be NEGATIVE for cross-mode penalties.",
      "signals": [
        { "signal": "Element type alignment with mode",  "boost": "0.05–0.30" },
        { "signal": "Checkbox/radio mode match",          "boost": "positive alignment" },
        { "signal": "Checkbox/radio mode mismatch",       "penalty": -0.50, "scaledApprox": -53000, "note": "Ruthless penalty for non-checkbox when step says 'Check'" },
        { "signal": "File upload context alignment",      "boost": "positive" }
      ]
    },
    {
      "name": "proximity",
      "weight": 0.10,
      "contextualWeight": 1.5,
      "method": "_score_proximity(el)",
      "returnRange": "[0.0, 1.0]",
      "modes": [
        { "qualifier": "NEAR",       "scoring": "Euclidean distance to the nearest point of the anchor element + DOM depth affinity" },
        { "qualifier": "ON HEADER",  "scoring": "Element in <header>/<nav> ancestry OR top 15% of viewport" },
        { "qualifier": "ON FOOTER",  "scoring": "Element in <footer> ancestry OR bottom 15% of viewport" },
        { "qualifier": "INSIDE",     "scoring": "Container subtree membership (resolved row/container)" },
        { "qualifier": "none",       "scoring": "Default DOM xpath depth reuse" }
      ]
    }
  ],

  "penalties": {
    "disabled": { "multiplier": 0.0, "effect": "Zeroes entire score" },
    "hidden":   { "multiplier": 0.1, "effect": "90% reduction" },
    "normal":   { "multiplier": 1.0, "effect": "No penalty" },
    "stateReads": "The disabled multiplier applies when choosing an element to act on. VERIFY states and WAIT FOR rank with it ignored and read the flag afterwards, so 'is disabled' finds the disabled control it asks about."
  },

  "targetEvidence": {
    "function": "pkg/scorer :: MatchesQuery(query, element)",
    "rule": "Before an action command acts on its winner, the winner must match the target in at least one query-dependent signal: text, label, placeholder, aria-label, data-qa, id or class names. A partial match counts: a text signal carries the query if it contains the whole phrase or more than half of its significant words — the bar a label already had. Half is not enough: 'Add to cart' shares one word of two with the paragraph \"Cart: 0\", and CLICK used to click the paragraph. A winner with none fails the step with failure_reason not_found and the ranked candidates attached.",
    "why": "The ranking always has a first place, and tag semantics plus a type hint outscore the confidence threshold by themselves — \"the 'Delete account' button\" scored 0.30 against every button on a page with no such button.",
    "appliesTo": ["click", "double_click", "right_click", "hover", "fill", "type", "check", "uncheck", "select", "upload", "drag (source and destination)", "custom dropdown option", "highlight", "press … on <target>", "verify … has text/value/placeholder", "verify visual"],
    "exempt": "A control found by its position next to a matching label (restrictive pass 3): there the label carries the match and the control is ranked with an empty query, which scores no label, placeholder, aria-label, data-qa or id: those describe the control itself, not the caption it sits beside.",
    "restrictivePasses": "FILL, TYPE, SELECT and CHECK take the best match outright only if it is a control of their kind. Otherwise a match is a caption, and the control is the only one of that kind inside it if there is exactly one, else the nearest one beside it, taken when it scores clearly above the runner-up or sits clearly nearer the caption. A single control of the right kind that carries the target itself, if only in its class name, is not refused for a low score. CHECK, UNCHECK and VERIFY checked/NOT checked then narrow an element holding two or more checkboxes or radios — the caption's container, or a box ranked first beside siblings — to the one box captioned with the target, by its <label> or by the text that follows it up to the next box or line break; a container with several boxes and none so captioned fails the step with failure_reason not_found.",
    "numbers": "A number in the query must appear as a number of its own in the same text signal of the element: 'Under 4 minutes' does not match \"Under 3 minutes\", 'Page 1' does not match \"Page 10\". A number is a run of digits with any '.' or ',' separators inside it, standing as a word of its own — the 4 of \"4K\" and the 2 of \"button2\" are part of a word and are no number. MatchesQuery fails an element no signal of which carries every number of the query, and a text signal whose numbers disagree scores nothing in partial and word-overlap matching. Word overlap otherwise ignores one-character words, which is how YouTube's 'Under 4 minutes' filter used to click the 'Under 3 minutes' one.",
    "roleWords": "A word that names a kind of control — button, link, field, checkbox, dropdown, tab, icon — says nothing about which one. When the query has such a word and other words besides, it is matched without them: 'Cross button' does not match \"Host button\", and 'Help link' matches a link reading \"Help\". A query that is only a role word is compared as it is."
  },

  "domScorer": {
    "constructor": {
      "parameters": [
        { "name": "step",               "type": "str",                         "required": true,  "description": "Raw step text." },
        { "name": "mode",               "type": "str",                         "required": true,  "description": "Interaction mode (input, clickable, select, hover, drag, locate)." },
        { "name": "search_texts",       "type": "list[str]",                   "required": true,  "description": "Quoted target texts extracted from step." },
        { "name": "target_field",       "type": "str | null",                  "required": true,  "description": "Optional target field identifier." },
        { "name": "is_blind",           "type": "bool",                        "required": true,  "description": "No visible text in step." },
        { "name": "learned_elements",   "type": "dict",                        "required": true,  "description": "Semantic cache: {(mode, texts, field, ctx) → {name, tag}}." },
        { "name": "last_xpath",         "type": "str | null",                  "required": true,  "description": "Previous element xpath for context reuse." },
        { "name": "explain",            "type": "bool",                        "default": false,  "description": "Enable per-channel scoring breakdown." },
        { "name": "contextual_hint",    "type": "ContextualHint | null",       "default": null,   "description": "Parsed NEAR/ON HEADER/ON FOOTER/INSIDE qualifier." },
        { "name": "anchor_rect",        "type": "dict | null",                 "default": null,   "description": "Anchor element bounding rect for NEAR scoring." },
        { "name": "container_elements", "type": "list[dict] | null",           "default": null,   "description": "Pre-filtered container subtree for INSIDE scoring." },
        { "name": "viewport_height",    "type": "int",                         "default": 0,      "description": "Viewport height for HEADER/FOOTER region detection." }
      ]
    },
    "publicMethods": [
      {
        "name": "score_all",
        "signature": "(els: list[dict]) -> list[dict]",
        "description": "Pre-processes, scores, and sorts elements descending by score. Adds 'score' (int) key to each element. When explain=True, also adds '_explain' dict with per-channel breakdown."
      }
    ]
  },

  "backwardCompatAPI": {
    "function": "score_elements",
    "signature": "(els, step, mode, search_texts, target_field, is_blind, learned_elements, last_xpath, explain=False, contextual_hint=None, anchor_rect=None, container_elements=None, viewport_height=0) -> list[dict]",
    "description": "Module-level function that delegates to DOMScorer.score_all(). Backward-compatible entry point."
  },

  "elementSnapshotShape": {
    "source": "SNAPSHOT_JS in pkg/dom (snapshot JS)",
    "collectionMethod": "document.createTreeWalker() over the main document, then over every entered iframe, evaluated in that frame: every frame of the page that is displayed (more than 2x2 px) and answers within 2 seconds, same-origin or cross-origin, including frames in open shadow roots. A frame is placed from the document that holds it — Chromium's DOM.getFrameOwner on the parent frame's session, BiDi's iframe.contentWindow, which serializes to the frame's context — since a cross-origin frame's own window.frameElement is null. Chromium's out-of-process (cross-site) frames are reached through Target.setAutoAttach sessions. An element's rect is moved onto the top-level viewport (the frame's offset added: iframe box, border and padding, at every level), so proximity is measured and clicks land on one viewport. Element IDs are drawn per frame from a random base a million apart, so they do not collide across frames; XPaths do, and are compared only within one frame.",
    "pruneSet": ["SCRIPT", "STYLE", "NOSCRIPT", "SVG", "TEMPLATE", "META", "PATH", "G", "BR", "HR"],
    "visibilityChecks": [
      "element.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })",
      "Fallback: offsetWidth > 0 && offsetHeight > 0",
      "Exception: hidden input[type='file|checkbox|radio'] are KEPT",
      "Exception: a <select> below opacity 0.01 is KEPT and counted visible: it is the native control laid transparent over a styled facade, and what a click on that facade reaches"
    ],
    "fields": [
      { "key": "id",                  "type": "int",      "description": "Stable runtime ID (manulIdCounter per page load)." },
      { "key": "name",                "type": "string",   "description": "Display name (max 150 chars). Format: 'context -> core_name [METADATA]'." },
      { "key": "xpath",               "type": "string",   "description": "XPath expression or //*[@id='...']." },
      { "key": "is_select",           "type": "boolean",  "description": "True for <select> elements." },
      { "key": "is_shadow",           "type": "boolean",  "description": "True if inside Shadow DOM." },
      { "key": "is_contenteditable",  "type": "boolean",  "description": "True if element.isContentEditable." },
      { "key": "class_name",          "type": "string",   "description": "className or class attribute (safe for SVG)." },
      { "key": "tag_name",            "type": "string",   "description": "Lowercased HTML tag name." },
      { "key": "input_type",          "type": "string",   "description": "@type for <input>, lowercased." },
      { "key": "data_qa",             "type": "string",   "description": "data-qa or data-testid attribute." },
      { "key": "html_id",             "type": "string",   "description": "id attribute, or 'for' attribute on <label> elements." },
      { "key": "icon_classes",        "type": "string",   "description": "Space-separated icon class names from <i>, <svg>, [class*='icon'] inside the element, leaving out those inside a link or button of their own: they name that control, not the container around it." },
      { "key": "accessible_name",     "type": "string",   "description": "aria-label, the text of every <label for> the field has, title or alt; for a link or button with none of those, the alt text of the images inside it. A text signal like the visible text, so <a><img alt='Cart'></a> is the 'Cart' link." },
      { "key": "aria_label",          "type": "string",   "description": "aria-label, title, or aria-labelledby resolved text." },
      { "key": "placeholder",         "type": "string",   "description": "placeholder / data-placeholder / aria-placeholder (lowercased)." },
      { "key": "role",                "type": "string",   "description": "ARIA role attribute." },
      { "key": "disabled",            "type": "boolean",  "description": "True if @disabled or .disabled." },
      { "key": "aria_disabled",       "type": "string",   "description": "aria-disabled value ('true' | 'false' | '')." },
      { "key": "name_attr",           "type": "string",   "description": "HTML name attribute." },
      { "key": "label_for",           "type": "string",   "description": "For @for attribute on <label> elements." },
      { "key": "rect_top",            "type": "int",      "description": "Math.round(boundingClientRect.top)." },
      { "key": "rect_left",           "type": "int",      "description": "Math.round(boundingClientRect.left)." },
      { "key": "rect_bottom",         "type": "int",      "description": "Math.round(boundingClientRect.bottom)." },
      { "key": "rect_right",          "type": "int",      "description": "Math.round(boundingClientRect.right)." },
      { "key": "ancestors",           "type": "string[]", "description": "Recent ancestor tag names (up to 8 levels)." },
      { "key": "frame_index",         "type": "int",      "description": "Index into Page.Frames (0 = main, omitted). Added by the Go snapshot, not by JS. Every in-page call about the element — typing, checking, measuring, highlighting — is evaluated in this frame." },
      { "key": "frame_url",           "type": "string",   "description": "URL of the element's frame, set by the Go snapshot for an element inside an iframe; informational." },
      { "key": "frame_name",          "type": "string",   "description": "Name of the element's frame as the backend reports it (CDP only; BiDi reports none); informational." }
    ],
    "nameSuffixes": [
      { "suffix": "[HIDDEN]",     "meaning": "Intentionally hidden (aria-hidden, off-screen left < -999, visibility hidden)." },
      { "suffix": "[ABOVE]",      "meaning": "Scrolled above viewport (rect.top < -999). Stripped for text matching." },
      { "suffix": "[SHADOW_DOM]", "meaning": "Element lives inside Shadow DOM." }
    ],
    "selectNameFormat": "dropdown [Option A | Option B | ...]",
    "labelDedup": "Skips <label> when linked <input> is visible and NOT type='file'."
  },

  "interactionModes": {
    "detection": "Keyword-based from step text (pkg/dsl :: detect_mode())",
    "modes": [
      { "mode": "input",     "keywords": ["type", "fill", "enter"] },
      { "mode": "clickable", "keywords": ["click", "double", "check", "uncheck"] },
      { "mode": "select",    "keywords": ["select", "choose"] },
      { "mode": "hover",     "keywords": ["hover"] },
      { "mode": "drag",      "keywords": ["drag", "drop"] },
      { "mode": "locate",    "keywords": null, "note": "Fallback when no other mode matched." }
    ]
  },

  "contextualQualifiers": {
    "parser": "pkg/dsl :: parse_contextual_hint()",
    "types": [
      {
        "syntax": "NEAR 'Anchor Text'",
        "scoring": "Euclidean distance from the element's rect center to the nearest point of the anchor's rect (its center when the element is beside it, a point on its edge otherwise), blended with DOM depth affinity. Closer = higher proximity score. Measured to the anchor's middle, a caption <p> spanning the viewport was 600px from the field right under it and gave it no proximity at all.",
        "proximityWeight": 1.5
      },
      {
        "syntax": "ON HEADER",
        "scoring": "Membership in <header>/<nav> ancestry OR element in top 15% of viewport.",
        "proximityWeight": 1.5
      },
      {
        "syntax": "ON FOOTER",
        "scoring": "Membership in <footer> ancestry OR element in bottom 15% of viewport.",
        "proximityWeight": 1.5
      },
      {
        "syntax": "INSIDE 'Container' row with 'Text'",
        "scoring": "Resolve container row first; restrict candidate scoring to that subtree.",
        "proximityWeight": 1.5
      }
    ]
  },

  "explainMode": {
    "trigger": "explain=True on DOMScorer or --explain CLI flag",
    "output": "Each scored element gets '_explain' dict with per-channel raw scores, weighted contributions, and final normalised confidence [0.0, 1.0]",
    "stderrFormat": "┌─ 🔍 EXPLAIN ... └─ ✅ Decision block printed to stderr"
  }
}
```
