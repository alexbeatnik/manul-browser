package pages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistry_LookupExactMatch(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{
    "site": "https://example.com/",
    "Domain": "Example",
    "https://example.com/login": "Login Page",
    "https://example.com/dashboard": "Dashboard"
}`)
	_ = os.WriteFile(filepath.Join(dir, "example.com.json"), data, 0644)

	r := NewRegistry(dir)

	if got := r.LookupPageName("https://example.com/login"); got != "Login Page" {
		t.Errorf("login = %q, want Login Page", got)
	}
	if got := r.LookupPageName("https://example.com/dashboard"); got != "Dashboard" {
		t.Errorf("dashboard = %q, want Dashboard", got)
	}
}

func TestRegistry_LookupRegexMatch(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{
    "site": "https://shop.com/",
    "Domain": "Shop",
    ".*/products/\\d+": "Product Detail Page"
}`)
	_ = os.WriteFile(filepath.Join(dir, "shop.com.json"), data, 0644)

	r := NewRegistry(dir)

	got := r.LookupPageName("https://shop.com/products/42")
	if got != "Product Detail Page" {
		t.Errorf("regex match = %q, want Product Detail Page", got)
	}
}

func TestRegistry_LookupDomainFallback(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{
    "site": "https://example.com/",
    "Domain": "Example Domain"
}`)
	_ = os.WriteFile(filepath.Join(dir, "example.com.json"), data, 0644)

	r := NewRegistry(dir)

	got := r.LookupPageName("https://example.com/unknown-page")
	if got != "Example Domain" {
		t.Errorf("domain fallback = %q, want Example Domain", got)
	}
}

func TestRegistry_AutoPopulate(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	got := r.LookupPageName("https://brand-new-site.io/dashboard")
	if !strings.HasPrefix(got, "Auto:") {
		t.Errorf("expected Auto: prefix, got %q", got)
	}

	// File should have been created.
	frag := filepath.Join(dir, "brand-new-site.io.json")
	if _, err := os.Stat(frag); os.IsNotExist(err) {
		t.Fatalf("auto-populate did not create fragment %s", frag)
	}

	// Second lookup should return the same placeholder without creating duplicates.
	got2 := r.LookupPageName("https://brand-new-site.io/dashboard")
	if got2 != got {
		t.Errorf("second lookup = %q, want %q", got2, got)
	}
}

func TestRegistry_WrappedForm(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{
    "https://wrapped.com/": {
        "Domain": "Wrapped",
        "/admin": "Admin Page"
    }
}`)
	_ = os.WriteFile(filepath.Join(dir, "wrapped.com.json"), data, 0644)

	r := NewRegistry(dir)
	if got := r.LookupPageName("https://wrapped.com/admin"); got != "Admin Page" {
		t.Errorf("wrapped form = %q, want Admin Page", got)
	}
}

func TestRegistry_LongestPrefixWins(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "example.com.json"), []byte(`{
    "site": "https://example.com/",
    "Domain": "Root"
}`), 0644)
	_ = os.WriteFile(filepath.Join(dir, "app.example.com.json"), []byte(`{
    "site": "https://app.example.com/",
    "Domain": "App"
}`), 0644)

	r := NewRegistry(dir)
	if got := r.LookupPageName("https://app.example.com/dashboard"); got != "App" {
		t.Errorf("longest prefix = %q, want App", got)
	}
	if got := r.LookupPageName("https://example.com/about"); got != "Root" {
		t.Errorf("root prefix = %q, want Root", got)
	}
}

