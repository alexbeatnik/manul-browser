// Package runtime implements the Manul Browser DSL execution engine.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/browser"
	"github.com/alexbeatnik/manul-browser/core/pkg/config"
	"github.com/alexbeatnik/manul-browser/core/pkg/core"
	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/explain"
	"github.com/alexbeatnik/manul-browser/core/pkg/heuristics"
	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
	"github.com/alexbeatnik/manul-browser/core/pkg/pages"
	"github.com/alexbeatnik/manul-browser/core/pkg/scorer"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

const (
	ThresholdHighConfidence = 0.15 // strong heuristic match
	ThresholdAmbiguous      = 0.03 // minimum for heuristic choice
	ThresholdRunnerUpGap    = 0.02
	ThresholdPass3Total     = 0.12
	ThresholdPass3Proximity = 0.18
	ThresholdPass3Gap       = 0.04
	// ThresholdPass3NearerGap is how much nearer to the label pass 3's winner
	// has to be than the runner-up when their scores are too close to tell
	// apart. Fields of one kind score alike except for proximity, which weighs
	// little, so a form's fields stacked under their captions never cleared
	// the score gap: the one under 'Name:' and the one under 'Country:' below it.
	ThresholdPass3NearerGap = 0.05
)

// Runtime executes Manul Browser DSL hunts against a live browser page.
//
// CONCURRENCY CONTRACT: A Runtime instance is NOT safe for concurrent use.
// Each goroutine executing hunts must own its own Runtime, Page, and
// (typically) ChromeProcess. The DOM snapshot cache, variable store, and
// sticky checkbox state are unguarded by design — sharing a Runtime across
// goroutines will cause data races detectable by `go test -race`.
//
// To run multiple hunts in parallel, construct one Runtime per worker via
// pkg/worker.NewWorker rather than sharing a single Runtime.
type Runtime struct {
	cfg    config.Config
	page   browser.Page
	logger *utils.Logger
	vars   *ScopedVariables
	pages  *pages.Registry

	// sourcePath is the .hunt file path, used for resolving relative mock/data files.
	sourcePath string

	cachedElements       []dom.ElementSnapshot
	stickyCheckboxStates map[string]bool

	// debug state — only populated when cfg.DebugMode is true
	breakLines        map[int]bool             // source line numbers that are breakpoints; empty = pause every step
	breakSteps        map[int]bool             // command indices queued by extension-mode "next" to pause at
	debugContinue     bool                     // when true, skip all future pauses
	lastExplainData   []scorer.RankedCandidate // cached ranking for the "explain" debug command
	pendingDebugPause bool                     // set in runCommands when an action step should pause after resolution
	pendingDebugIdx   int                      // command index for the pending debug pause
	whatIfHistory     []WhatIfResult           // dry-run evaluations made during this debug session
	whatIfExecuteStep string                   // step chosen via the REPL's !execute, consumed by runCommands

	// onStep, when non-nil, is invoked synchronously after each step
	// completes (pass or fail) with that step's ExecutionResult. Set via
	// SetOnStep. Intended for streaming consumers (e.g. the CLI's -jsonl
	// mode) that need per-step progress before the full HuntResult.
	onStep func(explain.ExecutionResult)

	// activeHuntRes is the HuntResult of the in-flight RunHunt call. Loop
	// (REPEAT/WHILE/FOR EACH) and IF bodies route through runCommands with
	// this so their steps are recorded in the report and surfaced via
	// onStep — not silently dropped. Nil outside RunHunt.
	activeHuntRes *explain.HuntResult

	// acted is set by executeCommandOnce once the step has changed the page,
	// after which a failure is not retried (wait.go).
	acted bool

	// Tabs (tabs.go). tabs is every open tab the hunt began on or followed,
	// in that order — what SWITCH TO TAB counts; openerOf the tab each was
	// opened from; seenTabs every tab id already looked at, so a tab is
	// followed once; tabsNoted whether tab 1 has been recorded.
	tabs      []browser.Page
	openerOf  map[browser.Page]browser.Page
	seenTabs  map[string]bool
	tabsNoted bool
}

// New creates a new Runtime bound to the given Config, Page, and Logger.
//
// The returned Runtime is single-goroutine; see the type doc for the
// concurrency contract. For parallel execution, use pkg/worker.
func New(cfg config.Config, page browser.Page, logger *utils.Logger) *Runtime {
	rt := &Runtime{
		cfg:                  cfg,
		page:                 page,
		logger:               logger,
		vars:                 NewScopedVariables(),
		pages:                pages.NewRegistry(""),
		stickyCheckboxStates: make(map[string]bool),
		breakLines:           make(map[int]bool),
	}
	for _, ln := range cfg.BreakLines {
		rt.breakLines[ln] = true
	}
	return rt
}

// ResolveVariable returns the value of a runtime variable, respecting scope
// precedence (Row > Step > Mission > Global > Import).
func (rt *Runtime) ResolveVariable(name string) (string, bool) {
	return rt.vars.Resolve(name)
}

// SetOnStep registers a callback that fires after every step (pass or
// fail) with that step's ExecutionResult. The callback runs on the
// runtime's own goroutine and MUST NOT block — long-running consumers
// should hand the value off to a buffered channel. Pass nil to clear.
//
// Single-goroutine contract: the callback inherits the Runtime's
// concurrency guarantees, so no locking is required when it reads the
// passed ExecutionResult.
func (rt *Runtime) SetOnStep(fn func(explain.ExecutionResult)) {
	rt.onStep = fn
}

// RunHunt executes all commands in hunt against the bound page.
// It returns an explain.HuntResult summarising the execution.
// Commands are grouped by their StepBlock label; each group emits
// BlockStart / BlockPass / BlockFail so the output mirrors the
// per-STEP structure of the .hunt file.
// Optional rowVars (data-driven testing) are applied at LevelRow scope
// so they override mission-level @var: declarations.
func (rt *Runtime) RunHunt(ctx context.Context, hunt *dsl.Hunt, rowVars ...map[string]string) (*explain.HuntResult, error) {
	if hunt == nil {
		return nil, fmt.Errorf("runtime: nil hunt")
	}

	for k, v := range hunt.Vars {
		rt.vars.Set(k, v, LevelMission)
	}
	for k, v := range hunt.ImportVars {
		rt.vars.Set(k, v, LevelImport)
	}

	if len(rowVars) > 0 && rowVars[0] != nil {
		for k, v := range rowVars[0] {
			rt.vars.Set(k, v, LevelRow)
		}
	}

	rt.sourcePath = hunt.SourcePath

	result := &explain.HuntResult{
		HuntFile: hunt.SourcePath,
		Title:    hunt.Title,
		Context:  hunt.Context,
	}
	// Expose the result so loop/IF bodies (executed inside executeCommand)
	// can record their steps through runCommands. Cleared on return.
	rt.activeHuntRes = result
	defer func() { rt.activeHuntRes = nil }()

	start := time.Now()
	passed, failed := 0, 0
	var firstErr error

	// Execute SETUP block before the mission body.
	if len(hunt.SetupCommands) > 0 {
		rt.logger.BlockStart("SETUP")
		p, f, err := rt.runCommands(ctx, hunt.SetupCommands, result, 0)
		passed += p
		failed += f
		if err != nil {
			rt.logger.BlockFail("SETUP")
			firstErr = err
		} else {
			rt.logger.BlockPass("SETUP")
		}
	}

	// Defer TEARDOWN so it always executes, even if setup or mission fails.
	defer func() {
		if len(hunt.TeardownCommands) > 0 {
			rt.logger.BlockStart("TEARDOWN")
			_, _, tdErr := rt.runCommands(ctx, hunt.TeardownCommands, result, 0)
			if tdErr != nil {
				rt.logger.BlockFail("TEARDOWN")
			} else {
				rt.logger.BlockPass("TEARDOWN")
			}
		}
		result.TotalDuration = time.Since(start)
		result.TotalDurationMS = result.TotalDuration.Milliseconds()
		// Derive the summary from the recorded results so that steps nested
		// inside loop/IF bodies (which runCommands appends but whose counters
		// are not threaded back to the outer pass/fail tally) are included.
		resPassed, resFailed := 0, 0
		for _, r := range result.Results {
			if r.Success {
				resPassed++
			} else {
				resFailed++
			}
		}
		result.TotalSteps = resPassed + resFailed
		result.Passed = resPassed
		result.Failed = resFailed
		result.Success = resFailed == 0
	}()

	if firstErr != nil {
		return result, firstErr
	}

	// Group consecutive commands by StepBlock, preserving order.
	type stepGroup struct {
		name     string
		commands []dsl.Command
	}
	var groups []stepGroup
	defaultLabel := hunt.Title
	if defaultLabel == "" {
		defaultLabel = "Mission"
	}
	for _, cmd := range hunt.Commands {
		label := cmd.StepBlock
		if label == "" {
			label = defaultLabel
		}
		if len(groups) == 0 || groups[len(groups)-1].name != label {
			groups = append(groups, stepGroup{name: label})
		}
		groups[len(groups)-1].commands = append(groups[len(groups)-1].commands, cmd)
	}

	var cmdOffset int
	for _, g := range groups {
		rt.logger.BlockStart(g.name)
		p, f, err := rt.runCommands(ctx, g.commands, result, cmdOffset)
		cmdOffset += len(g.commands)
		passed += p
		failed += f
		if err != nil || f > 0 {
			rt.logger.BlockFail(g.name)
			if firstErr == nil {
				firstErr = err
			}
			// Stop at first failed block (mirrors Python behaviour).
			break
		}
		rt.logger.BlockPass(g.name)
	}

	return result, firstErr
}

func isActionCommand(t dsl.CommandType) bool {
	switch t {
	case dsl.CmdClick, dsl.CmdFill, dsl.CmdType, dsl.CmdHover, dsl.CmdCheck,
		dsl.CmdUncheck, dsl.CmdSelect, dsl.CmdDoubleClick, dsl.CmdRightClick, dsl.CmdUploadFile:
		return true
	}
	return false
}

func (rt *Runtime) runCommands(ctx context.Context, commands []dsl.Command, huntRes *explain.HuntResult, offset int) (int, int, error) {
	passed, failed := 0, 0
	for i, cmd := range commands {
		if err := ctx.Err(); err != nil {
			return passed, failed, fmt.Errorf("runtime: context cancelled: %w", err)
		}

		globalIdx := offset + i

		// The What-If REPL can swap the step in flight via !execute. The
		// substitute is run in place of the original, and is not paused on
		// again — the user already decided at this breakpoint.
		replaced := false

	step:
		for {
			rt.pendingDebugPause = false
			if !replaced && rt.cfg.DebugMode && rt.shouldPause(cmd, globalIdx) {
				if isActionCommand(cmd.Type) {
					// Defer debug pause until after element resolution inside executeCommand.
					rt.pendingDebugPause = true
					rt.pendingDebugIdx = globalIdx
				} else {
					if dbgErr := rt.debugPrompt(ctx, cmd, globalIdx); dbgErr != nil {
						if next, ok := rt.takeWhatIfReplacement(dbgErr, cmd); ok {
							cmd, replaced = next, true
							continue step
						}
						return passed, failed, dbgErr
					}
				}
			}

			rt.logger.ActionStart(cmd.Raw)
			stepStart := time.Now()
			stepResult, err := rt.executeCommand(ctx, cmd)
			durMs := time.Since(stepStart).Milliseconds()

			// Checked before the step is recorded: an abandoned step must not
			// reach the report, the onStep stream, or the pass/fail tally.
			if next, ok := rt.takeWhatIfReplacement(err, cmd); ok {
				cmd, replaced = next, true
				continue step
			}
			stepResult.StepIndex = i
			stepResult.StepBlock = cmd.StepBlock
			rt.captureStepScreenshot(ctx, cmd, &stepResult, err)

			if huntRes != nil {
				huntRes.Results = append(huntRes.Results, stepResult)
			}
			if rt.onStep != nil {
				rt.onStep(stepResult)
			}
			if err != nil {
				failed++
				rt.logger.ActionFail(err)
				rt.logger.Error("step failed (%s): %v", cmd.Raw, err)
				return passed, failed, err
			}
			rt.logger.ActionPass(float64(durMs) / 1000)
			passed++
			break
		}
	}
	return passed, failed, nil
}

// StepResult is the result of a single DSL step run via RunStep.
type StepResult struct {
	// Command is the raw DSL text that was executed.
	Command string `json:"command"`
	// Success is true when the command succeeded.
	Success bool `json:"success"`
	// Error is the error message if Success is false.
	Error string `json:"error,omitempty"`
}

// RunStep executes a single raw DSL command string and returns its result.
func (rt *Runtime) RunStep(ctx context.Context, rawStep string) (*StepResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Parse the single-line command.
	hunt, err := dsl.Parse(strings.NewReader(rawStep))
	if err != nil {
		return &StepResult{Command: rawStep, Error: err.Error()}, err
	}
	if len(hunt.Commands) == 0 {
		return &StepResult{Command: rawStep, Success: true}, nil
	}
	stepResult, execErr := rt.executeCommand(ctx, hunt.Commands[0])
	return &StepResult{
		Command: rawStep,
		Success: execErr == nil,
		Error:   stepResult.Error,
	}, execErr
}

// RunCommand executes a single already-parsed DSL command and returns the
// full structured ExecutionResult — including the targeting decision chain
// (ranked candidates, winner score) and any extracted ActionValue.
//
// Unlike RunStep, which collapses the result into a thin pass/fail record,
// RunCommand preserves everything callers need to build their own compact
// views. It is the primitive that pkg/agent is built on. The command runs
// on the runtime's own goroutine and obeys the single-goroutine contract.
func (rt *Runtime) RunCommand(ctx context.Context, cmd dsl.Command) (explain.ExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return explain.ExecutionResult{}, err
	}
	return rt.executeCommand(ctx, cmd)
}

func (rt *Runtime) resolveAnchor(ctx context.Context, label string, elements []dom.ElementSnapshot) (*scorer.AnchorContext, error) {
	if label == "" {
		return nil, nil
	}
	winner, err := rt.resolveStructuralAnchor(label, elements)
	if err != nil {
		return nil, err
	}
	return &scorer.AnchorContext{
		Rect:       winner.Rect,
		XPath:      winner.XPath,
		FrameIndex: winner.FrameIndex,
		Words:      scorer.SignificantWords(winner.VisibleText),
	}, nil
}

func (rt *Runtime) resolveStructuralAnchor(label string, elements []dom.ElementSnapshot) (dom.ElementSnapshot, error) {
	if label == "" {
		return dom.ElementSnapshot{}, fmt.Errorf("anchor label is empty")
	}
	label = rt.resolveVariables(label)
	// Anchor resolution uses "none" mode to allow matching any structural element (div, span, etc.)
	ranked := scorer.Rank(label, "", string(dsl.ModeNone), elements, 1, nil)
	if len(ranked) == 0 || ranked[0].Explain.Score.Total < ThresholdAmbiguous {
		return dom.ElementSnapshot{}, fmt.Errorf("near anchor not found: %q", label)
	}
	return ranked[0].Element, nil
}

