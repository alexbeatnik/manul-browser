package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/scorer"
)

// errWhatIfReplace signals that the What-If REPL picked a replacement step via
// `!execute`. It travels up from debugPrompt so the caller can abandon the step
// it was about to run and execute rt.whatIfExecuteStep instead. It is a control
// signal, never a failure — runCommands must not record it as a failed step.
var errWhatIfReplace = errors.New("runtime: what-if replacement step selected")

// WhatIfResult is the outcome of evaluating a hypothetical step against the
// current page without touching it.
//
// Score is a 0–10 confidence, distinct from the raw scorer total: it is the
// terminal-facing summary. The extension wire format (explainNextPayload in
// debug.go) reports the raw score instead and is unaffected by this type.
type WhatIfResult struct {
	Step           string
	Score          int
	TargetFound    bool
	TargetElement  string
	Explanation    string
	Risk           string
	Suggestion     string
	HeuristicScore float64
	HeuristicMatch string
	// hasHeuristic distinguishes "scored 0.0" from "never scored" (system steps
	// and parse failures), which the report renders differently.
	hasHeuristic bool
}

// ConfidenceLabel buckets Score for human consumption.
func (r WhatIfResult) ConfidenceLabel() string {
	switch {
	case r.Score >= 8:
		return "HIGH"
	case r.Score >= 5:
		return "MODERATE"
	case r.Score >= 1:
		return "LOW"
	default:
		return "IMPOSSIBLE"
	}
}

// FormatReport renders the multi-line box report shown in the terminal REPL.
func (r WhatIfResult) FormatReport() string {
	var sb strings.Builder
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "    ┌─ 🔮 WHAT-IF ANALYSIS: %q\n", r.Step)
	fmt.Fprintf(&sb, "    │  Confidence: %d/10 (%s)\n", r.Score, r.ConfidenceLabel())
	if r.hasHeuristic {
		fmt.Fprintf(&sb, "    │  Heuristic Score: %.3f\n", r.HeuristicScore)
	}
	if r.HeuristicMatch != "" {
		fmt.Fprintf(&sb, "    │  Best Heuristic Match: %q\n", r.HeuristicMatch)
	}
	if r.TargetElement != "" {
		fmt.Fprintf(&sb, "    │  Target Element: %s\n", r.TargetElement)
	}
	fmt.Fprintf(&sb, "    │  Explanation: %s\n", r.Explanation)
	if r.Risk != "" {
		fmt.Fprintf(&sb, "    │  Risk: %s\n", r.Risk)
	}
	if r.Suggestion != "" {
		fmt.Fprintf(&sb, "    │  Suggestion: %s\n", r.Suggestion)
	}
	sb.WriteString("    └─ 🔮 END\n")
	return sb.String()
}

// isWhatIfSystemStep reports whether a step completes without resolving an
// element, and so must not be scored IMPOSSIBLE merely because nothing on the
// page matched it.
//
// The set is navigate, wait, wait-for, scroll, press, done, logical step,
// variable assignment and scan. SET is only a system step when it assigns a
// variable; used as a synonym for FILL it resolves an element like any other
// action.
func isWhatIfSystemStep(cmd dsl.Command) bool {
	switch cmd.Type {
	case dsl.CmdNavigate, dsl.CmdWait, dsl.CmdWaitFor, dsl.CmdWaitForResponse,
		dsl.CmdScroll, dsl.CmdPress, dsl.CmdPrint, dsl.CmdScreenshot:
		return true
	case dsl.CmdSet:
		return cmd.SetVar != ""
	}
	return false
}

