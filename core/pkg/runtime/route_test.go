package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/pagejs"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

// routePage answers RouteState from a script, one answer per poll, and the
// last answer for every poll after it.
func routePage(answers ...string) (*MockPage, *int) {
	polls := 0
	page := &MockPage{Elements: []dom.ElementSnapshot{regEl(1, "a", "Next page")}}
	page.EvalResult = func(expr string) ([]byte, bool) {
		if expr != pagejs.RouteState {
			return nil, false
		}
		a := answers[len(answers)-1]
		if polls < len(answers) {
			a = answers[polls]
		}
		polls++
		return []byte(a), true
	}
	return page, &polls
}

// On YouTube, Enter in the search box changed the address at once and asked
// the server for the results. The step returned on the address, and the map
// after it showed the suggestions and no results: the page was the old one
// for as long as the request took.
func TestRoute_PressWaitsForWhatTheNewAddressShows(t *testing.T) {
	page, polls := routePage(
		`{"moved":true,"pending":1,"idle":40}`,
		`{"moved":true,"pending":1,"idle":90}`,
		`{"moved":true,"pending":0,"idle":20}`, // the results came back and are being drawn
		`{"moved":true,"pending":0,"idle":250}`,
	)
	if _, err := runSource(t, page, "PRESS Enter\n"); err != nil {
		t.Fatal(err)
	}
	if *polls != 4 {
		t.Errorf("asked the page %d times, want 4: the step stopped waiting before the results", *polls)
	}
}

func TestRoute_ClickWaitsForWhatTheNewAddressShows(t *testing.T) {
	page, polls := routePage(
		`{"moved":true,"pending":1,"idle":40}`,
		`{"moved":true,"pending":0,"idle":300}`,
	)
	if _, err := runSource(t, page, "CLICK the 'Next page' link\n"); err != nil {
		t.Fatal(err)
	}
	if *polls != 2 {
		t.Errorf("asked the page %d times, want 2", *polls)
	}
}

// A click that only fetches something — Add to cart — leaves the address
// where it was. The next step waits for its own target, so this one does not.
func TestRoute_AnAddressThatStayedIsNotWaitedFor(t *testing.T) {
	page, polls := routePage(`{"moved":false,"pending":3,"idle":0}`)
	if _, err := runSource(t, page, "CLICK the 'Next page' link\n"); err != nil {
		t.Fatal(err)
	}
	if *polls != 1 {
		t.Errorf("asked the page %d times, want 1", *polls)
	}
}

// A page that never goes quiet — an animation that rewrites text, a request
// held open — is given up on, and the step passes as it did before.
func TestRoute_APageThatNeverSettlesIsGivenUpOn(t *testing.T) {
	page, _ := routePage(`{"moved":true,"pending":1,"idle":0}`)
	cfg := regConfig()
	cfg.NavTimeout = 400 * time.Millisecond
	rt := New(cfg, page, utils.NewLoggerTo(nopWriter{}, nil))
	hunt, err := dsl.Parse(strings.NewReader("PRESS Enter\n"))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := rt.RunHunt(context.Background(), hunt); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("took %s: the wait is bounded by nav_timeout", took)
	}
}

// A document that was never armed, or an answer that is not the state, is not
// a reason to wait.
func TestRoute_NoAnswerIsNoWait(t *testing.T) {
	page := &MockPage{} // answers RouteState with nothing
	start := time.Now()
	if _, err := runSource(t, page, "PRESS Enter\n"); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("took %s", took)
	}
}
