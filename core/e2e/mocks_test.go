//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A MOCK ends with its hunt. Every hunt of a run gets a page of its own on
// the same tab; on Firefox the last one's intercept stayed on the tab, and a
// hunt that mocked an API made every hunt after it read the mock. The hunt
// runner runs one file per process, so this needs two files in one run.
func TestMocks_EndWithTheirHunt(t *testing.T) {
	o := &origins{}
	o.site = serve(t, o)
	names, required := browsers()
	for _, browser := range names {
		t.Run(browser, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{
				"lang.txt": "mocked",
				"a_mocked.hunt": "@title: mocked\nSTEP 1: The header answered from a file\n" +
					"    MOCK GET \"echo-lang\" with 'lang.txt'\n" +
					"    NAVIGATE to " + o.site + "/locale.html\n" +
					"    VERIFY that 'header: mocked' is present\nDONE.\n",
				"b_plain.hunt": "@title: plain\nSTEP 1: The next hunt asks the server\n" +
					"    NAVIGATE to " + o.site + "/locale.html\n" +
					"    VERIFY that 'header: en-US' is present\n" +
					"    VERIFY that 'header: mocked' is NOT present\nDONE.\n",
			}
			for name, body := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), huntTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "run", "a_mocked.hunt", "b_plain.hunt", "--headless", "--browser", browser)
			cmd.Dir = dir
			var stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stderr, &stderr
			err := cmd.Run()
			logs := stderr.String()
			if !required && (strings.Contains(logs, "chrome not found") || strings.Contains(logs, "firefox not found")) {
				t.Skipf("%s is not installed", browser)
			}
			if err != nil {
				t.Fatalf("run failed: %v\n%s", err, logs)
			}
		})
	}
}