func TestSafeSiteFilename(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://www.saucedemo.com/", "www.saucedemo.com.json"},
		{"https://example.com/", "example.com.json"},
		{"https://sub.domain.co.uk/", "sub.domain.co.uk.json"},
	}
	for _, tc := range tests {
		if got := safeSiteFilename(tc.in); got != tc.want {
			t.Errorf("safeSiteFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBelongsToSite(t *testing.T) {
	tests := []struct {
		url  string
		site string
		want bool
	}{
		{"https://example.com/login", "https://example.com/", true},
		{"https://example.com/login", "https://other.com/", false},
		{"https://example.com/api/v1", "https://example.com/api/", true},
		{"https://example.com/", "https://example.com/api/", false},
	}
	for _, tc := range tests {
		if got := belongsToSite(tc.url, tc.site); got != tc.want {
			t.Errorf("belongsToSite(%q, %q) = %v, want %v", tc.url, tc.site, got, tc.want)
		}
	}
}

func TestNormalizeFragment_Lean(t *testing.T) {
	raw := map[string]any{
		"site":   "https://example.com/",
		"Domain": "Example",
		"/login": "Login",
	}
	got := normalizeFragment(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 site, got %d", len(got))
	}
	if got["https://example.com/"]["/login"] != "Login" {
		t.Errorf("login = %q", got["https://example.com/"]["/login"])
	}
}

func TestNormalizeFragment_Wrapped(t *testing.T) {
	raw := map[string]any{
		"https://example.com/": map[string]any{
			"Domain": "Example",
			"/login": "Login",
		},
	}
	got := normalizeFragment(raw)
	if len(got) != 1 {
		t.Fatalf("expected 1 site, got %d", len(got))
	}
	if got["https://example.com/"]["/login"] != "Login" {
		t.Errorf("login = %q", got["https://example.com/"]["/login"])
	}
}

// Two patterns that both matched one address named it at random: Go walks a
// map in no fixed order, and /login/sso was "Login" 156 times in 200 and
// "Single sign-on" 44. The most specific match wins now, every time.
func TestRegistry_TheMostSpecificPatternWins(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "shop.test.json"), []byte(`{
    "site": "https://shop.test/",
    "Domain": "Shop",
    "/login": "Login",
    "/login/sso": "Single sign-on"
}`), 0o644)
	r := NewRegistry(dir)
	for i := 0; i < 100; i++ {
		if got := r.LookupPageName("https://shop.test/login/sso"); got != "Single sign-on" {
			t.Fatalf("lookup %d: /login/sso = %q, want Single sign-on", i, got)
		}
	}
	if got := r.LookupPageName("https://shop.test/login?next=/cart"); got != "Login" {
		t.Errorf("/login?next=… = %q, want Login", got)
	}
}

// A path key was a substring of the whole address: '/admin' named a blog
// post about admin tips. It matches the path from its start, a segment at a
// time.
func TestRegistry_APathKeyMatchesFromTheStartOfThePath(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "site.test.json"), []byte(`{"site": "https://site.test/", "Domain": "Site", "/admin": "Admin", "/index.html": "Home"}`), 0o644)
	r := NewRegistry(dir)
	for url, want := range map[string]string{
		"https://site.test/admin":           "Admin",
		"https://site.test/admin/users":     "Admin",
		"https://site.test/blog/admin-tips": "Site",
		"https://site.test/administrators":  "Site",
		"https://site.test/index.html?x=1":  "Home",
		"https://site.test/indexXhtml":      "Site", // the dot is a dot
	} {
		if got := r.LookupPageName(url); got != want {
			t.Errorf("%s = %q, want %q", url, got, want)
		}
	}
}

// An exact address is the same page with a query or a fragment after it.
func TestRegistry_TheQueryAndFragmentDoNotChangeThePage(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "shop.test.json"), []byte(`{"site": "https://shop.test/", "Domain": "Shop", "https://shop.test/cart": "Cart"}`), 0o644)
	r := NewRegistry(dir)
	if got := r.LookupPageName("https://shop.test/cart?step=2#top"); got != "Cart" {
		t.Errorf("cart with a query = %q, want Cart", got)
	}
}

// Auto-populate named the whole site after the first address it saw, query
// and all — "Auto: other.test/p/1?ref=a" — and every other page of the site
// was then called that.
func TestRegistry_AutoPopulateNamesTheSiteAfterItsHost(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)
	if got := r.LookupPageName("https://other.test/p/1?ref=a"); got != "Auto: other.test/p/1" {
		t.Errorf("first page = %q, want Auto: other.test/p/1", got)
	}
	if got := r.LookupPageName("https://other.test/p/2"); got != "Auto: other.test" {
		t.Errorf("another page of the site = %q, want the site's name Auto: other.test", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, "other.test.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "?") {
		t.Errorf("the fragment keeps a query:\n%s", data)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("the directory holds %d files, want the one fragment: a write left a temporary behind", len(entries))
	}
}

