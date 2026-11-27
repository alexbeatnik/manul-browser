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

// browser_args was read from manul.config.json and MANUL_BROWSER_ARGS and
// passed to no browser: the contract's own --window-size example did nothing.
func TestBrowserArgs_ReachTheBrowser(t *testing.T) {
	o := &origins{}
	o.site = serve(t, o)
	names, required := browsers()
	for _, browser := range names {
		t.Run(browser, func(t *testing.T) {
			dir := t.TempDir()
			hunt := filepath.Join(dir, "width.hunt")
			src := "STEP 1: Look\n    NAVIGATE to " + o.site + "/width.html\n    VERIFY that 'Width 1000' is present\nDONE.\n"
			if err := os.WriteFile(hunt, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), huntTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "run", hunt, "--headless", "--browser", browser)
			cmd.Dir = dir
			// Each engine has its own flag: headless Firefox ignores
			// --window-size and takes -width and -height. Chrome's comes
			// from the environment, where the comma inside the value used to
			// split it in two; Firefox's from manul.config.json.
			if browser == "firefox" {
				if err := os.WriteFile(filepath.Join(dir, "manul.config.json"), []byte(`{"browser_args": ["-width", "1000", "-height", "700"]}`), 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				cmd.Env = append(os.Environ(), "MANUL_BROWSER_ARGS=--window-size=1000,700")
			}
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			err := cmd.Run()
			logs := out.String()
			if !required && (strings.Contains(logs, "chrome not found") || strings.Contains(logs, "firefox not found")) {
				t.Skipf("%s is not installed", browser)
			}
			if err != nil {
				t.Fatalf("the page did not get the window the args asked for: %v\n%s", err, logs)
			}
		})
	}
}