// evaluateWhatIf scores a hypothetical step against the live page.
//
// Read-only by construction: it parses the step, takes a DOM snapshot, and
// ranks candidates. It never dispatches the action, so no click, fill, or
// navigation can result from calling it. The only visible effect is the
// highlight applied by the caller.
func (rt *Runtime) evaluateWhatIf(ctx context.Context, stepText string) WhatIfResult {
	res := WhatIfResult{
		Step: stepText,
		Risk: "Heuristic-only evaluation — deterministic scoring, no LLM.",
	}

	hunt, err := dsl.Parse(strings.NewReader(stepText))
	if err != nil {
		res.Explanation = fmt.Sprintf("Step does not parse: %v", err)
		return res
	}
	if len(hunt.Commands) == 0 {
		res.Explanation = "Step parsed to no command."
		return res
	}
	cmd := hunt.Commands[0]

	// Decided before the snapshot: a system step needs no DOM, so taking one
	// would be wasted work. Heuristic fields are therefore absent for system
	// steps rather than being reported as scores nothing produced.
	if isWhatIfSystemStep(cmd) {
		res.Score = 8
		res.TargetFound = true
		res.Explanation = fmt.Sprintf("System command %s — does not require element resolution.", cmd.Type)
		return res
	}

	elements, err := rt.loadSnapshot(ctx)
	if err != nil {
		res.Explanation = fmt.Sprintf("Snapshot failed: %v", err)
		return res
	}

	query := cmd.Target
	if query == "" {
		query = cmd.Raw
	}
	mode := string(cmd.InteractionMode)
	if mode == "" {
		mode = string(dsl.ModeNone)
	}

	ranked := scorer.Rank(query, cmd.TypeHint, mode, elements, 5, nil)
	rt.lastExplainData = ranked

	if len(ranked) == 0 {
		res.Explanation = "No matching element found in the DOM snapshot."
		return res
	}

	top := ranked[0]
	total := top.Explain.Score.Total
	res.Score = scoreToConfidence(total)
	res.TargetFound = res.Score > 0
	res.HeuristicScore = total
	res.hasHeuristic = true
	res.TargetElement = top.Element.XPath

	match := top.Element.VisibleText
	if match == "" {
		match = top.Element.Tag
	}
	res.HeuristicMatch = match

	if res.Score == 0 {
		res.Explanation = "No matching element found in the DOM snapshot."
		return res
	}

	res.Explanation = fmt.Sprintf(
		"Heuristic scoring found candidate %q with normalized score %.3f; the step appears viable.",
		match, total)

	// A runner-up close behind the winner means the phrasing is ambiguous, which
	// is exactly when a suggestion is worth printing.
	if len(ranked) > 1 && res.Score < 8 {
		runnerUp := ranked[1]
		runnerText := runnerUp.Element.VisibleText
		if runnerText == "" {
			runnerText = runnerUp.Element.Tag
		}
		res.Suggestion = fmt.Sprintf(
			"Ambiguous — next candidate %q scores %.3f. Quote the target text exactly to disambiguate.",
			runnerText, runnerUp.Explain.Score.Total)
	}

	return res
}

// takeWhatIfReplacement turns an errWhatIfReplace signal into the command the
// REPL chose, with {variables} resolved the same way a scripted step would be.
//
// The second return value means "this was the replacement signal", not "a
// replacement was found": when the chosen text fails to parse it hands back the
// original command so the run continues rather than aborting on a typo. The
// pending step is always cleared, so a later error cannot replay it.
func (rt *Runtime) takeWhatIfReplacement(err error, current dsl.Command) (dsl.Command, bool) {
	if !errors.Is(err, errWhatIfReplace) {
		return dsl.Command{}, false
	}

	raw := rt.whatIfExecuteStep
	rt.whatIfExecuteStep = ""
	if raw == "" {
		return current, true
	}

	resolved := rt.resolveVariables(raw)
	hunt, perr := dsl.Parse(strings.NewReader(resolved))
	if perr != nil || len(hunt.Commands) == 0 {
		rt.logger.Warn("debug: what-if step %q did not parse, keeping the original step: %v", resolved, perr)
		return current, true
	}

	rt.logger.Info("debug: what-if replaced the step with %q", resolved)
	return hunt.Commands[0], true
}

