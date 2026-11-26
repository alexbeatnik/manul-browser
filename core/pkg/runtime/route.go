package runtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
)

const (
	// routeQuiet is how long a page that changed its address must go without
	// a change of its DOM or of a request it started before it is taken to
	// have drawn the new address.
	routeQuiet = 200 * time.Millisecond
	// routeSettleTimeout bounds that wait. A page that never goes quiet — an
	// animation that rewrites text, a request that is held open — costs this
	// much and no more; the step goes on as it did before.
	routeSettleTimeout = 5 * time.Second
	// routePollInterval is how often the page is asked.
	routePollInterval = 50 * time.Millisecond
)

// settleRoute waits for a page that changed its address without loading
// another document — a single-page app's route — to draw what the new address
// shows.
//
// Neither readyState nor beforeunload says anything about such a page: the
// document is the one it was, and "complete". On YouTube, Enter in the search
// box changes the address at once and asks the server for the results; for
// the 600 ms the request took the page did not change at all, and the map
// taken after the step showed the search suggestions and no results. So a
// page whose address moved is waited for until the requests it started since
// the action have come back and its DOM has been still for routeQuiet. A page
// whose address did not move is not waited for: a click that only fetches
// something is the next step's to wait for, as any target is.
func (rt *Runtime) settleRoute(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, routeSettleTimeout)
	defer cancel()
	for {
		raw, err := rt.page.EvalJS(ctx, pagejs.RouteState)
		if err != nil {
			return
		}
		var st struct {
			Moved   bool    `json:"moved"`
			Pending int     `json:"pending"`
			Idle    float64 `json:"idle"`
		}
		if json.Unmarshal(raw, &st) != nil || !st.Moved {
			return
		}
		if st.Pending <= 0 && time.Duration(st.Idle)*time.Millisecond >= routeQuiet {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(routePollInterval):
		}
	}
}
