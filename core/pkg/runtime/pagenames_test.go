package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexbeatnik/manul-browser/core/pkg/browser"
	"github.com/alexbeatnik/manul-browser/core/pkg/config"
	"github.com/alexbeatnik/manul-browser/core/pkg/dom"
	"github.com/alexbeatnik/manul-browser/core/pkg/dsl"
	"github.com/alexbeatnik/manul-browser/core/pkg/pages"
	"github.com/alexbeatnik/manul-browser/core/pkg/utils"
)

// runWithPages runs src against page with a pages/ registry made of the
// given fragment.
func runWithPages(t *testing.T, page *MockPage, fragment, src string) error {
	t.Helper()
	dir := t.TempDir()
	if fragment != "" {
		if err := os.WriteFile(filepath.Join(dir, "site.json"), []byte(fragment), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hunt, err := dsl.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))
	rt.pages = pages.NewRegistry(dir)
	_, err = rt.RunHunt(context.Background(), hunt)
	return err
}

const shopPages = `{"site": "https://shop.test/", "Domain": "Shop", "/login": "Login Page", "/cart": "Cart"}`

func TestVerifyPage_ByItsNameInPagesOrItsTitle(t *testing.T) {
	page := &MockPage{URL: "https://shop.test/login?next=/", Title: "Sign in · Shop"}
	for step, pass := range map[string]bool{
		"VERIFY that the page is 'Login Page'":     true,
		"VERIFY that the page is 'login  page'":    true, // as a person reads it
		"VERIFY the page is 'Sign in · Shop'":      true, // the title also names it
		"VERIFY that the page is 'Cart'":           false,
		"VERIFY that the page is NOT 'Cart'":       true,
		"VERIFY that the page is NOT 'Login Page'": false,
	} {
		err := runWithPages(t, page, shopPages, step+"\n")
		if (err == nil) != pass {
			t.Errorf("%s: err = %v, want pass=%v", step, err, pass)
		}
	}
	// A failure says what the page is.
	err := runWithPages(t, page, shopPages, "VERIFY that the page is 'Cart'\n")
	if err == nil || !strings.Contains(err.Error(), "'Login Page' in pages/") || !strings.Contains(err.Error(), "titled 'Sign in · Shop'") {
		t.Errorf("failure = %v", err)
	}
	// Without a registry, the title is the name.
	if err := runWithPages(t, page, "", "VERIFY that the page is 'Sign in · Shop'\n"); err != nil {
		t.Errorf("by title alone: %v", err)
	}
}

func TestNavigate_ToAPageByItsName(t *testing.T) {
	page := &MockPage{URL: "https://shop.test/"}
	if err := runWithPages(t, page, shopPages, "NAVIGATE to 'Cart'\n"); err != nil {
		t.Fatal(err)
	}
	if page.LastNavigate != "https://shop.test/cart" {
		t.Errorf("navigated to %q, want the address pages/ gives 'Cart'", page.LastNavigate)
	}
	// An address is still an address.
	if err := runWithPages(t, page, shopPages, "NAVIGATE to https://shop.test/login\n"); err != nil || page.LastNavigate != "https://shop.test/login" {
		t.Errorf("an address: %v %q", err, page.LastNavigate)
	}
	// A name nobody gave is not an address either.
	err := runWithPages(t, page, shopPages, "NAVIGATE to 'Order History'\n")
	if err == nil || !strings.Contains(err.Error(), "no page in pages/ has that name") {
		t.Errorf("an unknown name: %v", err)
	}
}