// executeCommand runs a single DSL command and returns its execution result.
// screenshotSlug turns an optional SCREENSHOT label into a filesystem-safe base
// name (no extension).
func screenshotSlug(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(strings.TrimSuffix(s, ".png"), ".PNG")
	var b strings.Builder
	prevDisallowed := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
			prevDisallowed = false
		default:
			// Collapse a run of disallowed chars into a single '_' (mirrors
			// the [^A-Za-z0-9._-]+ → '_' substitution).
			if !prevDisallowed {
				b.WriteRune('_')
				prevDisallowed = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func (rt *Runtime) executeCommandOnce(ctx context.Context, cmd dsl.Command) (res explain.ExecutionResult, err error) {
	start := time.Now()
	res = explain.ExecutionResult{
		Step:            cmd.Raw,
		CommandType:     string(cmd.Type),
		ActionPerformed: strings.ToLower(string(cmd.Type)),
	}
	rt.followTabs(ctx)
	defer func() {
		if pageURL, urlErr := rt.page.CurrentURL(ctx); urlErr == nil {
			res.PageURL = pageURL
		}
		res.Duration = time.Since(start)
		res.DurationMS = res.Duration.Milliseconds()
	}()

	switch cmd.Type {
	case dsl.CmdNavigate:
		url := rt.resolveVariables(cmd.URL)
		res.ActionValue = url
		err = rt.navigate(ctx, url)
		if err == nil {
			rt.invalidateSnapshot()
			if rt.cfg.AutoAnnotate {
				rt.autoAnnotateNavigate(ctx, url)
			}
		}

	case dsl.CmdOpenApp:
		// The desktop/Electron window is already attached at launch (the page
		// was selected via FirstPage/PageMatching). Treat OPEN APP as a
		// readiness checkpoint on the current window: ensure it is loaded and
		// report it — OPEN APP is a checkpoint, not a launch.
		if err = rt.page.WaitForLoad(ctx); err != nil {
			err = fmt.Errorf("open app: %w", err)
			break
		}
		rt.invalidateSnapshot()
		appURL, _ := rt.page.CurrentURL(ctx)
		res.ActionValue = appURL
		if appURL == "" {
			appURL = "(no URL)"
		}
		rt.logger.ActionDetail("📦", "Attached to app window: %s", appURL)

	case dsl.CmdWait:
		seconds := cmd.WaitSeconds
		if cmd.WaitExpr != "" {
			// What stood for the number was not one at parse time. A variable
			// has a value by now; anything else is a typo, and used to be a
			// wait of no time at all that passed.
			resolved := strings.TrimSpace(rt.resolveVariables(cmd.WaitExpr))
			n, perr := strconv.ParseFloat(resolved, 64)
			if perr != nil || n < 0 {
				err = fmt.Errorf("WAIT: %q is not a number of seconds", resolved)
				break
			}
			seconds = n
		}
		err = rt.page.Wait(ctx, time.Duration(seconds*float64(time.Second)))
		// Waiting is how a hunt lets the page change. What was read before
		// the wait says nothing about the page after it.
		rt.invalidateSnapshot()

	case dsl.CmdWaitFor:
		err = rt.waitForElement(ctx, cmd)

	case dsl.CmdWaitForSelector:
		res.ActionValue = cmd.Selector
		err = rt.waitForSelector(ctx, cmd.Selector)

	case dsl.CmdHighlight:
		err = rt.highlightTarget(ctx, cmd)

	case dsl.CmdFullScan:
		var report string
		report, err = rt.fullScan(ctx)
		if err == nil {
			res.ActionValue = report
			rt.logger.Info("%s", report)
		}

	case dsl.CmdScanPage:
		var draft string
		draft, err = rt.scanPageDraft(ctx, cmd.ScanOutput)
		if err == nil {
			res.ActionValue = draft
			rt.logger.Info("%s", draft)
		}

	case dsl.CmdPress:
		// PRESS <key> dispatches a keyboard event to whatever element
		// currently has focus. Typical use: FILL '<field>' '<text>'
		// followed by PRESS Enter to submit a form. "PRESS Key ON
		// '<target>'" focuses the target first, and a chord such as
		// Control+A holds its modifiers around the key.
		chord := strings.TrimSpace(cmd.PressKey)
		if chord == "" {
			err = fmt.Errorf("PRESS: missing key name")
			break
		}
		res.ActionValue = chord
		if target := strings.TrimSpace(rt.resolveVariables(cmd.PressTarget)); target != "" {
			res.TargetQuery = target
			if focusErr := rt.focusTarget(ctx, target); focusErr != nil {
				err = fmt.Errorf("PRESS %s ON %q: %w", chord, target, focusErr)
				res.FailureReason = explain.ReasonNotFound
				break
			}
		}
		key, modifiers := splitKeyChord(chord)
		rt.armNavigationWatch(ctx)
		rt.acted = true
		if dispatchErr := rt.page.DispatchKey(ctx, key, modifiers); dispatchErr != nil {
			err = fmt.Errorf("PRESS %s: %w", chord, dispatchErr)
			break
		}
		// Enter in a field submits its form. The results page came in under
		// the steps that followed, still loading, and a CLICK measured a link
		// that the page's images then pushed out from under the pointer.
		if waitErr := rt.page.Wait(ctx, 300*time.Millisecond); waitErr != nil {
			err = waitErr
			break
		}
		rt.invalidateSnapshot()
		rt.settleNavigation(ctx)

	case dsl.CmdPrint:
		text := rt.resolveVariables(cmd.PrintText)
		res.ActionValue = text
		rt.logger.ActionDetail("📢", "PRINT: %s", text)

	case dsl.CmdScreenshot:
		// Capture a full-page PNG on demand into screenshots/<name>.png under
		// the CWD (auto-named when no label).
		name := screenshotSlug(rt.resolveVariables(cmd.ScreenshotName))
		if name == "" {
			name = fmt.Sprintf("screenshot_%d", time.Now().UnixMilli())
		}
		var data []byte
		if data, err = rt.page.Screenshot(ctx); err != nil {
			err = fmt.Errorf("screenshot: %w", err)
			break
		}
		if err = os.MkdirAll("screenshots", 0o755); err != nil {
			err = fmt.Errorf("screenshot dir: %w", err)
			break
		}
		outPath := filepath.Join("screenshots", name+".png")
		if err = os.WriteFile(outPath, data, 0o644); err != nil {
			err = fmt.Errorf("screenshot write: %w", err)
			break
		}
		res.ActionValue = outPath
		rt.logger.ActionDetail("📸", "SCREENSHOT: %s", outPath)

	case dsl.CmdWaitForResponse:
		pattern := rt.resolveVariables(cmd.WaitResponseURL)
		res.ActionValue = pattern
		// nav_timeout is the one the config contract gives a response to
		// arrive in; a Runtime built without it keeps the step timeout.
		limit := rt.cfg.NavTimeout
		if limit <= 0 {
			limit = rt.cfg.DefaultTimeout
		}
		err = rt.page.WaitForResponse(ctx, pattern, limit)

	case dsl.CmdCallGo:
		res.ActionValue, res.ProbeMetadata, err = rt.executeCallGo(ctx, cmd)

	case dsl.CmdSwitchTab:
		res.ActionValue, err = rt.switchTab(ctx, cmd)
		if err == nil {
			rt.logger.ActionDetail("🗂️", "Switched to %s", res.ActionValue)
		}

	case dsl.CmdMock:
		err = rt.handleMock(ctx, cmd)
		if err == nil {
			res.ActionValue = fmt.Sprintf("%s %s → %s", cmd.MockMethod, cmd.MockPattern, cmd.MockFile)
		}

	case dsl.CmdPause:
		// Force an interactive debug prompt even if --debug is not set globally.
		// Contract §4.1: PAUSE only produces pauses in terminal (--debug) mode;
		// in pipe mode the extension does not respond, which would deadlock.
		if !isTTY() {
			rt.logger.Warn("PAUSE ignored in non-TTY mode")
			break
		}
		rt.logger.Info("PAUSE command encountered")
		err = rt.debugPrompt(ctx, cmd, -1)

	case dsl.CmdDebugVars:
		vars := rt.vars.String()
		rt.logger.Info("%s", vars)
		res.ActionValue = vars

	case dsl.CmdSet:
		if cmd.SetVar != "" {
			val := rt.resolveVariables(cmd.SetValue)
			res.ActionValue = val
			rt.vars.Set(cmd.SetVar, val, LevelRow)
			break
		}
		fallthrough

	case dsl.CmdClick, dsl.CmdFill, dsl.CmdType, dsl.CmdHover, dsl.CmdCheck, dsl.CmdUncheck, dsl.CmdSelect, dsl.CmdDoubleClick, dsl.CmdRightClick, dsl.CmdUploadFile:
		handled, actionValue, metadata, customErr := rt.tryExecuteCustomControl(ctx, cmd)
		if handled {
			rt.acted = true
			res.ActionValue = actionValue
			res.ProbeMetadata = metadata
			if customErr != nil {
				err = customErr
			} else {
				rt.invalidateSnapshot()
			}
			break
		}

		// Target resolution needed for interaction
		res.TargetRequired = true
		elements, errSnapshot := rt.loadSnapshot(ctx)
		if errSnapshot != nil {
			err = errSnapshot
			break
		}
		res.CandidatesConsidered = len(elements)

		// Figure out interaction mode
		mode := dsl.ModeNone
		if cmd.Type == dsl.CmdFill || cmd.Type == dsl.CmdSet || cmd.Type == dsl.CmdType {
			mode = dsl.ModeInput
		} else if cmd.Type == dsl.CmdClick {
			mode = dsl.ModeClickable
		} else if cmd.Type == dsl.CmdCheck || cmd.Type == dsl.CmdUncheck {
			mode = dsl.ModeCheckbox
		} else if cmd.Type == dsl.CmdSelect {
			mode = dsl.ModeSelect
		} else if cmd.Type == dsl.CmdUploadFile {
			mode = dsl.ModeClickable
		}

		targetPath := rt.resolveVariables(cmd.Target)
		if cmd.Type == dsl.CmdSet {
			targetPath = rt.resolveVariables(cmd.SetVar)
		}
		res.TargetQuery = targetPath
		res.TypeHint = cmd.TypeHint

		if (cmd.Type == dsl.CmdFill || cmd.Type == dsl.CmdType || cmd.Type == dsl.CmdSet) && isShadowLikeQuery(targetPath) {
			val := rt.resolveVariables(cmd.Value)
			if cmd.Type == dsl.CmdSet && cmd.SetValue != "" {
				val = rt.resolveVariables(cmd.SetValue)
			}
			handled, typeErr := rt.trySetShadowInputValue(ctx, targetPath, val)
			rt.acted = rt.acted || handled || typeErr != nil
			if typeErr != nil {
				err = typeErr
				break
			}
			if handled {
				res.ActionValue = val
				res.ProbeMetadata = map[string]any{"resolution_strategy": "shadow-input-direct"}
				rt.invalidateSnapshot()
				if waitErr := rt.page.Wait(ctx, 200*time.Millisecond); waitErr != nil {
					err = waitErr
				}
				break
			}
		}

		if cmd.Type == dsl.CmdClick && isDropdownLikeQuery(targetPath) {
			handled, clickErr := rt.tryClickDropdownTriggerByLabel(ctx, targetPath)
			rt.acted = rt.acted || handled || clickErr != nil
			if clickErr != nil {
				err = clickErr
				break
			}
			if handled {
				res.ActionValue = targetPath
				res.ProbeMetadata = map[string]any{"resolution_strategy": "dropdown-trigger-direct"}
				rt.invalidateSnapshot()
				if waitErr := rt.page.Wait(ctx, 300*time.Millisecond); waitErr != nil {
					err = waitErr
				}
				break
			}
		}

		if cmd.Type == dsl.CmdClick && isLikelyDropdownOptionQuery(targetPath) {
			handled, clickErr := rt.tryClickVisibleDropdownOption(ctx, targetPath)
			rt.acted = rt.acted || handled || clickErr != nil
			if clickErr != nil {
				err = clickErr
				break
			}
			if handled {
				res.ActionValue = targetPath
				res.ProbeMetadata = map[string]any{"resolution_strategy": "dropdown-option-direct"}
				rt.invalidateSnapshot()
				if waitErr := rt.page.Wait(ctx, 200*time.Millisecond); waitErr != nil {
					err = waitErr
				}
				break
			}
		}

		anchor, errAnchor := rt.resolveAnchor(ctx, cmd.NearAnchor, elements)
		if errAnchor != nil {
			err = errAnchor
			break
		}

		candidateElements, contextualStrategy, contextErr := rt.applyContextualFilters(cmd, elements)
		if contextErr != nil {
			err = contextErr
			break
		}

		// Restrictive modes (input, checkbox, select) need special handling
		// to support "Click/Check X" when X is a label or nearby table cell.
		isRestrictive := mode == dsl.ModeInput || mode == dsl.ModeCheckbox || mode == dsl.ModeSelect

		var ranked []scorer.RankedCandidate
		resolutionStrategy := "standard"
		if isRestrictive && targetPath != "" {
			ranked, resolutionStrategy = resolveRestrictiveCandidates(targetPath, cmd.TypeHint, mode, candidateElements, anchor, rt.logger)
		}

		// Standard resolution if not already handled by restrictive fallback
		if len(ranked) == 0 {
			ranked = scorer.Rank(targetPath, cmd.TypeHint, string(mode), candidateElements, 5, anchor)
			resolutionStrategy = "standard"
		}
		ranked = collapseNestedDuplicateRankedCandidates(ranked)
		if contextualStrategy != "" {
			resolutionStrategy = contextualStrategy + "+" + resolutionStrategy
		}

		if len(ranked) == 0 {
			err = fmt.Errorf("target not found: %q", targetPath)
			res.FailureReason = explain.ReasonNotFound
			break
		}

		best := ranked[0]
		// The ranking always has a first place. Unless the winner was found by
		// its position next to a matching label (pass 3), it has to show some
		// sign of the target itself — otherwise the target is simply not here,
		// and acting on the runner-up of nothing is how a missing 'Delete'
		// button became a click on whichever button came first.
		if !strings.Contains(resolutionStrategy, "pass3") && !scorer.MatchesQuery(targetPath, &best.Element) {
			err = fmt.Errorf("target not found: %q", targetPath)
			res.FailureReason = explain.ReasonNotFound
			appendRankedCandidates(&res, ranked, 5)
			break
		}
		// Pass 2 found one control of the kind asked for that carries the
		// target itself — a checkbox whose class is chkAgree for 'Agree'. Its
		// score is low because the class is all that matches, not because
		// anything else competes, so the confidence floor does not apply.
		if selectionIsAmbiguous(ranked) && !(strings.HasSuffix(resolutionStrategy, "restrictive-pass2") && len(ranked) == 1) {
			runnerUp := 0.0
			if len(ranked) > 1 {
				runnerUp = ranked[1].Explain.Score.Total
			}
			err = fmt.Errorf("target resolution too ambiguous (confidence %.3f, runner-up %.3f)", best.Explain.Score.Total, runnerUp)
			res.FailureReason = explain.ReasonAmbiguous
			// Attach the top candidates so callers can surface "you almost
			// matched X (0.18)" without a follow-up scan — and how they were
			// found, which is the first thing to ask about a refusal.
			appendRankedCandidates(&res, ranked, 5)
			res.ProbeMetadata = map[string]any{
				"resolution_strategy": resolutionStrategy,
				"interaction_mode":    string(mode),
			}
			break
		}
		appendRankedCandidates(&res, ranked, 5)
		res.WinnerXPath = best.Element.XPath
		res.WinnerScore = best.Explain.Score.Total
		res.ProbeMetadata = map[string]any{
			"resolution_strategy": resolutionStrategy,
			"interaction_mode":    string(mode),
		}
		if cmd.NearAnchor != "" {
			res.ProbeMetadata["near_anchor"] = cmd.NearAnchor
		}
		if cmd.OnRegion != "" {
			res.ProbeMetadata["on_region"] = cmd.OnRegion
		}
		if cmd.InsideContainer != "" {
			res.ProbeMetadata["inside_container"] = cmd.InsideContainer
		}
		if cmd.InsideRowText != "" {
			res.ProbeMetadata["inside_row_text"] = cmd.InsideRowText
		}

		// Anti-phantom guard for inputs/selects (soft warning; logging is inside the helper).
		rt.passesAntiPhantomGuard(string(mode), targetPath, best.Element)

		winner := best.Element
		conf := best.Explain.Score.Total
		label := "Context reuse"
		if conf >= ThresholdHighConfidence {
			label = "High confidence match"
		} else if conf >= ThresholdAmbiguous {
			label = "Keyword match"
		}
		rt.logger.HeuristicDetail(conf, fmt.Sprintf("%s — '%s' → %s (ID=%d)", label, targetPath, winner.Name, winner.ID))

		if rt.cfg.ExplainMode {
			rt.logger.Info("  Breakdown: Text=%.2f, Attr=%.2f, Sem=%.2f, Prox=%.2f",
				best.Explain.Score.NormalizedTextMatch,
				best.Explain.Score.LabelMatch+best.Explain.Score.AriaMatch,
				best.Explain.Score.TagSemantics,
				best.Explain.Score.ProximityScore)
		}

		// Visual feedback: flash highlight for every action (matches Python _highlight).
		_ = rt.pageFor(winner).HighlightElement(ctx, winner.ID, winner.XPath, 2000)
		// Ensure the highlight is removed as soon as the interaction finishes,
		// even if the action triggers navigation that destroys the element.
		defer func() {
			_ = rt.pageFor(winner).ClearHighlight(ctx)
		}()

		// Debug mode: persistent magenta highlight + pause before action execution.
		if rt.pendingDebugPause {
			rt.pendingDebugPause = false
			_ = rt.debugHighlight(ctx, winner.XPath)
			if dbgErr := rt.debugPrompt(ctx, cmd, rt.pendingDebugIdx); dbgErr != nil {
				err = dbgErr
				break
			}
			_ = rt.clearDebugHighlight(ctx)
		}

		// Perform action
		switch cmd.Type {
		case dsl.CmdFill, dsl.CmdSet, dsl.CmdType:
			val := rt.resolveVariables(cmd.Value)
			if cmd.Type == dsl.CmdSet && cmd.SetValue != "" {
				// This case should be handled by the specialized CmdSet above,
				// but we keep it for robustness if fallthrough happened.
				val = rt.resolveVariables(cmd.SetValue)
			}
			res.ActionValue = val
			// Setting a value on a heading or a table cell raises nothing, so
			// the step used to pass with the field it meant left untouched.
			if !winner.IsInteractive(string(dsl.ModeInput)) && winner.Tag != "select" {
				// The scorer gives a disabled field nothing, so a field that
				// is still waiting to be enabled lost to any sentence that
				// mentions it, and the step said the field was a <li>.
				if disabled := disabledFieldFor(targetPath, cmd.TypeHint, candidateElements); disabled != nil {
					err = fmt.Errorf("%q is disabled; nothing was typed (WAIT FOR '%s' to be enabled first)", targetPath, targetPath)
					res.FailureReason = explain.ReasonActionFailed
					break
				}
				err = fmt.Errorf("target not found: %q is <%s>, not a field that takes text", targetPath, winner.Tag)
				res.FailureReason = explain.ReasonNotFound
				break
			}
			// A field whose middle is under something else cannot be typed
			// into by a person, and pages check: one that clears such a field
			// on input left it empty while the step passed. Scrolling it into
			// view is usually what uncovers it — a field low in a scrolled
			// box sits under whatever is drawn over the box's edge.
			if winner.Tag == "input" || winner.Tag == "textarea" {
				if _, _, err = rt.uncoveredCenter(ctx, winner, targetPath, "in the middle of the field; nothing was typed"); err != nil {
					break
				}
			}
			rt.acted = true
			err = rt.pageFor(winner).SetInputValue(ctx, winner.ID, winner.XPath, val)
			if err == nil {
				rt.logger.ActionDetail("⌨️", "Typed %q → %q", val, winner.Name)
				if waitErr := rt.page.Wait(ctx, 300*time.Millisecond); waitErr != nil {
					err = waitErr
					break
				}
				rt.invalidateSnapshot()
			}
		case dsl.CmdClick:
			if isDropdownLikeQuery(targetPath) && !isDropdownLikeElement(winner) {
				handled, clickErr := rt.tryClickNearbyDropdownControl(ctx, winner)
				rt.acted = rt.acted || handled || clickErr != nil
				if clickErr != nil {
					err = clickErr
					break
				}
				if handled {
					rt.invalidateSnapshot()
					if waitErr := rt.page.Wait(ctx, 300*time.Millisecond); waitErr != nil {
						err = waitErr
					} else if stickyErr := rt.reconcileStickyCheckboxStates(ctx); stickyErr != nil {
						err = stickyErr
					}
					break
				}
			}
			x, y, e := rt.clickPoint(ctx, winner, targetPath)
			if e != nil {
				err = e
			} else {
				// Perform interaction
				rt.armNavigationWatch(ctx)
				rt.acted = true
				err = rt.page.Click(ctx, x, y)
				if err == nil {
					// A click may trigger navigation or AJAX update.
					if waitErr := rt.page.Wait(ctx, 500*time.Millisecond); waitErr != nil {
						err = waitErr
						break
					}
					rt.invalidateSnapshot()
					rt.settleNavigation(ctx)
					if stickyErr := rt.reconcileStickyCheckboxStates(ctx); stickyErr != nil {
						err = stickyErr
					}
				}
			}
		case dsl.CmdDoubleClick:
			x, y, e := rt.clickPoint(ctx, winner, targetPath)
			if e != nil {
				err = e
			} else {
				rt.acted = true
				err = rt.page.DoubleClick(ctx, x, y)
				err = rt.settleAfterPointer(ctx, err)
			}
		case dsl.CmdRightClick:
			x, y, e := rt.clickPoint(ctx, winner, targetPath)
			if e != nil {
				err = e
			} else {
				rt.acted = true
				err = rt.page.RightClick(ctx, x, y)
				err = rt.settleAfterPointer(ctx, err)
			}
		case dsl.CmdHover:
			x, y, e := rt.pageFor(winner).GetElementCenter(ctx, winner.ID, winner.XPath)
			if e != nil {
				err = fmt.Errorf("center calc: %w", e)
			} else {
				_ = rt.pageFor(winner).ScrollIntoView(ctx, winner.ID, winner.XPath)
				rt.acted = true
				err = rt.page.Hover(ctx, x, y)
				if err == nil {
					if waitErr := rt.page.Wait(ctx, 300*time.Millisecond); waitErr != nil {
						err = waitErr
						break
					}
					rt.invalidateSnapshot()
				}
			}
		case dsl.CmdCheck, dsl.CmdUncheck:
			checked := cmd.Type == dsl.CmdCheck
			if winner, err = rt.captionedBox(ctx, winner, targetPath, elements); err != nil {
				res.FailureReason = explain.ReasonNotFound
				break
			}
			rt.acted = true
			err = rt.pageFor(winner).SetChecked(ctx, winner.ID, winner.XPath, checked)
			if err == nil {
				rt.rememberStickyCheckboxState(targetPath, checked)
				rt.invalidateSnapshot()
				if verifyErr := rt.ensureCheckboxTargetState(ctx, winner, targetPath, checked, ranked); verifyErr != nil {
					err = verifyErr
				}
			}
		case dsl.CmdSelect:
			val := rt.resolveVariables(cmd.Value)
			res.ActionValue = val
			_ = rt.pageFor(winner).ScrollIntoView(ctx, winner.ID, winner.XPath)

			// Detect if it's a native select or custom dropdown
			if winner.Tag == "select" {
				rt.armNavigationWatch(ctx)
				raw, evalErr := rt.pageFor(winner).EvalJS(ctx, pagejs.SelectOption(winner.XPath, val))
				if evalErr != nil {
					err = evalErr
					break
				}
				outcome, decodeErr := decodeSelectResult(raw)
				switch {
				case decodeErr != nil:
					err = fmt.Errorf("select %q: %w", val, decodeErr)
				case !outcome.Matched:
					// The page is the only place that knows which options
					// exist, so it sends them back: a driver that guessed the
					// label wrong can correct itself from this message instead
					// of believing it succeeded.
					err = fmt.Errorf("option %q not found in the dropdown%s", val, formatOptions(outcome.Options))
				default:
					rt.acted = true
					// A sort or filter dropdown often reloads the page on
					// change, exactly as a click on a link would.
					if waitErr := rt.page.Wait(ctx, 300*time.Millisecond); waitErr != nil {
						err = waitErr
						break
					}
					rt.invalidateSnapshot()
					rt.settleNavigation(ctx)
				}
			} else {
				// Custom dropdown: Click then search for the option text.
				// Where to click is measured now, not taken from the snapshot:
				// the scroll just above moved the page, and the rectangle the
				// snapshot recorded is where the dropdown used to be.
				ox, oy, openErr := rt.clickPoint(ctx, winner, targetPath)
				if openErr == nil {
					rt.acted = true
					openErr = rt.page.Click(ctx, ox, oy)
				}
				if openErr != nil {
					err = fmt.Errorf("select %q: open %q: %w", val, targetPath, openErr)
					break
				}
				rt.invalidateSnapshot()
				if waitErr := rt.page.Wait(ctx, 300*time.Millisecond); waitErr != nil {
					err = waitErr
					break
				}

				// Re-probe to find the option that appeared
				elements, probeErr := rt.loadSnapshot(ctx)
				if probeErr != nil {
					err = fmt.Errorf("select %q: %w", val, probeErr)
					break
				}

				// Exclude the container itself and hidden elements from option search
				var candidates []dom.ElementSnapshot
				for _, e := range elements {
					if e.ID != winner.ID && e.IsVisible && e.Tag != "input" {
						candidates = append(candidates, e)
					}
				}

				// Same rule as a native <select>: an option that is not in the
				// list fails the step rather than clicking whatever ranked first.
				rankedOpt := scorer.Rank(val, "", "clickable", candidates, 1, nil)
				if len(rankedOpt) > 0 && scorer.MatchesQuery(val, &rankedOpt[0].Element) {
					opt := rankedOpt[0].Element
					rt.logger.Info("Selected option %q (Tag=%s ID=%d)", val, opt.Tag, opt.ID)
					_ = rt.pageFor(opt).ScrollIntoView(ctx, opt.ID, opt.XPath)
					cx, cy, centerErr := rt.pageFor(opt).GetElementCenter(ctx, opt.ID, opt.XPath)
					if centerErr != nil {
						err = fmt.Errorf("select %q: center calc: %w", val, centerErr)
						break
					}
					err = rt.page.Click(ctx, cx, cy)
					rt.invalidateSnapshot()
				} else {
					err = fmt.Errorf("could not find option %q after opening %q", val, targetPath)
				}
			}
		case dsl.CmdUploadFile:
			filePath := rt.resolveVariables(cmd.UploadFilePath)
			if filePath == "" {
				filePath = rt.resolveVariables(cmd.UploadFile)
			}
			res.ActionValue = filePath
			absPath, pathErr := rt.resolveUploadPath(filePath)
			if pathErr != nil {
				err = pathErr
				break
			}
			rt.acted = true
			err = rt.pageFor(winner).SetFileInput(ctx, winner.ID, winner.XPath, []string{absPath})
			if err == nil {
				rt.invalidateSnapshot()
			}
		}

	case dsl.CmdDrag:
		elements, errSnapshot := rt.loadSnapshot(ctx)
		if errSnapshot != nil {
			err = errSnapshot
			break
		}

		sourcePath := rt.resolveVariables(cmd.DragSource)
		rankedSrc := scorer.Rank(sourcePath, cmd.TypeHint, string(dsl.ModeClickable), elements, 5, nil)
		if len(rankedSrc) == 0 || !scorer.MatchesQuery(sourcePath, &rankedSrc[0].Element) {
			err = fmt.Errorf("drag source not found: %q", sourcePath)
			res.FailureReason = explain.ReasonNotFound
			break
		}
		for _, r := range rankedSrc {
			res.RankedCandidates = append(res.RankedCandidates, r.Explain)
		}
		srcEl := rankedSrc[0].Element

		dropPath := rt.resolveVariables(cmd.DragTarget)
		rankedDest := scorer.Rank(dropPath, "", string(dsl.ModeClickable), elements, 5, nil)
		if len(rankedDest) == 0 || !scorer.MatchesQuery(dropPath, &rankedDest[0].Element) {
			err = fmt.Errorf("drag destination not found: %q", dropPath)
			res.FailureReason = explain.ReasonNotFound
			break
		}
		for _, r := range rankedDest {
			res.RankedCandidates = append(res.RankedCandidates, r.Explain)
		}
		destEl := rankedDest[0].Element

		// Both centres in one measurement. Measuring them separately scrolls
		// twice, and the first result is stale by the time the second returns.
		x1, y1, x2, y2, errCenters := rt.pageFor(srcEl).GetDragCenters(ctx, srcEl.ID, srcEl.XPath, destEl.ID, destEl.XPath)
		if errCenters != nil {
			err = fmt.Errorf("drag centre calc failed: %w", errCenters)
			break
		}

		rt.logger.Info("Target '%s' resolved to element: ID=%d Tag=%s XPath=%s", sourcePath, srcEl.ID, srcEl.Tag, srcEl.XPath)
		rt.logger.Info("Target '%s' resolved to element: ID=%d Tag=%s XPath=%s", dropPath, destEl.ID, destEl.Tag, destEl.XPath)

		rt.acted = true
		err = rt.page.DragAndDrop(ctx, x1, y1, x2, y2)
		if err == nil {
			rt.invalidateSnapshot()
		}

	case dsl.CmdExtract:
		// Use dedicated extraction probe which handles tables/text nodes
		target := rt.resolveVariables(cmd.Target)
		res.TargetRequired = true
		res.TargetQuery = target
		hint := "" // we could extract hint from cmd if needed
		params := []string{target, hint}

		val, errProbe := rt.page.CallProbe(ctx, heuristics.BuildExtractProbe(), params)
		if errProbe != nil {
			err = errProbe
			break
		}

		extracted, fromField := decodeExtractResult(val)
		// What the page itself does not have may be inside one of its
		// iframes; the first frame that answers is the one read.
		if !fromField && (extracted == "" || extracted == "null") {
			for _, f := range rt.enteredFrames(ctx) {
				raw, ierr := rt.page.CallProbeInFrame(ctx, f.Index, heuristics.BuildExtractProbe(), params)
				if ierr != nil {
					continue
				}
				if v, field := decodeExtractResult(raw); field || (v != "" && v != "null") {
					extracted, fromField = v, field
					break
				}
			}
		}

		// An empty field was found and is empty; empty text was not found.
		if !fromField && (extracted == "" || extracted == "null") {
			err = fmt.Errorf("extract target not found or empty: %q", target)
			break
		}
		if fromField {
			res.ProbeMetadata = map[string]any{extractSourceKey: extractSourceField}
		}
		rt.vars.Set(cmd.ExtractVar, extracted, LevelRow)
		res.ActionValue = extracted
		rt.logger.Info("Extracted '%s' into {%s}", extracted, cmd.ExtractVar)

	case dsl.CmdScroll:
		containerID := ""
		if cmd.ScrollContainer != "" {
			res.TargetRequired = true
			container := rt.resolveVariables(cmd.ScrollContainer)
			res.TargetQuery = container
			if isGenericListContainer(container) {
				containerID = string(core.ScrollStrategyGenericList)
				res.ProbeMetadata = map[string]any{"scroll_strategy": "dropdown-list"}
			} else {
				elements, errSnapshot := rt.loadSnapshot(ctx)
				if errSnapshot != nil {
					err = errSnapshot
					break
				}
				res.CandidatesConsidered = len(elements)
				ranked := scorer.Rank(container, "", "none", elements, 1, nil)
				// The ranking always has a first place. A container that is
				// not on the page used to scroll whatever came out on top —
				// or, with nothing ranked, the page itself — and pass.
				if len(ranked) == 0 || ranked[0].Explain.Score.Total < ThresholdAmbiguous ||
					!scorer.MatchesQuery(container, &ranked[0].Element) {
					appendRankedCandidates(&res, ranked, 1)
					err = fmt.Errorf("scroll container not found: %q", container)
					res.FailureReason = explain.ReasonNotFound
					break
				}
				appendRankedCandidates(&res, ranked, 1)
				res.WinnerXPath = ranked[0].Element.XPath
				res.WinnerScore = ranked[0].Explain.Score.Total
				containerID = ranked[0].Element.XPath
			}
		}

		rt.acted = true
		if scroller, ok := rt.page.(interface {
			ScrollPage(context.Context, string, string) error
		}); ok {
			err = scroller.ScrollPage(ctx, cmd.ScrollDirection, containerID)
		} else {
			// fallback via EvalJS
			amount := 500
			if cmd.ScrollDirection == "up" {
				amount = -500
			}
			js := fmt.Sprintf("window.scrollBy(0, %d)", amount)
			if containerID != "" {
				js = fmt.Sprintf(`
					(document.evaluate("%s", document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue).scrollBy(0, %d)
				`, strings.ReplaceAll(containerID, `"`, `\"`), amount)
			}
			_, err = rt.page.EvalJS(ctx, js)
		}
		if err == nil {
			rt.invalidateSnapshot()
		}

	case dsl.CmdVerifySoft:
		// Non-fatal text presence check. Unlike CmdVerify, failure is downgraded
		// to a warning and the hunt continues. Single-shot (no retry loop):
		// a soft assert is informational, and waiting the full DefaultTimeout
		// on something the caller already expects might not be there would
		// unnecessarily slow hunts. Use WAIT FOR ... or hard VERIFY when you
		// need to actually wait for an element.
		target := rt.resolveVariables(cmd.VerifyText)
		res.TargetQuery = target
		var satisfied bool
		expect := "present"
		if cmd.VerifyState != "" {
			// `VERIFY SOFTLY that 'Submit' is disabled` asks about a state, and
			// used to be answered as if it had asked whether the word "Submit"
			// is on the page. Same single look, at the right thing.
			expect = cmd.VerifyState
			rt.invalidateSnapshot()
			elements, errSnapshot := rt.loadSnapshot(ctx)
			if errSnapshot != nil {
				rt.logger.ActionWarn(fmt.Sprintf("VERIFY SOFTLY skipped: probe error: %v", errSnapshot))
				break
			}
			ranked := rankForVerifyState(target, cmd.VerifyState, elements, rt.logger)
			if verifyRankedCandidateAcceptable(cmd.VerifyState, ranked) {
				satisfied = elementMatchesVerifyState(ranked[0].Element, cmd.VerifyState)
			} else {
				satisfied = missingElementSatisfiesVerifyState(cmd.VerifyState)
			}
		} else {
			raw, errProbe := rt.visibleText(ctx)
			if errProbe != nil {
				rt.logger.ActionWarn(fmt.Sprintf("VERIFY SOFTLY skipped: probe error: %v", errProbe))
				break
			}
			satisfied = strings.Contains(foldSpace(string(raw)), foldSpace(target))
		}
		if cmd.VerifyNegated {
			satisfied = !satisfied
			expect = "NOT " + expect
		}
		if !satisfied {
			msg := fmt.Sprintf("VERIFY SOFTLY failed (non-fatal): '%s' expected %s", target, expect)
			rt.logger.ActionWarn(msg)
			if rt.activeHuntRes != nil {
				rt.activeHuntRes.SoftErrors = append(rt.activeHuntRes.SoftErrors, msg)
			}
		}

	case dsl.CmdVerifyVisual:
		err = rt.verifyVisual(ctx, cmd, &res)

	case dsl.CmdVerify:
		// Lightweight text presence check via dedicated probe with a small retry loop
		target := rt.resolveVariables(cmd.VerifyText)
		res.TargetQuery = target
		var present bool
		var pageText string
		var probed bool
		var lastProbeErr error
		deadline := time.Now().Add(rt.cfg.DefaultTimeout)
		if dlat, ok := ctx.Deadline(); ok && dlat.Before(deadline) {
			deadline = dlat
		}

		for {
			raw, errProbe := rt.visibleText(ctx)
			if errProbe == nil {
				probed = true
				pageText = foldSpace(string(raw))
				present = strings.Contains(pageText, foldSpace(target))
			} else {
				lastProbeErr = errProbe
			}
			// Satisfied is what ends the wait, and for a negated VERIFY that
			// is the text being gone. Ending on "present" either way made
			// `is NOT present` sit out the whole timeout whenever it was
			// true, and fail on the first look whenever the text had not
			// finished disappearing.
			if (errProbe == nil && present != cmd.VerifyNegated) || time.Now().After(deadline) {
				break
			}
			if waitErr := rt.page.Wait(ctx, 200*time.Millisecond); waitErr != nil {
				err = waitErr
				break
			}
		}
		if err != nil {
			break
		}
		if !probed {
			// The page text was never read, so "not present" would be a guess —
			// and a guess that passes every negated VERIFY.
			err = fmt.Errorf("verification failed: could not read the page text: %w", lastProbeErr)
			break
		}

		if cmd.VerifyNegated {
			if present {
				err = fmt.Errorf("verification failed: '%s' is present, but expected NOT to be", target)
			}
		} else {
			if !present {
				rt.logger.Error("VERIFY FAILED. pageText sample: %s", truncateRunes(pageText, 500))
				err = fmt.Errorf("verification failed: '%s' is not present", target)
			}
		}

	case dsl.CmdVerifyField:
		// Attribute form: VERIFY '<label>' has value|text|placeholder "<expected>"
		// (parser sets Target/VerifyFieldKind/Value; the state form below uses
		// VerifyText/VerifyState instead).
		if cmd.VerifyFieldKind != "" {
			res.TargetRequired = true
			target := rt.resolveVariables(cmd.Target)
			expected := rt.resolveVariables(cmd.Value)
			res.TargetQuery = target
			verifyDeadline := time.Now().Add(rt.cfg.DefaultTimeout)
			if deadline, ok := ctx.Deadline(); ok && deadline.Before(verifyDeadline) {
				verifyDeadline = deadline
			}
			var ranked []scorer.RankedCandidate
			actual := ""
			matched := false
			found := false
			for {
				rt.invalidateSnapshot()
				elements, errSnapshot := rt.loadSnapshot(ctx)
				if errSnapshot != nil {
					err = errSnapshot
					break
				}
				res.CandidatesConsidered = len(elements)
				if cmd.VerifyFieldKind != "text" {
					// A <label> has neither a value nor a placeholder, but it
					// carries the field's name as its own text and outranked
					// the field it labels — whose value then read as "".
					elements = withoutLabels(elements)
				}
				// Ranked as if nothing were disabled, as the state form below
				// does: a disabled field still has a value to verify.
				ranked = restoreDisabled(scorer.Rank(target, cmd.TypeHint, string(dsl.ModeNone), asIfEnabled(elements), 5, nil), elements)
				byCaption := false
				if cmd.VerifyFieldKind != "text" && len(ranked) > 0 && !isValueControl(ranked[0].Element) {
					// The target is a caption — <p>Number</p><input>, an <h3>
					// over a <select> — and a caption has no value. FILL and
					// SELECT find the field beside it; reading the caption
					// instead reported "" for a field FILL had just filled.
					if field := fieldBesideCaption(target, cmd.TypeHint, elements); field != nil {
						ranked = field
						byCaption = true
					}
				}
				found = len(ranked) > 0 && (byCaption || scorer.MatchesQuery(target, &ranked[0].Element))
				if found {
					winner := ranked[0].Element
					switch cmd.VerifyFieldKind {
					case "value":
						actual = winner.Value
					case "placeholder":
						actual = winner.Placeholder
					default: // "text"
						actual = strings.TrimSpace(winner.VisibleText)
					}
					if actual == expected {
						matched = true
						break
					}
				}
				if time.Now().After(verifyDeadline) {
					break
				}
				if waitErr := rt.page.Wait(ctx, 200*time.Millisecond); waitErr != nil {
					err = waitErr
					break
				}
			}
			if found {
				appendRankedCandidates(&res, ranked, 1)
				res.WinnerXPath = ranked[0].Element.XPath
				res.WinnerScore = ranked[0].Explain.Score.Total
				res.ActionValue = actual
			}
			if err == nil && !matched {
				if !found {
					err = fmt.Errorf("verification failed: target field '%s' not found", target)
				} else {
					err = fmt.Errorf("verification failed: '%s' has %s %q, expected %q", target, cmd.VerifyFieldKind, actual, expected)
				}
			}
			break
		}

		// Full element resolution for state-specific verification.
		res.TargetRequired = true
		target := rt.resolveVariables(cmd.VerifyText)
		res.TargetQuery = target
		verifyDeadline := time.Now().Add(rt.cfg.DefaultTimeout)
		if deadline, ok := ctx.Deadline(); ok && deadline.Before(verifyDeadline) {
			verifyDeadline = deadline
		}
		stateVerified := false
		lastFound := false
		lastStateValue := false
		var lastWinner dom.ElementSnapshot
		var ranked []scorer.RankedCandidate

		for {
			rt.invalidateSnapshot()
			elements, errSnapshot := rt.loadSnapshot(ctx)
			if errSnapshot != nil {
				err = errSnapshot
				break
			}
			res.CandidatesConsidered = len(elements)

			ranked = rankForVerifyState(target, cmd.VerifyState, elements, rt.logger)
			lastFound = verifyRankedCandidateAcceptable(cmd.VerifyState, ranked)
			if lastFound {
				lastWinner = ranked[0].Element
				lastStateValue = elementMatchesVerifyState(lastWinner, cmd.VerifyState)
				if cmd.VerifyState == "checked" || cmd.VerifyState == "unchecked" {
					// The box CHECK would act on, read off the page: one of
					// several in a form, picked by its caption.
					if box, boxErr := rt.captionedBox(ctx, lastWinner, target, elements); boxErr == nil && box.XPath != lastWinner.XPath {
						lastWinner = box
						if checked, ok := rt.checkedState(ctx, box); ok {
							lastStateValue = checked == (cmd.VerifyState == "checked")
						}
					}
				}
				verifySatisfied := lastStateValue
				if cmd.VerifyNegated {
					verifySatisfied = !verifySatisfied
				}
				if verifySatisfied {
					stateVerified = true
					break
				}
			} else {
				verifySatisfied := missingElementSatisfiesVerifyState(cmd.VerifyState)
				if cmd.VerifyNegated {
					verifySatisfied = !verifySatisfied
				}
				if verifySatisfied {
					stateVerified = true
					break
				}
			}

			if time.Now().After(verifyDeadline) {
				break
			}
			if waitErr := rt.page.Wait(ctx, 200*time.Millisecond); waitErr != nil {
				err = waitErr
				break
			}
		}

		if len(ranked) > 0 {
			appendRankedCandidates(&res, ranked, 1)
			res.WinnerXPath = ranked[0].Element.XPath
			res.WinnerScore = ranked[0].Explain.Score.Total
		}
		if err != nil {
			break
		}
		if !stateVerified {
			if !lastFound {
				err = fmt.Errorf("verification failed: target field '%s' not found for state %q", target, cmd.VerifyState)
				break
			}
			err = fmt.Errorf("verification failed: target '%s' expected state %s, actual state %s", target, expectedVerifyStateDescription(cmd.VerifyState, cmd.VerifyNegated), actualVerifyStateDescription(cmd.VerifyState, lastStateValue))
		}

	case dsl.CmdIf:
		var bodyToRun []dsl.Command
		for _, b := range cmd.Branches {
			if b.Kind == "else" {
				bodyToRun = b.Body
				break
			}
			matched, cerr := rt.evaluateCondition(ctx, b.Condition)
			if cerr != nil {
				err = cerr
				break
			}
			if matched {
				bodyToRun = b.Body
				break
			}
		}
		if err == nil && len(bodyToRun) > 0 {
			_, _, err = rt.runCommands(ctx, bodyToRun, rt.activeHuntRes, 0)
		}

	case dsl.CmdRepeat:
		count := cmd.RepeatCount
		if cmd.RepeatCountExpr != "" {
			// Not a number at parse time: a variable that has a value by now,
			// or a typo. Either way not "zero times, and green".
			resolved := strings.TrimSpace(rt.resolveVariables(cmd.RepeatCountExpr))
			n, perr := strconv.Atoi(resolved)
			if perr != nil || n < 0 {
				err = fmt.Errorf("REPEAT: %q is not a number of times", resolved)
				break
			}
			count = n
		}
		for i := 0; i < count; i++ {
			if cmd.RepeatVar != "" {
				rt.vars.Set(cmd.RepeatVar, fmt.Sprintf("%d", i), LevelRow)
			}
			_, _, err = rt.runCommands(ctx, cmd.Body, rt.activeHuntRes, 0)
			if err != nil {
				break
			}
		}

	case dsl.CmdWhile:
		limit := 100
		for i := 0; i < limit; i++ {
			// A WHILE is a polling loop: each pass asks about the page as it
			// is now, not as the previous pass left it in the cache.
			rt.invalidateSnapshot()
			matched, cerr := rt.evaluateCondition(ctx, cmd.WhileCondition)
			if cerr != nil {
				err = cerr
				break
			}
			if !matched {
				break
			}
			_, _, err = rt.runCommands(ctx, cmd.Body, rt.activeHuntRes, 0)
			if err != nil {
				break
			}
			if i == limit-1 {
				rt.logger.Warn("WHILE loop reached limit (100)")
			}
		}

	case dsl.CmdForEach:
		coll, isVar := rt.vars.Resolve(cmd.ForEachCollection)
		if !isVar {
			// Not a variable name, so it is the list itself. That is what a
			// collection declared with @var: has become by now — @var: values
			// are substituted into the line at parse time — and treating
			// "Ada,Grace" as the name of a variable looped zero times, green.
			coll = rt.resolveVariables(cmd.ForEachCollection)
		}
		items := strings.Split(coll, ",")
		for _, val := range items {
			val = strings.TrimSpace(val)
			if val == "" {
				continue
			}
			rt.vars.Set(cmd.ForEachVar, val, LevelRow)
			_, _, err = rt.runCommands(ctx, cmd.Body, rt.activeHuntRes, 0)
			if err != nil {
				break
			}
		}

	case dsl.CmdUnknown:
		// A line no verb matched. "command UNKNOWN not yet implemented" read
		// as a missing feature, and named neither the line nor its number.
		err = fmt.Errorf("line %d is not a command: %q", cmd.LineNum, cmd.Raw)

	default:
		err = fmt.Errorf("runtime: command %q not yet implemented", cmd.Type)
	}

	if err != nil {
		res.Error = err.Error()
		// Classify the failure for machine-readable consumption. Sites that
		// know the precise cause (target not found / ambiguous) set
		// res.FailureReason directly above; only fill in the rest here.
		if res.FailureReason == explain.ReasonNone {
			res.FailureReason = classifyFailure(ctx, cmd, err)
		}
	} else {
		res.Success = true
	}
	return res, err
}

const (
	extractSourceKey   = "extract_source"
	extractSourceField = "field"
)

// ExtractedFromField reports whether an EXTRACT result is the value of a form
// control rather than text found on the page. Only then is an empty value an
// answer: the field is there and holds nothing.
func ExtractedFromField(res explain.ExecutionResult) bool {
	return res.ProbeMetadata[extractSourceKey] == extractSourceField
}

// decodeExtractResult reads what the extraction probe sent back. Text arrives
// as a string — bare, as every string from the page does. A form control
// answers with an object instead, so that its being empty can be told apart
// from nothing having been found.
func decodeExtractResult(raw []byte) (value string, fromField bool) {
	var field struct {
		Field bool   `json:"field"`
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &field) == nil && field.Field {
		return field.Value, true
	}
	return strings.Trim(string(raw), "\""), false // Unquote JSON string if needed
}

// classifyFailure derives a machine-readable FailureReason from a failed
// command's error and context, for callers that branch on failure kind
// without parsing error strings. Used only as a fallback — the targeting
// pipeline sets the precise not_found/ambiguous reasons at their source.
// selectOutcome is what pagejs.SelectOption reports back from the page.
type selectOutcome struct {
	Matched bool     `json:"matched"`
	Value   string   `json:"value,omitempty"`
	Options []string `json:"options,omitempty"`
}

// decodeSelectResult reads that report. The script stringifies it, and a JS
// string comes back from EvalJS as bare, unquoted bytes — so the JSON arrives
// ready to unmarshal. A bare `true`/`false` is accepted as the same answer in
// its shortest form.
func decodeSelectResult(raw []byte) (selectOutcome, error) {
	body := strings.TrimSpace(string(raw))
	if body == "" || body == "null" || body == "undefined" {
		return selectOutcome{}, errors.New("the page reported no result")
	}
	var matched bool
	if err := json.Unmarshal([]byte(body), &matched); err == nil {
		return selectOutcome{Matched: matched}, nil
	}
	var out selectOutcome
	if err := json.Unmarshal([]byte(body), &out); err == nil {
		return out, nil
	}
	// Double-encoded: a backend that quoted the string result for us.
	var inner string
	if err := json.Unmarshal([]byte(body), &inner); err == nil {
		if err := json.Unmarshal([]byte(inner), &out); err == nil {
			return out, nil
		}
	}
	return selectOutcome{}, fmt.Errorf("unreadable result from the page: %s", body)
}

// formatOptions renders the option list for an error message, empty when the
// page sent none.
func formatOptions(options []string) string {
	var kept []string
	for _, o := range options {
		if strings.TrimSpace(o) != "" {
			kept = append(kept, strconv.Quote(o))
		}
	}
	if len(kept) == 0 {
		return ""
	}
	return " (available: " + strings.Join(kept, ", ") + ")"
}

func classifyFailure(ctx context.Context, cmd dsl.Command, err error) explain.FailureReason {
	if ctx.Err() != nil {
		return explain.ReasonTimeout
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "too ambiguous"):
		return explain.ReasonAmbiguous
	case strings.Contains(msg, "not found"):
		return explain.ReasonNotFound
	case strings.Contains(msg, "verification failed"):
		return explain.ReasonVerifyFailed
	case strings.Contains(msg, "context deadline") || strings.Contains(msg, "timeout"):
		return explain.ReasonTimeout
	}
	// Commands that resolved a target but failed during the action itself.
	if cmd.Type == dsl.CmdVerify || cmd.Type == dsl.CmdVerifyField {
		return explain.ReasonVerifyFailed
	}
	return explain.ReasonActionFailed
}

func (rt *Runtime) evaluateCondition(ctx context.Context, cond string) (bool, error) {
	cond = strings.TrimSpace(cond)
	if cond == "" {
		return false, nil
	}
	// Keywords are matched on a lower-cased copy — the DSL is case-insensitive,
	// and the documented form is `'<target>' is NOT present`. Targets and
	// values are still read from cond as written.
	lower := strings.ToLower(cond)
	if lower == "true" {
		return true, nil
	}
	if lower == "false" {
		return false, nil
	}

	// 1. Variable comparisons: {var} == 'val', $var != 'val', {var} contains
	// 'val', or a bare {var} for truthiness. Looked at first: what a variable
	// is compared with is free text, and `{status} == 'exists'` is not a
	// question about an element.
	if strings.HasPrefix(cond, "{") || strings.HasPrefix(cond, "$") {
		resolved := rt.resolveVariables(cond)
		for _, op := range []string{" == ", " != ", " contains "} {
			// The operator is looked for in the condition as written, so a
			// value that happens to contain one cannot be mistaken for it.
			if !strings.Contains(dsl.MaskQuoted(cond), op) {
				continue
			}
			parts := strings.SplitN(resolved, op, 2)
			if len(parts) != 2 {
				continue
			}
			v1 := strings.TrimSpace(parts[0])
			v2 := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
			switch op {
			case " == ":
				return v1 == v2, nil
			case " != ":
				return v1 != v2, nil
			default:
				return strings.Contains(v1, v2), nil
			}
		}
		// Truthy check for {var}
		val := strings.TrimSpace(resolved)
		return val != "" && val != "false" && val != "0" && val != "null", nil
	}

	// The rest are questions about the page. Their keywords are looked for
	// outside the quotes: a label is free text, and 'Item exists already' or
	// 'Gift is present' would otherwise pick the branch.
	keywords := strings.ToLower(dsl.MaskQuoted(cond))
	target := rt.resolveVariables(dsl.FirstQuoted(cond))

	mode := ""
	switch {
	case strings.Contains(keywords, "exist"):
		// 2. (button|link|field|element|checkbox) 'Target' [not] exists
		mode = "clickable"
	case strings.Contains(keywords, "is present") || strings.Contains(keywords, "is not present"):
		// 3. text 'Target' is [not] present
		mode = "none"
	default:
		return false, fmt.Errorf("unknown condition format: %q", cond)
	}
	if strings.TrimSpace(target) == "" {
		return false, fmt.Errorf("condition names no target: %q", cond)
	}
	negated := strings.Contains(keywords, "not exist") || strings.Contains(keywords, "n't exist") ||
		strings.Contains(keywords, "is not present")

	elements, err := rt.loadSnapshot(ctx)
	if err != nil {
		return false, err
	}
	ranked := scorer.Rank(target, "", mode, elements, 1, nil)
	found := len(ranked) > 0 && ranked[0].Explain.Score.Total > 0.2 &&
		scorer.MatchesQuery(target, &ranked[0].Element)
	return found != negated, nil
}

func (rt *Runtime) handleMock(ctx context.Context, cmd dsl.Command) error {
	method := rt.resolveVariables(cmd.MockMethod)
	pattern := rt.resolveVariables(cmd.MockPattern)
	mockFile := rt.resolveVariables(cmd.MockFile)
	if method == "" || pattern == "" || mockFile == "" {
		return fmt.Errorf("MOCK: invalid syntax — expected MOCK <METHOD> \"<pattern>\" with '<file>'")
	}

	// Resolve mock file path: hunt dir → CWD
	huntDir := ""
	if rt.sourcePath != "" && !strings.HasPrefix(rt.sourcePath, "<") {
		// "<stdin>" and "<agent>" name where a script came from, not a file.
		huntDir = filepath.Dir(rt.sourcePath)
	}
	candidates := []string{}
	if huntDir != "" {
		candidates = append(candidates, filepath.Join(huntDir, mockFile))
	}
	if huntDir != "" {
		// The directory above the hunt, for a suite that keeps its mocks
		// beside its hunts folder. Without a hunt path this was the parent of
		// the working directory, which nobody asked to be searched.
		candidates = append(candidates, filepath.Join(huntDir, "..", mockFile))
	}
	candidates = append(candidates, mockFile)

	var resolved string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			resolved = c
			break
		}
	}
	if resolved == "" {
		return fmt.Errorf("MOCK: file not found: %s", mockFile)
	}

	body, err := os.ReadFile(resolved)
	if err != nil {
		return fmt.Errorf("MOCK: failed to read mock file %s: %w", mockFile, err)
	}

	contentType := "text/plain"
	if strings.HasSuffix(resolved, ".json") {
		contentType = "application/json"
	}

	// The page answers from here on, at the network: a navigation, a fetch,
	// an XHR and an image are all the same request to it, and a rule outlives
	// the document it was declared on. This used to be a patch over
	// window.fetch looked up by the exact string the page passed — so a
	// pattern never matched a full URL, nothing but fetch was covered, and
	// the next navigation threw the patch away.
	if err := rt.page.Mock(ctx, browser.MockRule{
		Method:      method,
		Pattern:     pattern,
		Body:        body,
		ContentType: contentType,
	}); err != nil {
		return fmt.Errorf("MOCK: %w", err)
	}
	rt.logger.ActionDetail("🔀", "MOCK %s *%s → %s", method, pattern, mockFile)
	return nil
}

