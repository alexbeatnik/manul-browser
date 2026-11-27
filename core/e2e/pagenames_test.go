//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Page names from pages/ are usable in a hunt: NAVIGATE to one, VERIFY the
// page is one, IF the page is one; and every step's result says which page
// it ended on.
func TestPageNames_InAHunt(t *testing.T) {
	o := &origins{}
	o.site = serve(t, o)
	names, required := browsers()
	for _, browser := range names {
		t.Run(browser, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
				t.Fatal(err)
			}
			fragment := `{"site": "` + o.site + `/", "Domain": "Fixtures", "/extract.html": "Order details", "/spa.html": "Search page"}`
			if err := os.WriteFile(filepath.Join(dir, "pages", "fixtures.json"), []byte(fragment), 0o644); err != nil {
				t.Fatal(err)
			}
			// A view of a single-page app, known by what it shows: spa.html
			// is at one address before and after its search.
			results := "@page: Results\n@expect: 'Manul in the snow'\n@export: Search\n\nSTEP Search:\n    FILL 'Search' field with '{query}'\n    PRESS ENTER\n"
			if err := os.WriteFile(filepath.Join(dir, "pages", "results.hunt"), []byte(results), 0o644); err != nil {
				t.Fatal(err)
			}
			hunt := `@title: page names
STEP 1: By name
    NAVIGATE to ` + o.site + `/checkboxes.html
    VERIFY that the page is 'Fixtures'
    NAVIGATE to 'Order details'
    VERIFY that the page is 'Order details'
    VERIFY that the page is 'Order summary'
    VERIFY that the page is NOT 'Search page'
    EXTRACT the 'Price' into {price}
STEP 2: A page's name in a condition
    NAVIGATE to 'Search page'
    IF the page is 'Search page':
        PRINT "on=[search]"
    IF the page is 'Order details':
        PRINT "on=[order]"
STEP 3: A view known by what it shows, and its page's action
    VERIFY that the page is NOT 'Results'
    SET {query} = manul
    USE Results.Search
    VERIFY that the page is 'Results'
DONE.
`
			if err := os.WriteFile(filepath.Join(dir, "names.hunt"), []byte(hunt), 0o644); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), huntTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "run", "names.hunt", "--headless", "--browser", browser, "--json")
			cmd.Dir = dir
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			logs := stderr.String()
			if !required && (strings.Contains(logs, "chrome not found") || strings.Contains(logs, "firefox not found")) {
				t.Skipf("%s is not installed", browser)
			}
			if err != nil {
				t.Fatalf("hunt failed: %v\n%s", err, logs)
			}
			if !strings.Contains(stdout.String()+logs, "on=[search]") || strings.Contains(stdout.String()+logs, "on=[order]") {
				t.Errorf("IF the page is took the wrong branch:\n%s", logs)
			}
			var res struct {
				Results []struct {
					Step     string `json:"step"`
					PageName string `json:"page_name"`
				} `json:"results"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
				t.Fatalf("--json: %v\n%s", err, stdout.String())
			}
			named := map[string]string{}
			for _, r := range res.Results {
				named[r.Step] = r.PageName
			}
			if got := named["EXTRACT the 'Price' into {price}"]; got != "Order details" {
				t.Errorf("EXTRACT's page_name = %q, want Order details; results %v", got, named)
			}
		})
	}
}
