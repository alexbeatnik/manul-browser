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

// A click that changes nothing the page has to ask the server about — a
// checkbox, a menu — is not waited for at all.
func TestRoute_AnActionThatAskedNothingIsNotWaitedFor(t *testing.T) {
	page, polls := routePage(`{"moved":false,"pending":0,"idle":0}`)
	if _, err := runSource(t, page, "CLICK the 'Next page' link\n"); err != nil {
		t.Fatal(err)
	}
	if *polls != 1 {
		t.Errorf("asked the page %d times, want 1", *polls)
	}
}

// Add to cart asks the server and leaves the address where it was; the total
// the step after it read was the old one.
func TestRoute_ARequestTheActionStartedIsWaitedFor(t *testing.T) {
	page, polls := routePage(
		`{"moved":false,"pending":1,"idle":0}`,
		`{"moved":false,"pending":1,"idle":40}`,
		`{"moved":false,"pending":0,"idle":30}`, // back, and being drawn
		`{"moved":false,"pending":0,"idle":250}`,
	)
	if _, err := runSource(t, page, "CLICK the 'Next page' link\n"); err != nil {
		t.Fatal(err)
	}
	if *polls != 4 {
		t.Errorf("asked the page %d times, want 4", *polls)
	}
}

// Rozetka's search asked the server first and changed the address only when
// it answered, then asked again for the category's products. The step
// returned on the old address and the old page.
func TestRoute_TheAddressMovesWhenTheServerAnswers(t *testing.T) {
	page, polls := routePage(
		`{"moved":false,"pending":1,"idle":0}`,
		`{"moved":false,"pending":1,"idle":60}`,
		`{"moved":true,"pending":1,"idle":0}`, // the second request
		`{"moved":true,"pending":0,"idle":20}`,
		`{"moved":true,"pending":0,"idle":300}`,
	)
	if _, err := runSource(t, page, "PRESS Enter\n"); err != nil {
		t.Fatal(err)
	}
	if *polls != 5 {
		t.Errorf("asked the page %d times, want 5", *polls)
	}
	closed := false
	for _, e := range page.EvalCalls {
		if e == pagejs.CloseActionWindow {
			closed = true
		}
		if e == pagejs.RouteState && !closed {
			t.Fatal("the page was asked before the action's window was closed: a background poll would count")
		}
	}
}

// Check out asks the server and then goes to another page: the wait for the
// request becomes the wait for the document that replaces this one.
func TestRoute_ARequestThatEndsInANavigationWaitsForThePage(t *testing.T) {
	page := &MockPage{Elements: []dom.ElementSnapshot{regEl(1, "button", "Check out")}}
	nav := 0
	page.EvalResult = func(expr string) ([]byte, bool) {
		switch expr {
		case pagejs.NavigationState:
			nav++
			switch {
			case nav < 3:
				return []byte("0"), true
			case nav < 5:
				return []byte("1"), true // leaving
			}
			return []byte("2"), true
		case pagejs.RouteState:
			return []byte(`{"moved":false,"pending":1,"idle":0}`), true
		}
		return nil, false
	}
	if _, err := runSource(t, page, "CLICK the 'Check out' button\n"); err != nil {
		t.Fatal(err)
	}
	if nav != 5 {
		t.Errorf("looked for the navigation %d times, want 5: the step stopped before the new page", nav)
	}
}

// A page whose DOM never goes still after the answer is given a second.
func TestRoute_AQuietThatNeverComesIsGivenUpOn(t *testing.T) {
	page, _ := routePage(`{"moved":true,"pending":0,"idle":0}`)
	start := time.Now()
	if _, err := runSource(t, page, "PRESS Enter\n"); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took < routeQuietTimeout || took > routeQuietTimeout+time.Second {
		t.Errorf("took %s, want about %s", took, routeQuietTimeout)
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
