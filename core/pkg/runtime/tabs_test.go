package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/browser"
	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

// tabbedMock is a MockPage that can open tabs. opens maps a click count to
// the tab that click opens; closesAfter is the click that closes its own tab.
type tabbedMock struct {
	*MockPage
	id          string
	tabs        map[string]*tabbedMock
	opens       map[int]*tabbedMock
	closesAfter int
}

func (m *tabbedMock) OpenedTabs(ctx context.Context) ([]browser.Tab, error) {
	var out []browser.Tab
	for n, child := range m.opens {
		if len(m.Clicks) >= n && !child.closed() {
			out = append(out, browser.Tab{ID: child.id, URL: child.URL})
		}
	}
	return out, nil
}

func (m *tabbedMock) AttachTab(ctx context.Context, id string) (browser.Page, error) {
	return m.tabs[id], nil
}

func (m *tabbedMock) closed() bool { return m.closesAfter > 0 && len(m.Clicks) >= m.closesAfter }

func (m *tabbedMock) TabClosed(ctx context.Context) bool { return m.closed() }

func (m *tabbedMock) ActivateTab(ctx context.Context) error { return nil }

func runOn(t *testing.T, page browser.Page, src string) error {
	t.Helper()
	hunt, err := dsl.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil)).RunHunt(context.Background(), hunt)
	return err
}

// CLICK on a target="_blank" link passed, and the steps after it looked for
// their targets on the page that had been left behind. A popup that closes
// itself, as a sign-in popup does, hands back to its opener.
func TestTabs_ANewTabIsFollowedAndLeftWhenItCloses(t *testing.T) {
	popup := &tabbedMock{id: "popup", closesAfter: 1, MockPage: &MockPage{
		URL:      "https://shop.test/signin",
		Elements: []dom.ElementSnapshot{regEl(1, "button", "Authorize")},
	}}
	opener := &tabbedMock{id: "main", MockPage: &MockPage{
		URL: "https://shop.test/",
		Elements: []dom.ElementSnapshot{
			regEl(1, "button", "Sign in"), regEl(2, "button", "Checkout"),
		},
	}}
	opener.opens = map[int]*tabbedMock{1: popup}
	opener.tabs = map[string]*tabbedMock{"popup": popup}

	err := runOn(t, opener, "CLICK the 'Sign in' button\nCLICK the 'Authorize' button\nCLICK the 'Checkout' button\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(popup.Clicks) != 1 {
		t.Errorf("popup got %d clicks, want 1 (Authorize)", len(popup.Clicks))
	}
	if len(opener.Clicks) != 2 {
		t.Errorf("opener got %d clicks, want 2 (Sign in, then Checkout after the popup closed)", len(opener.Clicks))
	}
}

// A tab already open when the hunt began was not opened by any of its steps.
func TestTabs_ATabOpenBeforeTheHuntIsNotFollowed(t *testing.T) {
	old := &tabbedMock{id: "old", MockPage: &MockPage{URL: "https://shop.test/old"}}
	page := &tabbedMock{id: "main", MockPage: &MockPage{
		URL:      "https://shop.test/",
		Elements: []dom.ElementSnapshot{regEl(1, "button", "Checkout")},
	}}
	page.opens = map[int]*tabbedMock{0: old}
	page.tabs = map[string]*tabbedMock{"old": old}

	if err := runOn(t, page, "CLICK the 'Checkout' button\nCLICK the 'Checkout' button\n"); err != nil {
		t.Fatal(err)
	}
	if len(page.Clicks) != 2 {
		t.Errorf("page got %d clicks, want 2", len(page.Clicks))
	}
}

// Following a tab is automatic; going back to one that is still open is not,
// and before SWITCH TO TAB there was no way to.
func TestTabs_SwitchToTabByNumberAndByURL(t *testing.T) {
	product := &tabbedMock{id: "product", MockPage: &MockPage{
		URL:      "https://shop.test/product/42",
		Elements: []dom.ElementSnapshot{regEl(1, "button", "Add to cart")},
	}}
	front := &tabbedMock{id: "main", MockPage: &MockPage{
		URL:      "https://shop.test/",
		Elements: []dom.ElementSnapshot{regEl(1, "a", "Open product"), regEl(2, "button", "Checkout")},
	}}
	front.opens = map[int]*tabbedMock{1: product}
	front.tabs = map[string]*tabbedMock{"product": product}

	src := "CLICK the 'Open product' link\n" +
		"CLICK the 'Add to cart' button\n" +
		"SWITCH TO TAB 1\n" +
		"CLICK the 'Checkout' button\n" +
		"SWITCH TO TAB 'product/42'\n" +
		"CLICK the 'Add to cart' button\n"
	if err := runOn(t, front, src); err != nil {
		t.Fatal(err)
	}
	if len(front.Clicks) != 2 || len(product.Clicks) != 2 {
		t.Errorf("front got %d clicks, product %d; want 2 and 2", len(front.Clicks), len(product.Clicks))
	}
}

func TestTabs_SwitchToATabThatIsNotThereFails(t *testing.T) {
	page := &tabbedMock{id: "main", MockPage: &MockPage{URL: "https://shop.test/"}}
	for src, want := range map[string]string{
		"SWITCH TO TAB 2\n":          "there are 1 open tab(s)",
		"SWITCH TO TAB 'checkout'\n": `no open tab has "checkout"`,
		"SWITCH TO TAB next\n":       "want a tab number",
	} {
		if err := runOn(t, page, src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want an error containing %q", src, err, want)
		}
	}
}