// navigate loads url and waits for the page, for no longer than nav_timeout.
//
// The timeout was declared, documented and never read: a page with one
// resource that never arrives held NAVIGATE for as long as the server cared
// to, and with nothing else bounding a step that could be for ever.
func (rt *Runtime) navigate(ctx context.Context, url string) error {
	limited, cancel := rt.withNavTimeout(ctx)
	defer cancel()

	err := rt.page.Navigate(limited, url)
	if err == nil {
		// Brief pause helps CDP catch up before we check readyState.
		if err = rt.page.Wait(limited, 300*time.Millisecond); err == nil {
			err = rt.page.WaitForLoad(limited)
		}
	}
	if err == nil || limited.Err() == nil || ctx.Err() != nil {
		// Loaded, failed for a reason of its own, or the caller gave up.
		return err
	}

	// Out of time. Whether that is a failure depends on what there is to work
	// with: a document that has been parsed is a page a person would already
	// be using, held up by an image or a tracker that may never come. One
	// that has not is not a page yet.
	probe, cancelProbe := context.WithTimeout(ctx, 2*time.Second)
	defer cancelProbe()
	state, _ := rt.page.EvalJS(probe, pagejs.ReadyState)
	if s := strings.Trim(string(state), `"`); s == "interactive" || s == "complete" {
		rt.logger.Warn("NAVIGATE: %s had not finished loading after %s; its document is ready, so the hunt goes on", url, rt.cfg.NavTimeout)
		return nil
	}
	return fmt.Errorf("navigation timeout: %s did not load within %s", url, rt.cfg.NavTimeout)
}

