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
