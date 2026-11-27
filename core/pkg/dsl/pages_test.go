package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const loginPage = `@page: Login
@url: /login.html
@expect: 'Sign in', 'Username'
@export: SignIn, Open
@var: {user} = ada

STEP Open:
    NAVIGATE to 'Login'

STEP SignIn:
    USE Open
    FILL 'Username' field with '{user}'
    CLICK the 'Sign in' button
`

func withPages(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	pages := filepath.Join(dir, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(pages, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := PagesDir
	PagesDir = pages
	t.Cleanup(func() { PagesDir = old })
	return dir
}

func TestPageHeaders(t *testing.T) {
	withPages(t, map[string]string{"login.hunt": loginPage, "notes.hunt": "@title: not a page\nSTEP 1: X\n    PRINT \"x\"\n"})
	got := ReadPageHeaders(PagesDir)
	if len(got) != 1 {
		t.Fatalf("headers = %+v, want the one page", got)
	}
	h := got[0]
	if h.Name != "Login" || h.URL != "/login.html" || strings.Join(h.Expect, "|") != "Sign in|Username" {
		t.Errorf("header = %+v", h)
	}
}

// A page's actions are used from any hunt as USE <Page>.<Block>, with no
// @import, and a block of the page that uses another of its own still finds
// it.
func TestUse_APagesBlockByThePagesName(t *testing.T) {
	dir := withPages(t, map[string]string{"login.hunt": loginPage})
	path := filepath.Join(dir, "t.hunt")
	_ = os.WriteFile(path, []byte("STEP 1: In\n    USE Login.SignIn\n    VERIFY that 'Welcome, {user}' is present\nDONE.\n"), 0o644)
	h, err := ParseFile(path)
	if err == nil {
		err = ResolveImports(h)
	}
	if err == nil {
		err = h.Expand()
	}
	if err != nil {
		t.Fatal(err)
	}
	var steps []string
	for _, c := range h.Commands {
		steps = append(steps, c.Raw)
	}
	want := "NAVIGATE to 'Login' | FILL 'Username' field with '{user}' | CLICK the 'Sign in' button | VERIFY that 'Welcome, {user}' is present"
	if got := strings.Join(steps, " | "); got != want {
		t.Errorf("expanded:\n %s\nwant\n %s", got, want)
	}
	if h.ImportVars["user"] != "ada" {
		t.Errorf("the page's @var is not a default: %v", h.ImportVars)
	}
}

func TestUse_APageThatIsNotThere(t *testing.T) {
	dir := withPages(t, map[string]string{"login.hunt": loginPage})
	for src, want := range map[string]string{
		"STEP 1: X\n    USE Checkout.Pay\n":   "no page in",
		"STEP 1: X\n    USE Login.Register\n": `has no block "Register"`,
	} {
		path := filepath.Join(dir, "t.hunt")
		_ = os.WriteFile(path, []byte(src), 0o644)
		h, _ := ParseFile(path)
		if err := ResolveImports(h); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want %q", src, err, want)
		}
	}
}