// waitForLoad waits for the page to finish loading, for no longer than
// nav_timeout.
func (rt *Runtime) waitForLoad(ctx context.Context) error {
	limited, cancel := rt.withNavTimeout(ctx)
	defer cancel()
	return rt.page.WaitForLoad(limited)
}

// foldSpace lowercases s and collapses every run of whitespace, line breaks
// and no-break spaces included, into one space. VERIFY compares text the way
// a person reads it: two spans on one line read "Shopping cart (1)" while the
// page's innerText says "Shopping cart\n(1)".
func foldSpace(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// armNavigationWatch marks the document before an action that may navigate,
// for settleNavigation to look at afterwards.
func (rt *Runtime) armNavigationWatch(ctx context.Context) {
	_, _ = rt.page.EvalJS(ctx, pagejs.ArmNavigationWatch)
}

// settleNavigation waits, no longer than nav_timeout, for a navigation the
// last action set off to land, and for the new document to load.
//
// A SELECT whose change handler reloads the page waited for nothing, and the
// FILL after it typed into the old document: on a slow server it was still
// there, "complete", until the response came and replaced it — empty field,
// green step. An evaluation that fails is a document in transit, so it counts
// as still leaving; one that answers nothing useful (no watch armed) as not.
func (rt *Runtime) settleNavigation(ctx context.Context) {
	limited, cancel := rt.withNavTimeout(ctx)
	defer cancel()
	for {
		raw, err := rt.page.EvalJS(limited, pagejs.NavigationState)
		if err == nil && strings.TrimSpace(string(raw)) != "1" {
			break
		}
		// A page that cannot be evaluated is usually one in transit — or one
		// whose tab the action closed: a sign-in popup's "Authorize" does,
		// and the step waited out nav_timeout for it to come back.
		if t, ok := rt.page.(browser.Tabbed); err != nil && ok && t.TabClosed(limited) {
			return
		}
		select {
		case <-limited.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	_ = rt.page.WaitForLoad(limited)
}

// withNavTimeout bounds ctx by nav_timeout. Zero means no limit, which is
// also what a Runtime built from an empty Config gets.
func (rt *Runtime) withNavTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if rt.cfg.NavTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, rt.cfg.NavTimeout)
}

// resolveUploadPath turns the path of an UPLOAD into an absolute one that
// exists, looking beside the hunt first and then in the working directory.
//
// A browser has to be handed an absolute path. Firefox refuses a relative one;
// Chrome accepts it, and then stops answering — the next step, whatever it is,
// never returns. A file that is not there is reported here for the same
// reason: neither browser says so in a way a hunt author could act on.
func (rt *Runtime) resolveUploadPath(path string) (string, error) {
	candidates := []string{path}
	if rt.sourcePath != "" && !filepath.IsAbs(path) {
		candidates = []string{filepath.Join(filepath.Dir(rt.sourcePath), path), path}
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return filepath.Abs(c)
		}
	}
	return "", fmt.Errorf("UPLOAD: file not found: %s", path)
}

