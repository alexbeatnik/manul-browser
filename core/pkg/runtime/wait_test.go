package runtime

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alexbeatnik/manul-browser/core/pkg/browser"
	"github.com/alexbeatnik/manul-browser/core/pkg/config"
	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

// lateMock adds elements to the page once it has been looked at `after`
// times, the way content fetched after load arrives.
type lateMock struct {
	*MockPage
	after int
	late  []dom.ElementSnapshot
	seen  int
}

func (m *lateMock) CallProbe(ctx context.Context, fn string, arg any) ([]byte, error) {
	if strings.Contains(fn, "elements:") {
		m.seen++
		if m.seen == m.after {
			m.Elements = append(m.Elements, m.late...)
		}
	}
	return m.MockPage.CallProbe(ctx, fn, arg)
}

// Products fetched after load made the step after NAVIGATE fail "target not
// found" unless the hunt waited for them first.
func TestWait_ATargetThatArrivesLateIsActedOnOnce(t *testing.T) {
	page := &lateMock{
		MockPage: &MockPage{Elements: []dom.ElementSnapshot{regEl(1, "h1", "Products")}},
		after:    3,
		late:     []dom.ElementSnapshot{regEl(2, "button", "Add to cart")},
	}
	cfg := regConfig()
	cfg.DefaultTimeout = 3 * time.Second
	err := runOnCfg(t, page, cfg, "CLICK the 'Add to cart' button\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Clicks) != 1 {
		t.Errorf("clicked %d times, want 1", len(page.Clicks))
	}
}

func TestWait_AMissingTargetFailsAfterTheTimeout(t *testing.T) {
	page := &MockPage{Elements: []dom.ElementSnapshot{regEl(1, "h1", "Products")}}
	cfg := regConfig()
	cfg.DefaultTimeout = 600 * time.Millisecond
	start := time.Now()
	err := runOnCfg(t, page, cfg, "CLICK the 'Add to cart' button\n")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want not found, got %v", err)
	}
	if took := time.Since(start); took < 400*time.Millisecond || took > 2*time.Second {
		t.Errorf("failed after %v, want about the 600ms timeout", took)
	}
}

// typeFailsMock fails every typing with an error that reads as not-found —
// the node went away under the action — and counts the attempts.
type typeFailsMock struct {
	*MockPage
	typed int
}

func (m *typeFailsMock) SetInputValue(ctx context.Context, id int, xpath, value string) error {
	m.typed++
	return fmt.Errorf("node not found for id %d", id)
}

// Once a step has acted, running it again would act twice, whatever its
// failure says.
func TestWait_AStepThatHasActedIsNotRunAgain(t *testing.T) {
	field := regEl(1, "input", "")
	field.InputType = "text"
	field.LabelText = "Email"
	page := &typeFailsMock{MockPage: &MockPage{Elements: []dom.ElementSnapshot{field}}}
	cfg := regConfig()
	cfg.DefaultTimeout = 2 * time.Second
	if err := runOnCfg(t, page, cfg, "FILL 'Email' field with 'a@b.c'\n"); err == nil {
		t.Fatal("want the typing failure")
	}
	if page.typed != 1 {
		t.Errorf("typed %d times, want 1", page.typed)
	}
}

func runOnCfg(t *testing.T, page browser.Page, cfg config.Config, src string) error {
	t.Helper()
	hunt, err := dsl.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = New(cfg, page, utils.NewLoggerTo(nopWriter{}, nil)).RunHunt(context.Background(), hunt)
	return err
}
