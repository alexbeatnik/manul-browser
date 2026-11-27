//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The guide manul mcp gives Claude for building a test suite carries a whole
// example project. A guide is believed, so this runs that project as the
// guide prints it — the layout, the imports, the data, --var for the site and
// MANUL_VAR_password for the secret — against the quickstart site it was
// written for, on every browser.
func TestSuiteGuide_ExampleProjectRuns(t *testing.T) {
	guide, err := os.ReadFile(filepath.Join("..", "pkg", "mcp", "suite_guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	for _, m := range regexp.MustCompile("(?s)```[a-z]+ ([^\\s`]+)\n(.*?)```").FindAllSubmatch(guide, -1) {
		p := filepath.Join(project, filepath.FromSlash(string(m[1])))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, m[2], 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.FileServer(http.Dir(filepath.Join("..", "..", "examples", "quickstart", "site")))}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	site := "http://" + ln.Addr().String()

	names, required := browsers()
	for _, browser := range names {
		t.Run(browser, func(t *testing.T) {
			run := func(extra ...string) (string, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*huntTimeout)
				defer cancel()
				args := append([]string{"run", "tests/", "--headless", "--browser", browser, "--var", "site=" + site}, extra...)
				cmd := exec.CommandContext(ctx, binary, args...)
				cmd.Dir = project // manul.config.json is read from here
				cmd.Env = append(os.Environ(), "MANUL_VAR_password=lovelace")
				var out bytes.Buffer
				cmd.Stdout, cmd.Stderr = &out, &out
				err := cmd.Run()
				logs := out.String()
				if !required && (strings.Contains(logs, "chrome not found") || strings.Contains(logs, "firefox not found")) {
					t.Skipf("%s is not installed", browser)
				}
				return logs, err
			}

			logs, err := run()
			if err != nil {
				t.Fatalf("the guide's suite failed: %v\n%s", err, logs)
			}
			for _, title := range []string{"login.hunt", "login_rejected.hunt", "cart.hunt", "roles.hunt"} {
				if !strings.Contains(logs, title) {
					t.Errorf("%s did not run:\n%s", title, logs)
				}
			}

			logs, err = run("--tags", "smoke")
			if err != nil {
				t.Fatalf("--tags smoke: %v\n%s", err, logs)
			}
			if strings.Contains(logs, "roles.hunt") || strings.Contains(logs, "login_rejected.hunt") || !strings.Contains(logs, "cart.hunt") {
				t.Errorf("--tags smoke ran the wrong tests:\n%s", logs)
			}
		})
	}
}
