package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/explain"
)

// Screenshot modes, as `--screenshot` / MANUL_SCREENSHOT / `screenshot` spell
// them. Anything else — including the empty string a zero Config carries —
// captures nothing.
const (
	screenshotOnFail = "on-fail"
	screenshotAlways = "always"
)

// stepShotDir is where step screenshots land, beside the ones the SCREENSHOT
// verb writes: screenshots/ under the working directory.
const stepShotDir = "screenshots"

// stepShotSeq keeps two shots taken in the same millisecond — parallel workers
// do that — from overwriting each other.
var stepShotSeq atomic.Uint64

// captureStepScreenshot saves what the page looked like after a step, when the
// configured mode asks for it, and records the file on the step's result.
//
// It never changes the step's outcome: a screenshot that cannot be taken is a
// missing picture, not a failed test.
func (rt *Runtime) captureStepScreenshot(ctx context.Context, cmd dsl.Command, res *explain.ExecutionResult, stepErr error) {
	switch strings.ToLower(strings.TrimSpace(rt.cfg.Screenshot)) {
	case screenshotAlways:
	case screenshotOnFail:
		if stepErr == nil {
			return
		}
	default:
		return
	}

	if errors.Is(stepErr, ErrDebugStop) {
		return // stopping the run from the debugger is not a failure to document
	}

	// A block reports the failure of a step inside it. That step already has
	// its picture; the block would only add the same one again.
	switch cmd.Type {
	case dsl.CmdIf, dsl.CmdRepeat, dsl.CmdWhile, dsl.CmdForEach:
		return
	}

	// The step's own context may be the reason it failed — a timeout is when
	// the picture matters most — so the capture gets a short one of its own.
	shotCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	data, err := rt.page.Screenshot(shotCtx)
	if err != nil || len(data) == 0 {
		if err != nil {
			rt.logger.Debug("step screenshot skipped: %v", err)
		}
		return
	}
	if err := os.MkdirAll(stepShotDir, 0o755); err != nil {
		rt.logger.Debug("step screenshot skipped: %v", err)
		return
	}

	kind := "step"
	if stepErr != nil {
		kind = "fail"
	}
	name := fmt.Sprintf("%s_%d_%03d.png", kind, time.Now().UnixMilli(), stepShotSeq.Add(1))
	path := filepath.Join(stepShotDir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		rt.logger.Debug("step screenshot skipped: %v", err)
		return
	}

	// Forward slashes: the path is read back from JSON and from the HTML
	// report, on machines other than the one that wrote it.
	res.ScreenshotPath = filepath.ToSlash(path)
	if stepErr != nil {
		rt.logger.ActionDetail("📸", "Screenshot: %s", res.ScreenshotPath)
	}
}
