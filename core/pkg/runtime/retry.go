package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/explain"
)

// RunWithRetries runs a hunt and, while it fails, runs it again — up to retries
// more times. It is what `--retries` means, shared by the sequential and
// parallel paths so the two cannot count differently.
//
// run performs one attempt, numbered from 1. Each attempt should start from a
// fresh Runtime: a retry that inherits the failed attempt's variables is not
// the same hunt run again.
//
// The result returned is the last attempt's, with Attempts set and the time of
// every attempt added up. A hunt that failed and then passed is Flaky, and
// counts as a pass. A cancelled context and a debugger stop are not retried:
// neither is the hunt failing.
func RunWithRetries(ctx context.Context, retries int, run func(attempt int) (*explain.HuntResult, error)) (*explain.HuntResult, error) {
	var spent time.Duration
	for attempt := 1; ; attempt++ {
		res, err := run(attempt)
		if res != nil {
			spent += res.TotalDuration
			res.Attempts = attempt
			res.TotalDuration = spent
			res.TotalDurationMS = spent.Milliseconds()
		}

		if err == nil && res != nil && res.Success {
			res.Flaky = attempt > 1
			return res, nil
		}
		if attempt > retries || ctx.Err() != nil || errors.Is(err, ErrDebugStop) {
			return res, err
		}
	}
}
