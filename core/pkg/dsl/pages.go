package dsl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A page file is a library hunt that says which page it is:
//
//	@page: Login
//	@url: /login.html
//	@expect: 'Sign in'
//	@export: SignIn
//
//	STEP SignIn:
//	    FILL 'Username' field with '{user}'
//	    …
//
// It lives in pages/ beside the registry's JSON fragments. Its name and
// address join the registry, so NAVIGATE to 'Login' and VERIFY that the page
// is 'Login' know it; its @expect lines are what VERIFY looks for to tell the
// page is open, which is how two views a single-page app keeps at one address
// are told apart; and its blocks are the page's actions, used from any hunt
// as USE Login.SignIn with no @import.

// PagesDir is where page files are looked for: pages/ under the working
// directory, as the registry is.
var PagesDir = "pages"

// PageHeader is what a page file says about the page.
type PageHeader struct {
	// Name is @page.
	Name string
	// URL is @url: a path ("/login.html", "{site}/login.html") or a whole
	// address.
	URL string
	// Expect is every @expect: text the page shows when it is open.
	Expect []string
	// Path is the file.
	Path string
}

// ReadPageHeaders reads every page file in dir — a .hunt with @page: — and
// returns what each says, sorted by file. A file that does not parse, or is
// not a page, is left out.
func ReadPageHeaders(dir string) []PageHeader {
	var out []PageHeader
	for _, h := range loadPages(dir) {
		out = append(out, PageHeader{Name: h.Page, URL: h.PageURL, Expect: h.PageExpect, Path: h.SourcePath})
	}
	return out
}

func loadPages(dir string) []*Hunt {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".hunt") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var out []*Hunt
	for _, n := range names {
		h, err := parseLibrary(filepath.Join(dir, n))
		if err != nil || strings.TrimSpace(h.Page) == "" {
			continue
		}
		out = append(out, h)
	}
	return out
}

// resolvePageUses makes USE <Page>.<Block> work without an @import: the
// block is taken from the page file whose @page is <Page>. Each of that
// page's blocks is added as <page>.<block>, and a USE inside one of them of
// another block of the same page is made to name it the same way, so a
// page's actions may use each other as they are written in the file.
func resolvePageUses(hunt *Hunt) error {
	wanted := map[string]string{} // lower-cased qualified name → as written
	var walk func(cmds []Command)
	walk = func(cmds []Command) {
		for _, c := range cmds {
			if (c.Type == CmdUse || c.Type == CmdCallStep) && strings.Contains(c.CallStepName, ".") {
				wanted[strings.ToLower(c.CallStepName)] = c.CallStepName
			}
			walk(c.Body)
			for _, b := range c.Branches {
				walk(b.Body)
			}
		}
	}
	walk(hunt.Commands)
	walk(hunt.SetupCommands)
	walk(hunt.TeardownCommands)
	for _, cmds := range hunt.Blueprints {
		walk(cmds)
	}

	var pages []*Hunt
	loaded := false
	for key, written := range wanted {
		if _, ok := hunt.Blueprints[key]; ok {
			continue
		}
		if !loaded {
			pages, loaded = loadPages(PagesDir), true
		}
		pageName, block, _ := strings.Cut(written, ".")
		var page *Hunt
		for _, p := range pages {
			if strings.EqualFold(strings.TrimSpace(p.Page), strings.TrimSpace(pageName)) {
				page = p
				break
			}
		}
		if page == nil {
			return fmt.Errorf("USE %s: no page in %s/ has @page: %s", written, PagesDir, pageName)
		}
		blocks := extractStepBlocks(page.Commands)
		if _, ok := blocks[strings.ToLower(strings.TrimSpace(block))]; !ok {
			return fmt.Errorf("USE %s: the page %q (%s) has no block %q", written, page.Page, page.SourcePath, block)
		}
		if hunt.Blueprints == nil {
			hunt.Blueprints = map[string][]Command{}
		}
		if hunt.ImportVars == nil {
			hunt.ImportVars = map[string]string{}
		}
		prefix := strings.ToLower(strings.TrimSpace(page.Page)) + "."
		for name, cmds := range blocks {
			hunt.Blueprints[prefix+name] = qualifyUses(cmds, prefix, blocks)
		}
		inheritVars(hunt.ImportVars, page.Vars)
	}
	return nil
}

// qualifyUses copies cmds with every USE of one of the page's own blocks
// written as <page>.<block>.
func qualifyUses(cmds []Command, prefix string, own map[string][]Command) []Command {
	out := make([]Command, len(cmds))
	for i, c := range cmds {
		if (c.Type == CmdUse || c.Type == CmdCallStep) && !strings.Contains(c.CallStepName, ".") {
			if _, ok := own[strings.ToLower(c.CallStepName)]; ok {
				c.CallStepName = prefix + strings.ToLower(c.CallStepName)
			}
		}
		if len(c.Body) > 0 {
			c.Body = qualifyUses(c.Body, prefix, own)
		}
		if len(c.Branches) > 0 {
			branches := make([]Branch, len(c.Branches))
			copy(branches, c.Branches)
			for j := range branches {
				branches[j].Body = qualifyUses(branches[j].Body, prefix, own)
			}
			c.Branches = branches
		}
		out[i] = c
	}
	return out
}

// quotedOrWhole is every quoted text in s, or s itself when it quotes none:
// @expect: 'Sign in', 'Username' or @expect: Sign in.
func quotedOrWhole(s string) []string {
	var out []string
	rest := s
	for {
		q := extractFirstQuoted(rest)
		if q == "" {
			break
		}
		out = append(out, q)
		i := strings.Index(rest, q)
		rest = rest[i+len(q)+1:]
	}
	if len(out) == 0 {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}