func (rt *Runtime) resolveVariables(s string) string {
	return rt.vars.Interpolate(s)
}

func (rt *Runtime) passesAntiPhantomGuard(mode string, query string, el dom.ElementSnapshot) bool {
	if mode != string(dsl.ModeInput) && mode != "select" {
		return true
	}

	q := strings.ToLower(query)
	words := strings.Fields(q)
	if len(words) == 0 {
		return true
	}

	// Collected signals for this element
	signals := el.AllTextSignals()
	signals = append(signals, el.HTMLId, el.Tag)

	for _, s := range signals {
		s_l := strings.ToLower(s)
		for _, w := range words {
			if len(w) >= 2 && strings.Contains(s_l, w) {
				return true
			}
		}
	}
	rt.logger.ActionDetail("👻", "ANTI-PHANTOM GUARD: heuristic choice %q for target %q has low keyword correlation.", el.Tag, query)
	return false
}

func (rt *Runtime) autoAnnotateNavigate(ctx context.Context, url string) {
	pageName := rt.pages.LookupPageName(url)
	if pageName == "" {
		pageName = url
	}
	rt.logger.ActionDetail("📍", "Auto-Nav: %s", pageName)
}

// defaultControlHint is the type a restrictive mode asks for when the step
// itself named none.
var defaultControlHint = map[dsl.InteractionMode]string{
	dsl.ModeInput:    "field",
	dsl.ModeSelect:   "dropdown",
	dsl.ModeCheckbox: "checkbox",
}

// visibleText is the lower-cased visible text of the page and of every
// entered iframe, which is what a text VERIFY looks for. Only the main
// document's failure is the step's.
func (rt *Runtime) visibleText(ctx context.Context) ([]byte, error) {
	text, err := rt.page.CallProbe(ctx, heuristics.BuildVisibleTextProbe(), nil)
	if err != nil {
		return nil, err
	}
	for _, f := range rt.enteredFrames(ctx) {
		fctx, cancel := context.WithTimeout(ctx, frameTimeout)
		more, err := rt.page.CallProbeInFrame(fctx, f.Index, heuristics.BuildVisibleTextProbe(), nil)
		cancel()
		if err == nil {
			text = append(append(text, ' '), more...)
		}
	}
	return text, nil
}

// pageFor is the page el is addressed through: the page itself for the main
// document, a view of el's own frame for an element inside an <iframe>.
func (rt *Runtime) pageFor(el dom.ElementSnapshot) browser.Page {
	return browser.InFrame(rt.page, el.FrameIndex)
}

// frameTimeout bounds what one iframe may take to answer. A page carries
// iframes the hunt never asked about — ads, trackers, chat widgets — and one
// that hangs must not stall the step.
const frameTimeout = 2 * time.Second

// enteredFrames are the page's iframes the engine looks into, each with where
// its viewport sits on the page's: every one that can be evaluated in and
// placed, same-origin or not. A frame with no size — a tracking pixel, a
// hidden widget — is left out, and so is one that does not answer in time.
func (rt *Runtime) enteredFrames(ctx context.Context) []enteredFrame {
	frames, err := rt.page.Frames(ctx)
	if err != nil || len(frames) < 2 {
		return nil
	}
	var out []enteredFrame
	if placer, ok := rt.page.(browser.FramePlacer); ok {
		pctx, cancel := context.WithTimeout(ctx, frameTimeout)
		placed, err := placer.PlaceFrames(pctx)
		cancel()
		if err == nil {
			for _, f := range frames {
				if p, ok := placed[f.Index]; ok && f.Index != 0 && p.W*p.H >= 4 {
					out = append(out, enteredFrame{FrameRef: f, dx: p.X, dy: p.Y})
				}
			}
			return out
		}
	}
	for _, f := range frames {
		if f.Index == 0 {
			continue
		}
		fctx, cancel := context.WithTimeout(ctx, frameTimeout)
		dx, dy, err := browser.FrameOffset(fctx, rt.page, f.Index)
		cancel()
		if err == nil {
			out = append(out, enteredFrame{FrameRef: f, dx: dx, dy: dy})
		}
	}
	return out
}