const whatIfHelp = `
    ┌─ 🔮 What-If REPL ────────────────────────────────────────────
    │  Commands:
    │    <any step>   — evaluate a hypothetical step (dry-run)
    │    !history     — show all evaluations from this session
    │    !execute     — accept the last evaluated step & resume
    │    !execute N   — accept evaluation #N from history & resume
    │    !context     — show current page URL & title
    │    !help        — show this help
    │    !quit        — exit REPL without executing anything
    └──────────────────────────────────────────────────────────────
`

// runWhatIfREPL drives the interactive dry-run loop during a debug pause.
// It returns the step chosen via !execute, or "" if the user quit.
//
// It reads from the caller's scanner rather than opening its own, so no input
// buffered by the debug prompt is lost when control passes back and forth.
func (rt *Runtime) runWhatIfREPL(ctx context.Context, sc *bufio.Scanner, currentStep string) string {
	out := os.Stdout
	fmt.Fprint(out, whatIfHelp)
	if currentStep != "" {
		fmt.Fprintf(out, "    ℹ️  Paused before: %q\n\n", currentStep)
	}

	defer func() {
		_ = rt.clearDebugHighlight(ctx)
	}()

	for {
		fmt.Fprint(out, "  🔮 what-if> ")
		_ = out.Sync()

		if !sc.Scan() {
			fmt.Fprintln(out, "    Exiting What-If REPL.")
			return ""
		}
		input := strings.TrimSpace(sc.Text())
		if input == "" {
			continue
		}

		switch {
		case input == "!quit":
			fmt.Fprintln(out, "    Exiting What-If REPL.")
			return ""

		case input == "!help":
			fmt.Fprint(out, whatIfHelp)

		case input == "!context":
			url, err := rt.page.CurrentURL(ctx)
			if err != nil {
				fmt.Fprintln(out, "    ⚠️  Page context lost.")
				continue
			}
			fmt.Fprintf(out, "    URL:   %s\n", url)
			if raw, err := rt.page.EvalJS(ctx, `document.title`); err == nil {
				var title string
				if json.Unmarshal(raw, &title) == nil {
					fmt.Fprintf(out, "    Title: %s\n", title)
				}
			}

		case input == "!history":
			if len(rt.whatIfHistory) == 0 {
				fmt.Fprintln(out, "    (no evaluations yet)")
				continue
			}
			for i, r := range rt.whatIfHistory {
				fmt.Fprintf(out, "    #%d  [%d/10 %s] %s\n", i+1, r.Score, r.ConfidenceLabel(), r.Step)
			}

		case strings.HasPrefix(input, "!execute"):
			chosen, ok := rt.pickWhatIfStep(out, input)
			if !ok {
				continue
			}
			fmt.Fprintf(out, "    ✅ Executing: %q\n", chosen)
			return chosen

		default:
			fmt.Fprintln(out, "    ⏳ Analyzing...")
			result := rt.evaluateWhatIf(ctx, input)
			rt.whatIfHistory = append(rt.whatIfHistory, result)

			_ = rt.clearDebugHighlight(ctx)
			if result.TargetElement != "" {
				_ = rt.debugHighlight(ctx, result.TargetElement)
			}
			fmt.Fprint(out, result.FormatReport())
		}
	}
}

// pickWhatIfStep resolves `!execute` / `!execute N` against the session history.
// It reports ok=false — after explaining why on out — when there is nothing to
// execute.
func (rt *Runtime) pickWhatIfStep(out io.Writer, input string) (string, bool) {
	fields := strings.Fields(input)

	if len(fields) >= 2 {
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			fmt.Fprintf(out, "    ⚠️  %q is not a history index.\n", fields[1])
			return "", false
		}
		if n < 1 || n > len(rt.whatIfHistory) {
			fmt.Fprintf(out, "    ⚠️  Invalid index. History has %d entries.\n", len(rt.whatIfHistory))
			return "", false
		}
		return rt.whatIfHistory[n-1].Step, true
	}

	if len(rt.whatIfHistory) == 0 {
		fmt.Fprintln(out, "    ⚠️  No evaluations in history. Evaluate a step first.")
		return "", false
	}
	return rt.whatIfHistory[len(rt.whatIfHistory)-1].Step, true
}
