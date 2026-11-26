package runtime

import (
	"context"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/explain"
)

// targetPollInterval is how often a step looks again for a target that is
// not on the page yet.
const targetPollInterval = 250 * time.Millisecond

// waitsForTarget lists the verbs that look for their target again, until the
// step timeout, when it is not on the page yet. Every one of them finds its
// target before it changes anything.
var waitsForTarget = map[dsl.CommandType]bool{
	dsl.CmdClick: true, dsl.CmdDoubleClick: true, dsl.CmdRightClick: true,
	dsl.CmdFill: true, dsl.CmdType: true, dsl.CmdSelect: true,
	dsl.CmdCheck: true, dsl.CmdUncheck: true, dsl.CmdHover: true,
	dsl.CmdDrag: true, dsl.CmdUploadFile: true, dsl.CmdExtract: true,
	dsl.CmdPress: true, dsl.CmdScroll: true,
}

// executeCommand runs one step, and for an action whose target is not on the
// page yet runs it again until the target appears or the step timeout
// (`timeout`, default 5s) passes.
//
// Content that a page loads after it has loaded — products fetched by AJAX,
// a dialog that fades in, a list that renders on scroll — made the step after
// NAVIGATE fail "target not found" unless the hunt put a WAIT FOR in front of
// it. A person waits for the thing to show up; so does the engine now.
//
// Only a not-found failure is retried, and only while the step has changed
// nothing: once it has typed, clicked or scrolled (rt.acted), running it
// again would do that twice. An ambiguous target is not retried either —
// more content does not usually make a choice clearer.
func (rt *Runtime) executeCommand(ctx context.Context, cmd dsl.Command) (explain.ExecutionResult, error) {
	if !waitsForTarget[cmd.Type] || rt.cfg.DefaultTimeout <= 0 {
		rt.acted = false
		return rt.executeCommandOnce(ctx, cmd)
	}
	start := time.Now()
	deadline := start.Add(rt.cfg.DefaultTimeout)
	waited := false
	for {
		rt.acted = false
		res, err := rt.executeCommandOnce(ctx, cmd)
		if err == nil || rt.acted || ctx.Err() != nil || !notFound(ctx, cmd, res, err) ||
			time.Now().Add(targetPollInterval).After(deadline) {
			if waited {
				res.Duration = time.Since(start)
				res.DurationMS = res.Duration.Milliseconds()
				if err != nil {
					rt.logger.ActionDetail("⏳", "Still not on the page after %s", rt.cfg.DefaultTimeout)
				}
			}
			return res, err
		}
		if !waited {
			waited = true
			rt.logger.ActionDetail("⏳", "Not on the page yet; looking again for up to %s", rt.cfg.DefaultTimeout)
		}
		select {
		case <-ctx.Done():
			return res, err
		case <-time.After(targetPollInterval):
		}
		rt.invalidateSnapshot()
	}
}

func notFound(ctx context.Context, cmd dsl.Command, res explain.ExecutionResult, err error) bool {
	if res.FailureReason != "" {
		return res.FailureReason == explain.ReasonNotFound
	}
	return classifyFailure(ctx, cmd, err) == explain.ReasonNotFound
}