type enteredFrame struct {
	browser.FrameRef
	dx, dy float64
}

// frameElements snapshots every entered iframe, each element tagged with its
// frame and its box moved onto the top-level viewport — where proximity is
// measured and clicks land.
func (rt *Runtime) frameElements(ctx context.Context) []dom.ElementSnapshot {
	var out []dom.ElementSnapshot
	for _, f := range rt.enteredFrames(ctx) {
		fctx, cancel := context.WithTimeout(ctx, frameTimeout)
		if _, err := rt.page.EvalJSInFrame(fctx, f.Index, pagejs.SeedFrameIDs); err != nil {
			cancel()
			continue
		}
		raw, err := rt.page.CallProbeInFrame(fctx, f.Index, heuristics.BuildSnapshotProbe(), nil)
		cancel()
		if err != nil {
			continue
		}
		els, err := heuristics.ParseProbeResult(raw)
		if err != nil {
			continue
		}
		for i := range els {
			els[i].FrameIndex = f.Index
			els[i].FrameURL = f.URL
			els[i].FrameName = f.Name
			r := &els[i].Rect
			r.Top, r.Bottom, r.Left, r.Right = r.Top+f.dy, r.Bottom+f.dy, r.Left+f.dx, r.Right+f.dx
		}
		out = append(out, els...)
	}
	return out
}

// isValueControl reports whether el holds a value of its own: a text field,
// a <select>, or an editable element.
func isValueControl(el dom.ElementSnapshot) bool {
	return el.Tag == "select" || el.IsInteractive(string(dsl.ModeInput))
}

// disabledFieldFor returns the field target names when that field is
// disabled — the one FILL would have resolved were it enabled — or nil.
func disabledFieldFor(target, typeHint string, elements []dom.ElementSnapshot) *dom.ElementSnapshot {
	ranked, _ := resolveRestrictiveCandidates(target, typeHint, dsl.ModeInput, asIfEnabled(elements), nil, nil)
	ranked = restoreDisabled(ranked, elements)
	if len(ranked) == 0 || !ranked[0].Element.IsDisabled || ranked[0].Element.Tag == "select" {
		return nil
	}
	el := ranked[0].Element
	el.IsDisabled = false
	if !el.IsInteractive(string(dsl.ModeInput)) {
		return nil
	}
	return &ranked[0].Element
}

// fieldBesideCaption ranks the field a caption names, the way FILL and SELECT
// find it, or returns nil when there is none. Disabled fields count: a
// disabled field still has a value to verify.
func fieldBesideCaption(target, typeHint string, elements []dom.ElementSnapshot) []scorer.RankedCandidate {
	probe := asIfEnabled(elements)
	for _, mode := range []dsl.InteractionMode{dsl.ModeInput, dsl.ModeSelect} {
		ranked, strategy := resolveRestrictiveCandidates(target, typeHint, mode, probe, nil, nil)
		if len(ranked) > 0 && strings.Contains(strategy, "pass3") && isValueControl(ranked[0].Element) {
			return restoreDisabled(ranked, elements)
		}
	}
	return nil
}

func resolveRestrictiveCandidates(targetPath, typeHint string, mode dsl.InteractionMode, elements []dom.ElementSnapshot, anchor *scorer.AnchorContext, logger *utils.Logger) ([]scorer.RankedCandidate, string) {
	// A type hint alone clears the confidence bar, so pass 1 also has to match
	// the target — otherwise the first input on the page wins before the label
	// passes below ever get to look for the right one.
	//
	// And it has to be a control of this mode. A table header 'Qty.' matches
	// its own text perfectly, so it took pass 1, and FILL typed into the <th>
	// instead of the quantity field the label passes would have found under it.
	selfRanked := scorer.Rank(targetPath, typeHint, string(mode), elements, 5, anchor)
	if len(selfRanked) > 0 && selfRanked[0].Explain.Score.Total >= ThresholdHighConfidence &&
		scorer.MatchesQuery(targetPath, &selfRanked[0].Element) &&
		selfRanked[0].Element.IsInteractive(string(mode)) {
		return selfRanked, "restrictive-pass1"
	}

	anchorRanked := scorer.Rank(targetPath, typeHint, string(dsl.ModeNone), elements, 8, anchor)
	if len(anchorRanked) == 0 {
		return nil, "restrictive-pass2"
	}

	bestScore := -1.0
	var bestRanked []scorer.RankedCandidate
	bestStrategy := "restrictive-anchor"

	// Pass 3 looks for a control of this mode beside a label, so it asks for
	// one by type even when the step named none. FILL and TYPE never carry a
	// hint, and without one a field right under its column header scored just
	// short of the bar.
	controlHint := typeHint
	if controlHint == "" {
		controlHint = defaultControlHint[mode]
	}

	for _, anchorCandidate := range anchorRanked {
		// Only something that carries the target text can stand in as its label.
		if !scorer.MatchesQuery(targetPath, &anchorCandidate.Element) {
			continue
		}
		if anchorCandidate.Element.IsInteractive(string(mode)) {
			candidateScore := anchorCandidate.Explain.Score.Total + 0.05
			if candidateScore > bestScore {
				bestScore = candidateScore
				bestRanked = []scorer.RankedCandidate{anchorCandidate}
				bestStrategy = "restrictive-pass2"
			}
			continue
		}

		newAnchor := &scorer.AnchorContext{
			Rect:       anchorCandidate.Element.Rect,
			XPath:      anchorCandidate.Element.XPath,
			FrameIndex: anchorCandidate.Element.FrameIndex,
			Words:      scorer.SignificantWords(anchorCandidate.Element.VisibleText),
		}
		// The text and its control in one element, the way a wrapping <label>
		// holds them: <div><input type=checkbox> Agree to the <a>Terms</a></div>.
		// Nothing labels that checkbox, and measured from the middle of the
		// text it was too far off for the proximity passes below.
		if inside := controlsInside(anchorCandidate.Element, elements, mode); len(inside) == 1 {
			candidateScore := anchorCandidate.Explain.Score.Total + 0.2
			if candidateScore > bestScore {
				bestScore = candidateScore
				bestRanked = scorer.Rank("", controlHint, string(mode), inside, 1, newAnchor)
				bestStrategy = "restrictive-pass3-inside"
			}
			continue
		}
		if mode == dsl.ModeCheckbox {
			rowScoped := checkboxCandidatesInSameRow(anchorCandidate.Element, elements)
			if len(rowScoped) > 0 {
				rowRanked := nearbyControls(scorer.Rank("", controlHint, string(mode), rowScoped, 0, newAnchor))
				if pass3CandidateAcceptable(rowRanked) {
					candidateScore := rowRanked[0].Explain.Score.Total + anchorCandidate.Explain.Score.Total*0.35 + 0.15
					if candidateScore > bestScore {
						bestScore = candidateScore
						bestRanked = rowRanked
						bestStrategy = "restrictive-pass3-row"
					}
					continue
				}
			}
		}
		rankedFallback := nearbyControls(scorer.Rank("", controlHint, string(mode), elements, 0, newAnchor))
		if !pass3CandidateAcceptable(rankedFallback) {
			continue
		}
		candidateScore := rankedFallback[0].Explain.Score.Total + anchorCandidate.Explain.Score.Total*0.25
		if candidateScore > bestScore {
			bestScore = candidateScore
			bestRanked = rankedFallback
			bestStrategy = "restrictive-pass3"
		}
	}

	if len(bestRanked) > 0 {
		if bestStrategy == "restrictive-pass3" && logger != nil {
			logger.Info("Resolved restrictive target %q via multi-anchor nearby control search.", targetPath)
		}
		return bestRanked, bestStrategy
	}

	return anchorRanked, "restrictive-anchor"
}

func (rt *Runtime) applyContextualFilters(cmd dsl.Command, elements []dom.ElementSnapshot) ([]dom.ElementSnapshot, string, error) {
	filtered := elements
	var strategies []string

	if cmd.OnRegion != "" {
		regionFiltered := filterRegionCandidates(cmd.OnRegion, filtered)
		if len(regionFiltered) == 0 {
			return nil, "", fmt.Errorf("no candidates found in region %q", cmd.OnRegion)
		}
		filtered = regionFiltered
		strategies = append(strategies, "on-"+normalizeContextLabel(cmd.OnRegion))
	}

	// Container before row: "INSIDE 'Users' row with 'John'" means the John row
	// of the Users table. Filtering by row first left only that row's cells, among
	// which the table itself can never be, so the container was never found.
	if cmd.InsideContainer != "" {
		containerFiltered, err := rt.resolveContainerDescendants(cmd.InsideContainer, filtered)
		if err != nil {
			return nil, "", err
		}
		filtered = containerFiltered
		strategies = append(strategies, "inside-container")
	}

	if cmd.InsideRowText != "" {
		rowAnchor, err := rt.resolveStructuralAnchor(cmd.InsideRowText, filtered)
		if err != nil {
			return nil, "", fmt.Errorf("inside row anchor not found: %q", cmd.InsideRowText)
		}
		rowFiltered := candidatesInSameRow(rowAnchor, filtered)
		if len(rowFiltered) == 0 {
			return nil, "", fmt.Errorf("no candidates found inside row %q", cmd.InsideRowText)
		}
		filtered = rowFiltered
		strategies = append(strategies, "inside-row")
	}

	return filtered, strings.Join(strategies, "+"), nil
}

// containerCandidates is how many ranked matches INSIDE looks through for one
// that contains something.
const containerCandidates = 5

// resolveContainerDescendants returns the elements inside the container that
// label names. The container is the best-ranked match that has elements inside
// it, not the best match outright: a section's heading names it at least as
// well as the section does, and contains nothing.
func (rt *Runtime) resolveContainerDescendants(label string, elements []dom.ElementSnapshot) ([]dom.ElementSnapshot, error) {
	resolved := rt.resolveVariables(label)
	ranked := scorer.Rank(resolved, "", string(dsl.ModeNone), elements, containerCandidates, nil)
	found := false
	for _, cand := range ranked {
		if cand.Explain.Score.Total < ThresholdAmbiguous {
			break
		}
		found = true
		if inside := descendantsOf(cand.Element, elements); len(inside) > 0 {
			return inside, nil
		}
	}
	if !found {
		return nil, fmt.Errorf("inside container not found: %q", label)
	}
	return nil, fmt.Errorf("no candidates found inside container %q", label)
}

// controlsInside returns the controls of mode that sit inside container.
// For text, that excludes the inputs that take none: IsInteractive counts any
// <input> as a field, and the 'Remove' checkbox in a cart row became the only
// "field" inside a cell and took FILL 'Qty.'.
func controlsInside(container dom.ElementSnapshot, elements []dom.ElementSnapshot, mode dsl.InteractionMode) []dom.ElementSnapshot {
	var out []dom.ElementSnapshot
	for _, element := range elements {
		if mode == dsl.ModeInput && element.Tag == "input" && nonTextInputTypes[strings.ToLower(element.InputType)] {
			continue
		}
		if element.IsInteractive(string(mode)) && element.FrameIndex == container.FrameIndex && scorer.IsXPathAncestor(container.XPath, element.XPath) {
			out = append(out, element)
		}
	}
	return out
}

// nonTextInputTypes are the <input> types that take no typed text.
var nonTextInputTypes = map[string]bool{
	"checkbox": true, "radio": true, "submit": true, "button": true,
	"image": true, "reset": true, "hidden": true, "file": true,
}

func checkboxCandidatesInSameRow(anchor dom.ElementSnapshot, elements []dom.ElementSnapshot) []dom.ElementSnapshot {
	rowPrefix := rowXPathPrefix(anchor.XPath)
	if rowPrefix == "" {
		return nil
	}
	var out []dom.ElementSnapshot
	for _, element := range elements {
		if !element.IsInteractive(string(dsl.ModeCheckbox)) {
			continue
		}
		if element.FrameIndex == anchor.FrameIndex && strings.HasPrefix(element.XPath, rowPrefix+"/") {
			out = append(out, element)
		}
	}
	return out
}

func candidatesInSameRow(anchor dom.ElementSnapshot, elements []dom.ElementSnapshot) []dom.ElementSnapshot {
	rowPrefix := rowXPathPrefix(anchor.XPath)
	if rowPrefix == "" {
		return nil
	}
	var out []dom.ElementSnapshot
	for _, element := range elements {
		if element.FrameIndex == anchor.FrameIndex && strings.HasPrefix(element.XPath, rowPrefix+"/") {
			out = append(out, element)
		}
	}
	return out
}

func descendantsOf(container dom.ElementSnapshot, elements []dom.ElementSnapshot) []dom.ElementSnapshot {
	prefix := strings.TrimRight(container.XPath, "/") + "/"
	var out []dom.ElementSnapshot
	for _, element := range elements {
		if element.XPath == container.XPath {
			continue
		}
		if element.FrameIndex == container.FrameIndex && strings.HasPrefix(element.XPath, prefix) {
			out = append(out, element)
		}
	}
	return out
}

func rowXPathPrefix(xpath string) string {
	parts := strings.Split(strings.Trim(xpath, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	prefix := make([]string, 0, len(parts))
	for _, part := range parts {
		prefix = append(prefix, part)
		if strings.HasPrefix(part, "tr[") {
			return "/" + strings.Join(prefix, "/")
		}
	}
	return ""
}

func filterRegionCandidates(region string, elements []dom.ElementSnapshot) []dom.ElementSnapshot {
	viewportHeight := inferredViewportHeight(elements)
	var out []dom.ElementSnapshot
	for _, element := range elements {
		if elementMatchesRegion(region, element, viewportHeight) {
			out = append(out, element)
		}
	}
	return out
}

func inferredViewportHeight(elements []dom.ElementSnapshot) float64 {
	maxBottom := 0.0
	for _, element := range elements {
		bottom := element.Rect.Top + element.Rect.Height
		if bottom > maxBottom {
			maxBottom = bottom
		}
	}
	if maxBottom <= 0 {
		return 1000
	}
	return maxBottom
}

func elementMatchesRegion(region string, el dom.ElementSnapshot, viewportHeight float64) bool {
	region = normalizeContextLabel(region)
	if viewportHeight <= 0 {
		viewportHeight = 1000
	}
	bottom := el.Rect.Top + el.Rect.Height
	switch region {
	case "header":
		for _, ancestor := range el.Ancestors {
			ancestor = normalizeContextLabel(ancestor)
			if ancestor == "header" || ancestor == "nav" {
				return true
			}
		}
		return el.Rect.Top >= 0 && el.Rect.Top <= viewportHeight*0.15
	case "footer":
		for _, ancestor := range el.Ancestors {
			ancestor = normalizeContextLabel(ancestor)
			if ancestor == "footer" {
				return true
			}
		}
		return bottom >= viewportHeight*0.85
	default:
		return true
	}
}

func normalizeContextLabel(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), "-")
}

func isGenericListContainer(query string) bool {
	lower := strings.ToLower(strings.TrimSpace(query))
	lower = strings.TrimPrefix(lower, "the ")
	return lower == "list" || lower == "dropdown" || lower == "dropdown list" || lower == "listbox"
}

func isDropdownLikeQuery(query string) bool {
	lower := strings.ToLower(query)
	return strings.Contains(lower, "dropdown") || strings.Contains(lower, "combo box") || strings.Contains(lower, "combobox")
}