// A step asks for its page's name: the name a person gave, never a
// placeholder, and nothing is written to find out.
func TestRegistry_PageNameWritesNothingAndIgnoresPlaceholders(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "shop.test.json"), []byte(`{"site": "https://shop.test/", "Domain": "Shop", "/login": "Login Page", "/new": "Auto: shop.test/new"}`), 0o644)
	r := NewRegistry(dir)
	for url, want := range map[string]string{
		"https://shop.test/login?next=/": "Login Page",
		"https://shop.test/other":        "Shop",
		"https://shop.test/new":          "",
		"https://unknown.test/":          "",
		"about:blank":                    "",
	} {
		if got := r.PageName(url); got != want {
			t.Errorf("PageName(%s) = %q, want %q", url, got, want)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("PageName wrote to the registry: %d files", len(entries))
	}
}

// NAVIGATE to 'Login Page' goes where the registry says the page is.
func TestRegistry_PagesNamed(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "shop.test.json"), []byte(`{"site": "https://shop.test/", "Domain": "Shop", "/login": "Login Page", "https://shop.test/cart": "Cart", ".*/p/\\d+": "Product"}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "staging.json"), []byte(`{"site": "https://staging.shop.test/", "/login": "login  page"}`), 0o644)
	r := NewRegistry(dir)
	got := r.PagesNamed("Login Page")
	if strings.Join(got, " ") != "https://shop.test/login https://staging.shop.test/login" {
		t.Errorf("Login Page = %v", got)
	}
	if got := r.PagesNamed("cart"); len(got) != 1 || got[0] != "https://shop.test/cart" {
		t.Errorf("Cart = %v", got)
	}
	if got := r.PagesNamed("Shop"); len(got) != 1 || got[0] != "https://shop.test/" {
		t.Errorf("Shop (the site's name) = %v", got)
	}
	if got := r.PagesNamed("Product"); len(got) != 0 {
		t.Errorf("a pattern names many pages and so none to go to: %v", got)
	}
	if !SameSite("https://shop.test/a?b", "https://shop.test/") || SameSite("https://shop.test/", "https://staging.shop.test/") {
		t.Error("SameSite")
	}
}

// Reading the fragments for every step is waste when none changed; a change
// is still seen.
func TestRegistry_ReloadSeesAChangeAndOnlyThen(t *testing.T) {
	dir := t.TempDir()
	frag := filepath.Join(dir, "shop.test.json")
	_ = os.WriteFile(frag, []byte(`{"site": "https://shop.test/", "/a": "One"}`), 0o644)
	r := NewRegistry(dir)
	if got := r.PageName("https://shop.test/a"); got != "One" {
		t.Fatalf("PageName = %q", got)
	}
	_ = os.WriteFile(frag, []byte(`{"site": "https://shop.test/", "/a": "Two, renamed"}`), 0o644)
	if got := r.PageName("https://shop.test/a"); got != "Two, renamed" {
		t.Errorf("after an edit: %q", got)
	}
}

// A suite run against localhost and staging names its pages once, by path,
// in a fragment for any site; a fragment for a site of its own comes first.
func TestRegistry_AFragmentForAnySite(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "app.json"), []byte(`{"site": "*", "/login.html": "Login", "/shop.html": "Shop"}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "admin.json"), []byte(`{"site": "https://admin.test/", "/login.html": "Admin login"}`), 0o644)
	r := NewRegistry(dir)
	for url, want := range map[string]string{
		"http://localhost:8000/login.html":   "Login",
		"https://staging.test/shop.html?x=1": "Shop",
		"https://admin.test/login.html":      "Admin login",
		"about:blank":                        "",
	} {
		if got := r.PageName(url); got != want {
			t.Errorf("PageName(%s) = %q, want %q", url, got, want)
		}
	}
	if got := r.PagesNamed("Login"); len(got) != 1 || got[0] != "/login.html" {
		t.Errorf("PagesNamed(Login) = %v, want the path for the caller to put on a site", got)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Errorf("a site the any-site fragment covers was auto-populated: %d files", len(entries))
	}
}