// The same page on localhost and on staging: the site open now decides.
func TestNavigate_ANameOnTwoSitesGoesToTheOneOpen(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "local.json"), []byte(`{"site": "http://localhost:8000/", "/login": "Login Page"}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "staging.json"), []byte(`{"site": "https://staging.test/", "/login": "Login Page"}`), 0o644)
	for _, c := range []struct{ open, want string }{
		{"https://staging.test/cart", "https://staging.test/login"},
		{"http://localhost:8000/", "http://localhost:8000/login"},
	} {
		page := &MockPage{URL: c.open}
		hunt, _ := dsl.Parse(strings.NewReader("NAVIGATE to 'Login Page'\n"))
		rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))
		rt.pages = pages.NewRegistry(dir)
		if _, err := rt.RunHunt(context.Background(), hunt); err != nil || page.LastNavigate != c.want {
			t.Errorf("from %s: %v, went to %q, want %q", c.open, err, page.LastNavigate, c.want)
		}
	}
	page := &MockPage{URL: "about:blank"}
	hunt, _ := dsl.Parse(strings.NewReader("NAVIGATE to 'Login Page'\n"))
	rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))
	rt.pages = pages.NewRegistry(dir)
	if _, err := rt.RunHunt(context.Background(), hunt); err == nil || !strings.Contains(err.Error(), "several addresses") {
		t.Errorf("from a blank page: %v", err)
	}
}

func TestCondition_ThePageIs(t *testing.T) {
	page := &MockPage{URL: "https://shop.test/login", Title: "Sign in"}
	src := "IF the page is 'Login Page':\n    SET {was} = login\nIF page is not 'Cart':\n    SET {not_cart} = yes\nIF the page is 'Cart':\n    SET {cart} = yes\n"
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "site.json"), []byte(shopPages), 0o644)
	hunt, err := dsl.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))
	rt.pages = pages.NewRegistry(dir)
	if _, err := rt.RunHunt(context.Background(), hunt); err != nil {
		t.Fatal(err)
	}
	v := rt.Vars()
	if v["was"] != "login" || v["not_cart"] != "yes" || v["cart"] != "" {
		t.Errorf("branches taken: was=%q not_cart=%q cart=%q", v["was"], v["not_cart"], v["cart"])
	}
}

// A step's result carries the page's name.
func TestStepResult_CarriesThePageName(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "site.json"), []byte(shopPages), 0o644)
	page := &MockPage{URL: "https://shop.test/cart"}
	rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))
	rt.pages = pages.NewRegistry(dir)
	hunt, _ := dsl.Parse(strings.NewReader("PRINT \"hi\"\n"))
	res, err := rt.RunHunt(context.Background(), hunt)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) == 0 || res.Results[0].PageName != "Cart" {
		t.Errorf("results = %+v", res.Results)
	}
}

// NAVIGATE to a page of the any-site fragment goes to it on the site open
// now, or from a blank page on the hunt's {site}.
func TestNavigate_APageOnAnySite(t *testing.T) {
	frag := `{"site": "*", "/login.html": "Login"}`
	page := &MockPage{URL: "https://staging.test/shop.html"}
	if err := runWithPages(t, page, frag, "NAVIGATE to 'Login'\n"); err != nil || page.LastNavigate != "https://staging.test/login.html" {
		t.Errorf("from staging: %v, went to %q", err, page.LastNavigate)
	}
	blank := &MockPage{URL: "about:blank"}
	if err := runWithPages(t, blank, frag, "@var: {site} = http://localhost:8000\nNAVIGATE to 'Login'\n"); err != nil || blank.LastNavigate != "http://localhost:8000/login.html" {
		t.Errorf("from a blank page with {site}: %v, went to %q", err, blank.LastNavigate)
	}
	none := &MockPage{URL: "about:blank"}
	if err := runWithPages(t, none, frag, "NAVIGATE to 'Login'\n"); err == nil || !strings.Contains(err.Error(), "give the hunt {site}") {
		t.Errorf("from a blank page without {site}: %v", err)
	}
}

// A page a page file describes is open when what it @expects is on it: two
// views a single-page app keeps at one address are told apart that way.
func TestVerifyPage_APageFilesExpectations(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "login.hunt"), []byte("@page: Login\n@url: /login.html\n@expect: 'Sign in'\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "settings.hunt"), []byte("@page: Settings\n@expect: 'Account settings', 'Save changes'\n"), 0o644)
	run := func(page *MockPage, step string) error {
		hunt, err := dsl.Parse(strings.NewReader(step + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))
		rt.pages = pages.NewRegistry(dir)
		_, err = rt.RunHunt(context.Background(), hunt)
		return err
	}
	login := &MockPage{URL: "https://shop.test/login.html", Elements: []dom.ElementSnapshot{regEl(1, "h1", "Sign in")}}
	if err := run(login, "VERIFY that the page is 'Login'"); err != nil {
		t.Errorf("the login page: %v", err)
	}
	notYet := &MockPage{URL: "https://shop.test/login.html", Elements: []dom.ElementSnapshot{regEl(1, "p", "Loading…")}}
	if err := run(notYet, "VERIFY that the page is 'Login'"); err == nil || !strings.Contains(err.Error(), "'Sign in', which its page file expects, is not on it") {
		t.Errorf("the address without the text: %v", err)
	}
	elsewhere := &MockPage{URL: "https://shop.test/cart.html", Elements: []dom.ElementSnapshot{regEl(1, "h1", "Sign in")}}
	if err := run(elsewhere, "VERIFY that the page is 'Login'"); err == nil {
		t.Error("the text at another address passed for the page")
	}
	// No @url: the view is what it shows, at any address.
	view := &MockPage{URL: "https://app.test/#/", Elements: []dom.ElementSnapshot{regEl(1, "h1", "Account settings"), regEl(2, "button", "Save changes")}}
	if err := run(view, "VERIFY that the page is 'Settings'"); err != nil {
		t.Errorf("a view by what it shows: %v", err)
	}
	if err := run(view, "VERIFY that the page is NOT 'Login'"); err != nil {
		t.Errorf("NOT: %v", err)
	}
}

// A custom control is registered under a page's name in pages/ as well as
// under its title. It was found by the title (or a label made from the URL)
// alone, so a page renamed in the registry could not be told apart there.
func TestCustomControl_ByThePagesNameInPages(t *testing.T) {
	ResetRuntimeRegistries()
	t.Cleanup(ResetRuntimeRegistries)
	var gotPage, which string
	_ = RegisterCustomControl("Payment", "Card", func(_ context.Context, _ browser.Page, inv CustomControlInvocation) error {
		gotPage, which = inv.Page, "named"
		return nil
	})
	_ = RegisterCustomControl("*", "Card", func(_ context.Context, _ browser.Page, inv CustomControlInvocation) error {
		which = "any page"
		return nil
	})
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "pay.hunt"), []byte("@page: Payment\n@url: /pay.html\n"), 0o644)
	mock := &MockPage{URL: "https://shop.test/pay.html", Title: "Pay — Shop"}
	rt := New(config.Config{}, mock, utils.NewLogger(nil))
	rt.pages = pages.NewRegistry(dir)
	if _, err := rt.executeCommand(context.Background(), dsl.Command{Type: dsl.CmdFill, Raw: "FILL 'Card' field with '4242'", Target: "Card", Value: "4242"}); err != nil {
		t.Fatal(err)
	}
	if which != "named" || gotPage != "Payment" {
		t.Errorf("control found = %q under %q, want the one registered under the page's name", which, gotPage)
	}

	// A page pages/ does not name falls back as before: its title, then "*".
	mock.URL = "https://shop.test/other.html"
	which = ""
	if _, err := rt.executeCommand(context.Background(), dsl.Command{Type: dsl.CmdFill, Raw: "FILL 'Card' field with '1'", Target: "Card", Value: "1"}); err != nil {
		t.Fatal(err)
	}
	if which != "any page" {
		t.Errorf("on an unnamed page: %q, want the any-page control", which)
	}
}

// A page file with several addresses and patterns: NAVIGATE goes to its
// first address, VERIFY knows it at any of them, and a page known by a
// pattern alone has nowhere to go — NAVIGATE to 'Book' went to "Book".
func TestNavigateAndVerify_APageWithSeveralAddressesAndPatterns(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "catalogue.hunt"), []byte("@page: Catalogue\n@url: /index.html / /catalogue/page-*.html\n@expect: 'All products'\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "book.hunt"), []byte("@page: Book\n@url: /catalogue/*/index.html\n@expect: 'Product Information'\n"), 0o644)
	run := func(page *MockPage, src string) error {
		hunt, err := dsl.Parse(strings.NewReader(src))
		if err != nil {
			t.Fatal(err)
		}
		rt := New(regConfig(), page, utils.NewLoggerTo(nopWriter{}, nil))
		rt.pages = pages.NewRegistry(dir)
		_, err = rt.RunHunt(context.Background(), hunt)
		return err
	}
	home := &MockPage{URL: "https://books.test/catalogue/olio_984/index.html"}
	if err := run(home, "NAVIGATE to 'Catalogue'\n"); err != nil || home.LastNavigate != "https://books.test/index.html" {
		t.Errorf("NAVIGATE to 'Catalogue': %v, went to %q", err, home.LastNavigate)
	}
	for _, url := range []string{"https://books.test/", "https://books.test/catalogue/page-3.html"} {
		at := &MockPage{URL: url, Elements: []dom.ElementSnapshot{regEl(1, "h1", "All products")}}
		if err := run(at, "VERIFY that the page is 'Catalogue'\n"); err != nil {
			t.Errorf("%s: %v", url, err)
		}
	}
	book := &MockPage{URL: "https://books.test/catalogue/olio_984/index.html", Elements: []dom.ElementSnapshot{regEl(1, "h2", "Product Information")}}
	if err := run(book, "VERIFY that the page is 'Book'\nVERIFY that the page is NOT 'Catalogue'\n"); err != nil {
		t.Errorf("a book: %v", err)
	}
	err := run(&MockPage{URL: "https://books.test/"}, "NAVIGATE to 'Book'\n")
	if err == nil || !strings.Contains(err.Error(), "by pattern only (/catalogue/*/index.html)") {
		t.Errorf("NAVIGATE to 'Book': %v", err)
	}
}