func isLikelyDropdownOptionQuery(query string) bool {
	lower := strings.ToLower(strings.TrimSpace(query))
	return strings.HasPrefix(lower, "item ") || strings.HasPrefix(lower, "option ")
}

func isShadowLikeQuery(query string) bool {
	return strings.Contains(strings.ToLower(query), "shadow")
}

func isDropdownLikeElement(el dom.ElementSnapshot) bool {
	if el.Tag == "select" || strings.EqualFold(el.Role, "combobox") || strings.EqualFold(el.Role, "listbox") {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{el.Name, el.HTMLId, el.ClassName, el.Placeholder, el.AriaLabel}, " "))
	return strings.Contains(haystack, "dropdown") || strings.Contains(haystack, "combo") || strings.Contains(haystack, "select")
}

func rankForVerifyState(target, state string, elements []dom.ElementSnapshot, logger *utils.Logger) []scorer.RankedCandidate {
	mode := dsl.ModeNone
	typeHint := ""
	switch strings.ToLower(state) {
	case "checked", "unchecked":
		mode = dsl.ModeCheckbox
		typeHint = "checkbox"
	case "selected":
		mode = dsl.ModeSelect
	}
	probe := asIfEnabled(elements)
	if mode != dsl.ModeNone && target != "" {
		ranked, _ := resolveRestrictiveCandidates(target, typeHint, mode, probe, nil, nil)
		if len(ranked) > 0 {
			return restoreDisabled(ranked, elements)
		}
	}
	return restoreDisabled(scorer.Rank(target, "", "none", probe, 1, nil), elements)
}

// asIfEnabled copies elements with the disabled flag cleared, for ranking.
//
// The scorer zeroes a disabled element, which is right when choosing something
// to act on and wrong when reading a state: `VERIFY that 'Submit' is disabled`
// could never find the button it was asking about, and a read-only checkbox
// was invisible to `is checked`. Pair it with restoreDisabled, which puts the
// real flag back on whatever was ranked.
func asIfEnabled(elements []dom.ElementSnapshot) []dom.ElementSnapshot {
	probe := make([]dom.ElementSnapshot, len(elements))
	copy(probe, elements)
	for i := range probe {
		probe[i].IsDisabled = false
	}
	return probe
}

// withoutLabels returns elements minus the <label> ones, as a new slice.
func withoutLabels(elements []dom.ElementSnapshot) []dom.ElementSnapshot {
	kept := make([]dom.ElementSnapshot, 0, len(elements))
	for _, el := range elements {
		if el.Tag != "label" {
			kept = append(kept, el)
		}
	}
	return kept
}

func restoreDisabled(ranked []scorer.RankedCandidate, elements []dom.ElementSnapshot) []scorer.RankedCandidate {
	for i := range ranked {
		c := &ranked[i].Element
		for j := range elements {
			o := &elements[j]
			if o.ID == c.ID && o.XPath == c.XPath && o.FrameIndex == c.FrameIndex &&
				o.Tag == c.Tag && o.VisibleText == c.VisibleText {
				c.IsDisabled = o.IsDisabled
				ranked[i].Explain.IsEnabled = !o.IsDisabled
				break
			}
		}
	}
	return ranked
}

func elementMatchesVerifyState(el dom.ElementSnapshot, state string) bool {
	switch strings.ToLower(state) {
	case "checked":
		return el.IsChecked
	case "unchecked":
		return !el.IsChecked
	case "enabled":
		return !el.IsDisabled
	case "disabled":
		return el.IsDisabled
	case "visible":
		return el.IsVisible && !el.IsHidden
	case "hidden", "disappear":
		return !el.IsVisible || el.IsHidden
	case "selected":
		return el.IsSelected
	default:
		return false
	}
}

func missingElementSatisfiesVerifyState(state string) bool {
	switch strings.ToLower(state) {
	case "hidden", "disappear":
		return true
	default:
		return false
	}
}

func expectedVerifyStateDescription(state string, negated bool) string {
	base := strings.ToLower(strings.TrimSpace(state))
	if base == "" {
		base = "present"
	}
	if negated {
		return "NOT " + base
	}
	return base
}

func actualVerifyStateDescription(state string, matches bool) string {
	base := strings.ToLower(strings.TrimSpace(state))
	if base == "" {
		return "unknown"
	}
	if matches {
		return base
	}
	switch base {
	case "checked":
		return "unchecked"
	case "unchecked":
		return "checked"
	case "enabled":
		return "disabled"
	case "disabled":
		return "enabled"
	case "visible":
		return "hidden"
	case "hidden", "disappear":
		return "visible"
	case "selected":
		return "not selected"
	default:
		return "not " + base
	}
}

func verifyRankedCandidateAcceptable(state string, ranked []scorer.RankedCandidate) bool {
	if len(ranked) == 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "checked", "unchecked", "selected":
		return !selectionIsAmbiguous(ranked)
	default:
		return ranked[0].Explain.Score.Total > 0.3
	}
}

func (rt *Runtime) ensureCheckboxTargetState(ctx context.Context, acted dom.ElementSnapshot, target string, desired bool, initialRanked []scorer.RankedCandidate) error {
	if waitErr := rt.page.Wait(ctx, 150*time.Millisecond); waitErr != nil {
		return waitErr
	}
	// Ask the control that was just acted on before asking the page for one
	// by name. A box with an unbound caption beside it — <input><label> with
	// no for= — is reached through the caption but cannot be found by it, so
	// the by-name check below reported a ticked box as never ticked, and then
	// went on to try other boxes.
	if checked, ok := rt.checkedState(ctx, acted); ok && checked == desired {
		return nil
	}
	rt.invalidateSnapshot()
	matched, err := rt.checkboxTargetHasState(ctx, target, desired)
	if err != nil {
		return err
	}
	if matched {
		return nil
	}

	retryCandidates, err := rt.collectCheckboxRetryCandidates(ctx, target, initialRanked)
	if err != nil {
		return err
	}
	tried := map[string]bool{}
	for _, candidate := range retryCandidates {
		key := candidate.Element.XPath
		if key == "" {
			key = fmt.Sprintf("id:%d", candidate.Element.ID)
		}
		if tried[key] {
			continue
		}
		tried[key] = true

		if err := rt.pageFor(candidate.Element).SetChecked(ctx, candidate.Element.ID, candidate.Element.XPath, desired); err != nil {
			continue
		}
		if waitErr := rt.page.Wait(ctx, 150*time.Millisecond); waitErr != nil {
			return waitErr
		}
		rt.invalidateSnapshot()
		matched, err = rt.checkboxTargetHasState(ctx, target, desired)
		if err != nil {
			return err
		}
		if matched {
			return nil
		}
	}

	return fmt.Errorf("checkbox target %q did not reach checked=%t", target, desired)
}

func (rt *Runtime) checkboxTargetHasState(ctx context.Context, target string, desired bool) (bool, error) {
	elements, err := rt.loadSnapshot(ctx)
	if err != nil {
		return false, err
	}
	ranked := rankForVerifyState(target, "checked", elements, nil)
	if !verifyRankedCandidateAcceptable("checked", ranked) {
		return false, nil
	}
	return ranked[0].Element.IsChecked == desired, nil
}

func (rt *Runtime) collectCheckboxRetryCandidates(ctx context.Context, target string, initialRanked []scorer.RankedCandidate) ([]scorer.RankedCandidate, error) {
	elements, err := rt.loadSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	var candidates []scorer.RankedCandidate
	candidates = append(candidates, initialRanked...)
	// "restrictive-anchor" is that resolver giving up: it hands back whatever
	// ranked highest, matching or not.
	if restrictive, strategy := resolveRestrictiveCandidates(target, "checkbox", dsl.ModeCheckbox, elements, nil, nil); len(restrictive) > 0 && strategy != "restrictive-anchor" {
		candidates = append(candidates, restrictive...)
	}
	// The two plain rankings always return five of something. With the
	// checkbox hint that is the page's first five checkboxes whether or not
	// they have anything to do with the target, and a retry ticked them.
	for _, c := range scorer.Rank(target, "checkbox", string(dsl.ModeCheckbox), elements, 5, nil) {
		if scorer.MatchesQuery(target, &c.Element) {
			candidates = append(candidates, c)
		}
	}
	for _, c := range scorer.Rank(target, "", string(dsl.ModeNone), elements, 5, nil) {
		if scorer.MatchesQuery(target, &c.Element) {
			candidates = append(candidates, c)
		}
	}
	return candidates, nil
}

// clickPoint returns where a click on el should land, once nothing else is
// in the way there.
//
// Something on top of the target is often on its way out — a modal's backdrop
// fading, a toast, a menu closing — so it is given a moment to clear. What is
// still there after that is reported instead of clicked: the click would have
// gone to it, and the step would have passed.
func (rt *Runtime) clickPoint(ctx context.Context, el dom.ElementSnapshot, target string) (x, y float64, err error) {
	return rt.uncoveredCenter(ctx, el, target, "where the click would land; nothing was clicked")
}

// uncoveredCenter scrolls el into view and returns its centre once nothing
// else is on top of it there, failing with "<target> is covered by … <where>"
// when something still is after coverChecks looks.
func (rt *Runtime) uncoveredCenter(ctx context.Context, el dom.ElementSnapshot, target, where string) (x, y float64, err error) {
	for check := 1; ; check++ {
		// Measured again each time round: what was in the way may have been
		// holding the layout where it was.
		x, y, err = rt.pageFor(el).GetElementCenter(ctx, el.ID, el.XPath)
		if err != nil {
			return 0, 0, fmt.Errorf("center calc: %w", err)
		}
		cover := rt.coveredBy(ctx, el, x, y)
		if cover == "" {
			return x, y, nil
		}
		if check == coverChecks {
			return 0, 0, fmt.Errorf("%q is covered by %s %s", target, cover, where)
		}
		if waitErr := rt.page.Wait(ctx, coverCheckEvery); waitErr != nil {
			return 0, 0, waitErr
		}
	}
}

// Something covering a click target is given coverChecks looks, coverCheckEvery
// apart, to go away before the step fails: about a second and a half.
const (
	coverChecks     = 10
	coverCheckEvery = 150 * time.Millisecond
)

// coveredBy names the element sitting on top of el at (x, y), or returns ""
// when a click there reaches el.
func (rt *Runtime) coveredBy(ctx context.Context, el dom.ElementSnapshot, x, y float64) string {
	page := rt.pageFor(el)
	// (x, y) is on the top-level viewport, where clicks land; the frame
	// looks at its own.
	if framed, ok := page.(browser.Framed); ok {
		dx, dy, err := framed.FrameOffset(ctx)
		if err != nil {
			return ""
		}
		x, y = x-dx, y-dy
	}
	raw, err := page.EvalJS(ctx, pagejs.CoveredBy(el.ID, el.XPath, x, y))
	if err != nil {
		return ""
	}
	var report struct {
		Covered bool   `json:"covered"`
		By      string `json:"by"`
	}
	if json.Unmarshal(raw, &report) != nil || !report.Covered {
		return ""
	}
	return report.By
}

// captionedBox narrows el, when it holds several checkboxes or radios, to the
// one captioned with target. An element with one box or none, or a box
// itself, is returned as it is. Several boxes and none captioned so is an
// error: acting on the first of them is acting on a box the step never named.
func (rt *Runtime) captionedBox(ctx context.Context, el dom.ElementSnapshot, target string, elements []dom.ElementSnapshot) (dom.ElementSnapshot, error) {
	if el.Tag == "label" {
		return el, nil
	}
	// A box that was ranked first may still be the wrong one of a pair whose
	// captions are bare text: it is weighed against its siblings, and kept
	// when none of them is captioned with the target either.
	scope, scopeID := el.XPath, el.ID
	if el.Tag == "input" {
		cut := strings.LastIndex(el.XPath, "/")
		if cut <= 0 {
			return el, nil
		}
		scope, scopeID = el.XPath[:cut], -1
	}
	raw, err := rt.pageFor(el).EvalJS(ctx, pagejs.CaptionedBox(scopeID, scope, target))
	if err != nil {
		return el, nil
	}
	var pick struct {
		Count int    `json:"count"`
		XPath string `json:"xpath"`
	}
	if json.Unmarshal(raw, &pick) != nil || pick.Count < 2 {
		return el, nil
	}
	if pick.XPath == "" {
		if el.Tag == "input" {
			return el, nil
		}
		return el, fmt.Errorf("target not found: %q names an element holding %d checkboxes, and none of them is captioned %q", target, pick.Count, target)
	}
	for _, candidate := range elements {
		if candidate.XPath == pick.XPath {
			return candidate, nil
		}
	}
	// Not in the snapshot; the page finds it by its path. An ID of -1 is
	// in no registry, so the path is what is used.
	return dom.ElementSnapshot{ID: -1, XPath: pick.XPath, Tag: "input"}, nil
}

// checkedState reads the state of the checkbox or radio el stands for, off the
// page as it is now. ok is false when the page has no such control to report.
func (rt *Runtime) checkedState(ctx context.Context, el dom.ElementSnapshot) (checked, ok bool) {
	raw, err := rt.pageFor(el).EvalJS(ctx, pagejs.CheckedState(el.ID, el.XPath))
	if err != nil {
		return false, false
	}
	var state struct {
		Found   bool `json:"found"`
		Checked bool `json:"checked"`
	}
	if json.Unmarshal(raw, &state) != nil {
		return false, false
	}
	return state.Checked, state.Found
}

func (rt *Runtime) rememberStickyCheckboxState(target string, checked bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	if rt.stickyCheckboxStates == nil {
		rt.stickyCheckboxStates = make(map[string]bool)
	}
	rt.stickyCheckboxStates[target] = checked
}

func (rt *Runtime) reconcileStickyCheckboxStates(ctx context.Context) error {
	if len(rt.stickyCheckboxStates) == 0 {
		return nil
	}
	states := rt.stickyCheckboxStates
	rt.stickyCheckboxStates = nil
	elements, err := rt.loadSnapshot(ctx)
	if err != nil {
		rt.stickyCheckboxStates = states
		return err
	}
	for target, desired := range states {
		ranked := rankForVerifyState(target, "checked", elements, nil)
		if !verifyRankedCandidateAcceptable("checked", ranked) {
			continue
		}
		winner := ranked[0].Element
		if winner.IsChecked == desired {
			continue
		}
		if err := rt.pageFor(winner).SetChecked(ctx, winner.ID, winner.XPath, desired); err != nil {
			return err
		}
		rt.invalidateSnapshot()
		elements, err = rt.loadSnapshot(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (rt *Runtime) tryClickNearbyDropdownControl(ctx context.Context, anchor dom.ElementSnapshot) (bool, error) {
	js := fmt.Sprintf(`(() => {
		const anchor = (window.__manulReg && window.__manulReg[%[1]d]) || document.evaluate(%[2]q, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
		if (!anchor) return false;

		const isVisible = (node) => {
			if (!node) return false;
			const rect = node.getBoundingClientRect();
			const cs = window.getComputedStyle(node);
			return cs.display !== 'none' && cs.visibility !== 'hidden' && parseFloat(cs.opacity || '1') > 0 && rect.width > 0 && rect.height > 0;
		};

		let root = anchor.closest('.widget, .form-group, section, article, aside, div') || anchor.parentElement || anchor;
		const preferredSelectors = [
			'#comboBox',
			'[role="combobox"]',
			'[id*="combo"]',
			'[class*="combo"]',
			'[id*="dropdown"]',
			'[class*="dropdown"]'
		];
		const selectors = [
			'#comboBox',
			'[role="combobox"]',
			'select',
			'input[list]',
			'input[type="text"]',
			'[id*="combo"]',
			'[class*="combo"]',
			'[id*="dropdown"]',
			'[class*="dropdown"]'
		];

		let target = null;
		for (const selector of preferredSelectors) {
			const matches = Array.from(document.querySelectorAll(selector)).filter(isVisible);
			const preferred = matches.find(node => node.id === 'comboBox' || node.getAttribute('role') === 'combobox' || node.tagName === 'INPUT');
			if (preferred) {
				target = preferred;
				break;
			}
		}

		for (const selector of selectors) {
			if (target) break;
			const matches = Array.from(root.querySelectorAll(selector)).filter(isVisible);
			const preferred = matches.find(node => node.id === 'comboBox' || node.getAttribute('role') === 'combobox' || node.tagName === 'SELECT' || node.tagName === 'INPUT');
			if (preferred) {
				target = preferred;
				break;
			}
			if (matches.length > 0) {
				target = matches[0];
				break;
			}
		}

		if (!target && root.parentElement) {
			root = root.parentElement;
			for (const selector of selectors) {
				const candidate = root.querySelector(selector);
				if (isVisible(candidate)) {
					target = candidate;
					break;
				}
			}
		}

		if (!target) return false;
		target.scrollIntoView({block: 'center', inline: 'center'});
		if (typeof target.focus === 'function') target.focus();
		['mousedown', 'mouseup', 'click'].forEach((evt) => {
			target.dispatchEvent(new MouseEvent(evt, { bubbles: true, cancelable: true, view: window }));
		});
		if (typeof target.click === 'function') target.click();
		return true;
	})()`, anchor.ID, anchor.XPath)

	raw, err := rt.page.EvalJS(ctx, js)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(raw)) == "true", nil
}

func (rt *Runtime) tryClickDropdownTriggerByLabel(ctx context.Context, targetPath string) (bool, error) {
	js := fmt.Sprintf(`(() => {
		const normalize = (value) => (value || '').replace(/\s+/g, ' ').trim().toLowerCase();
		const isVisible = (node) => {
			if (!node) return false;
			const rect = node.getBoundingClientRect();
			const cs = window.getComputedStyle(node);
			return cs.display !== 'none' && cs.visibility !== 'hidden' && parseFloat(cs.opacity || '1') > 0 && rect.width > 0 && rect.height > 0;
		};
		const clickNode = (node) => {
			if (!node) return false;
			node.scrollIntoView({ block: 'center', inline: 'center' });
			if (typeof node.focus === 'function') node.focus();
			['mousedown', 'mouseup', 'click'].forEach((evt) => {
				node.dispatchEvent(new MouseEvent(evt, { bubbles: true, cancelable: true, view: window }));
			});
			if (typeof node.click === 'function') node.click();
			return true;
		};

		const wanted = normalize(%q);
		const triggerSelectors = ['#comboBox', '[role="combobox"]', 'input[list]', '[id*="combo"]', '[class*="combo"]'];
		const all = Array.from(document.querySelectorAll('h1,h2,h3,h4,h5,h6,label,legend,span,div,p'));
		const labels = all.filter((node) => normalize(node.innerText) === wanted);
		for (const label of labels) {
			let scope = label;
			for (let depth = 0; depth < 6 && scope; depth += 1) {
				for (const selector of triggerSelectors) {
					const local = Array.from(scope.querySelectorAll(selector)).find(isVisible);
					if (local && clickNode(local)) return true;
				}
				let sibling = scope.nextElementSibling;
				while (sibling) {
					for (const selector of triggerSelectors) {
						const siblingMatch = sibling.matches(selector) ? sibling : sibling.querySelector(selector);
						if (isVisible(siblingMatch) && clickNode(siblingMatch)) return true;
					}
					sibling = sibling.nextElementSibling;
				}
				scope = scope.parentElement;
			}
		}

		for (const selector of triggerSelectors) {
			const fallback = Array.from(document.querySelectorAll(selector)).find(isVisible);
			if (fallback && clickNode(fallback)) return true;
		}
		return false;
	})()`, targetPath)

	raw, err := rt.page.EvalJS(ctx, js)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(raw)) == "true", nil
}

func (rt *Runtime) trySetShadowInputValue(ctx context.Context, targetPath, value string) (bool, error) {
	js := fmt.Sprintf(`(() => {
		const normalize = (text) => (text || '').replace(/\s+/g, ' ').trim().toLowerCase();
		const query = normalize(%q);
		const value = %q;
		const hosts = [];
		const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT);
		while (walker.nextNode()) {
			const node = walker.currentNode;
			if (node.shadowRoot) hosts.push(node);
		}
		for (const host of hosts) {
			const nearby = normalize((host.parentElement && host.parentElement.innerText) || host.innerText || host.id || host.className || '');
			if (query && !nearby.includes('shadow') && !query.includes('shadow')) continue;
			const control = host.shadowRoot.querySelector('input[type="text"], textarea, input:not([type]), input[type="search"], input[type="email"], input[type="tel"], input[type="url"]');
			if (!control) continue;
			host.scrollIntoView({ block: 'center', inline: 'center' });
			control.focus();
			control.value = value;
			control.dispatchEvent(new Event('input', { bubbles: true }));
			control.dispatchEvent(new Event('change', { bubbles: true }));
			return true;
		}
		return false;
	})()`, targetPath, value)

	raw, err := rt.page.EvalJS(ctx, js)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(raw)) == "true", nil
}

func (rt *Runtime) tryClickVisibleDropdownOption(ctx context.Context, targetPath string) (bool, error) {
	js := fmt.Sprintf(`(() => {
		const normalize = (value) => (value || '').replace(/\s+/g, ' ').trim().toLowerCase();
		const isVisible = (node) => {
			if (!node) return false;
			const rect = node.getBoundingClientRect();
			const cs = window.getComputedStyle(node);
			return cs.display !== 'none' && cs.visibility !== 'hidden' && parseFloat(cs.opacity || '1') > 0 && rect.width > 0 && rect.height > 0;
		};

		const wanted = normalize(%q);
		let list = document.querySelector('#dropdown') || document.querySelector('[role="listbox"]') || document.querySelector('[class*="dropdown"]');
		const combo = document.querySelector('#comboBox') || document.querySelector('[role="combobox"]') || document.querySelector('input[list]') || document.querySelector('[id*="combo"]');
		if ((!list || !isVisible(list)) && combo && isVisible(combo)) {
			combo.scrollIntoView({ block: 'center', inline: 'center' });
			if (typeof combo.focus === 'function') combo.focus();
			['mousedown', 'mouseup', 'click'].forEach((evt) => {
				combo.dispatchEvent(new MouseEvent(evt, { bubbles: true, cancelable: true, view: window }));
			});
			if (typeof combo.click === 'function') combo.click();
		}
		list = document.querySelector('#dropdown') || document.querySelector('[role="listbox"]') || document.querySelector('[class*="dropdown"]');
		if (!list || !isVisible(list)) return false;

		const candidates = Array.from(list.querySelectorAll('.option, [role="option"], div, li'))
			.filter(isVisible)
			.filter((node) => normalize(node.innerText) !== '');
		const target = candidates.find((node) => normalize(node.innerText) === wanted) ||
			candidates.find((node) => normalize(node.innerText).includes(wanted));
		if (!target) return false;

		target.scrollIntoView({ block: 'center', inline: 'nearest' });
		['mousedown', 'mouseup', 'click'].forEach((evt) => {
			target.dispatchEvent(new MouseEvent(evt, { bubbles: true, cancelable: true, view: window }));
		});
		if (typeof target.click === 'function') target.click();
		return true;
	})()`, targetPath)

	raw, err := rt.page.EvalJS(ctx, js)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(raw)) == "true", nil
}

// settleAfterPointer finishes a double or right click: the page is given a
// moment to react, and what was known about it is thrown away. Both exist to
// change the page — a context menu, an inline editor — and the step that
// follows used to look for what they opened in the snapshot taken before.
func (rt *Runtime) settleAfterPointer(ctx context.Context, err error) error {
	if err != nil {
		return err
	}
	rt.invalidateSnapshot()
	return rt.page.Wait(ctx, 300*time.Millisecond)
}

func (rt *Runtime) loadSnapshot(ctx context.Context) ([]dom.ElementSnapshot, error) {
	if !rt.cfg.DisableCache && rt.cachedElements != nil {
		return rt.cachedElements, nil
	}
	raw, err := rt.page.CallProbe(ctx, heuristics.BuildSnapshotProbe(), nil)
	if err != nil {
		return nil, fmt.Errorf("probe failed: %w", err)
	}
	elements, err := heuristics.ParseProbeResult(raw)
	if err != nil {
		return nil, fmt.Errorf("parse probe failed: %w", err)
	}
	elements = append(elements, rt.frameElements(ctx)...)
	if !rt.cfg.DisableCache {
		rt.cachedElements = elements
	}
	return elements, nil
}

func (rt *Runtime) invalidateSnapshot() {
	rt.cachedElements = nil
}

func appendRankedCandidates(res *explain.ExecutionResult, ranked []scorer.RankedCandidate, limit int) {
	if limit <= 0 || len(ranked) < limit {
		limit = len(ranked)
	}
	for i := 0; i < limit; i++ {
		res.RankedCandidates = append(res.RankedCandidates, ranked[i].Explain)
	}
}

func collapseNestedDuplicateRankedCandidates(ranked []scorer.RankedCandidate) []scorer.RankedCandidate {
	if len(ranked) < 2 {
		return ranked
	}
	collapsed := make([]scorer.RankedCandidate, 0, len(ranked))
	for _, candidate := range ranked {
		merged := false
		for i := range collapsed {
			if !nestedDuplicateRankedCandidates(collapsed[i], candidate) {
				continue
			}
			if preferMoreSpecificRankedCandidate(candidate, collapsed[i]) {
				collapsed[i] = candidate
			}
			merged = true
			break
		}
		if !merged {
			collapsed = append(collapsed, candidate)
		}
	}
	for i := range collapsed {
		collapsed[i].Explain.Rank = i + 1
		collapsed[i].Explain.Chosen = i == 0
	}
	return collapsed
}

func nestedDuplicateRankedCandidates(existing, candidate scorer.RankedCandidate) bool {
	if math.Abs(existing.Explain.Score.Total-candidate.Explain.Score.Total) > 0.02 {
		return false
	}
	textA := normalizeCandidateText(existing.Element.VisibleText)
	textB := normalizeCandidateText(candidate.Element.VisibleText)
	if textA == "" || textA != textB {
		return false
	}
	if existing.Element.FrameIndex != candidate.Element.FrameIndex ||
		!(isXPathAncestor(existing.Element.XPath, candidate.Element.XPath) || isXPathAncestor(candidate.Element.XPath, existing.Element.XPath)) {
		return false
	}
	return rectIntersectionRatio(existing.Element.Rect, candidate.Element.Rect) >= 0.85
}

func preferMoreSpecificRankedCandidate(candidate, existing scorer.RankedCandidate) bool {
	candidateDepth := len(xpathParts(candidate.Element.XPath))
	existingDepth := len(xpathParts(existing.Element.XPath))
	if candidateDepth != existingDepth {
		return candidateDepth > existingDepth
	}
	candidateArea := candidate.Element.Rect.Width * candidate.Element.Rect.Height
	existingArea := existing.Element.Rect.Width * existing.Element.Rect.Height
	if candidateArea != existingArea {
		return candidateArea < existingArea
	}
	return candidate.Element.ID > existing.Element.ID
}

func normalizeCandidateText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func xpathParts(xpath string) []string {
	var parts []string
	for _, part := range strings.Split(xpath, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func isXPathAncestor(ancestor, descendant string) bool {
	if ancestor == "" || descendant == "" || ancestor == descendant {
		return false
	}
	ancestorParts := xpathParts(ancestor)
	descendantParts := xpathParts(descendant)
	if len(ancestorParts) >= len(descendantParts) {
		return false
	}
	for i := range ancestorParts {
		if ancestorParts[i] != descendantParts[i] {
			return false
		}
	}
	return true
}

func rectIntersectionRatio(a, b dom.Rect) float64 {
	left := math.Max(a.Left, b.Left)
	top := math.Max(a.Top, b.Top)
	right := math.Min(a.Right, b.Right)
	bottom := math.Min(a.Bottom, b.Bottom)
	if right <= left || bottom <= top {
		return 0.0
	}
	intersection := (right - left) * (bottom - top)
	areaA := a.Width * a.Height
	areaB := b.Width * b.Height
	if areaA <= 0 || areaB <= 0 {
		return 0.0
	}
	return intersection / math.Min(areaA, areaB)
}

func selectionIsAmbiguous(ranked []scorer.RankedCandidate) bool {
	if len(ranked) == 0 {
		return true
	}
	best := ranked[0].Explain.Score.Total
	if best < ThresholdAmbiguous {
		return true
	}
	if len(ranked) == 1 {
		return false
	}
	return best < ThresholdHighConfidence && best-ranked[1].Explain.Score.Total < ThresholdRunnerUpGap
}

// nearbyControls keeps the five best candidates that are near enough to the
// label for pass 3 to consider them at all. Ranked by score alone, a search
// box across the page outranked the field beside the caption on the strength
// of its placeholder, and pass 3 then gave up because the winner was not near.
func nearbyControls(ranked []scorer.RankedCandidate) []scorer.RankedCandidate {
	var out []scorer.RankedCandidate
	for _, r := range ranked {
		if r.Explain.Score.ProximityScore >= ThresholdPass3Proximity {
			r.Explain.Rank = len(out) + 1
			r.Explain.Chosen = len(out) == 0
			out = append(out, r)
			if len(out) == 5 {
				break
			}
		}
	}
	return out
}

func pass3CandidateAcceptable(ranked []scorer.RankedCandidate) bool {
	if len(ranked) == 0 {
		return false
	}
	best := ranked[0].Explain.Score
	if best.Total < ThresholdPass3Total || best.ProximityScore < ThresholdPass3Proximity {
		return false
	}
	if len(ranked) == 1 {
		return true
	}
	runnerUp := ranked[1].Explain.Score
	return best.Total-runnerUp.Total >= ThresholdPass3Gap ||
		best.ProximityScore-runnerUp.ProximityScore >= ThresholdPass3NearerGap
}

// focusTarget gives keyboard focus to the element a PRESS … ON '<target>' names.
// A field is tried first, since that is what a key is usually aimed at, then
// anything clickable.
func (rt *Runtime) focusTarget(ctx context.Context, target string) error {
	elements, err := rt.loadSnapshot(ctx)
	if err != nil {
		return err
	}
	for _, mode := range []dsl.InteractionMode{dsl.ModeInput, dsl.ModeClickable} {
		ranked := scorer.Rank(target, "", string(mode), elements, 1, nil)
		if len(ranked) == 0 || ranked[0].Explain.Score.Total < ThresholdAmbiguous ||
			!scorer.MatchesQuery(target, &ranked[0].Element) {
			continue
		}
		return rt.pageFor(ranked[0].Element).Focus(ctx, ranked[0].Element.ID, ranked[0].Element.XPath)
	}
	return fmt.Errorf("target not found: %q", target)
}

// splitKeyChord separates "Control+Shift+P" into the key and the modifier
// bitmask Page.DispatchKey takes (1=Alt, 2=Ctrl, 4=Meta, 8=Shift). Anything
// that is not a chord it recognises is returned unchanged with no modifiers,
// so a literal "+" is still pressable.
func splitKeyChord(chord string) (string, int) {
	parts := strings.Split(chord, "+")
	key := strings.TrimSpace(parts[len(parts)-1])
	if len(parts) < 2 || key == "" {
		return chord, 0
	}
	modifiers := 0
	for _, part := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "alt", "option":
			modifiers |= 1
		case "control", "ctrl":
			modifiers |= 2
		case "meta", "cmd", "command", "win", "super":
			modifiers |= 4
		case "shift":
			modifiers |= 8
		default:
			return chord, 0
		}
	}
	// A shortcut is bound to the letter, not to its capital: Control+A is the
	// "a" key with Control held.
	if modifiers&^8 != 0 && len(key) == 1 {
		key = strings.ToLower(key)
	}
	return key, modifiers
}

// truncateRunes cuts s to at most n characters without splitting one.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
